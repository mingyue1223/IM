// Package main 是 GoIM 服务端入口。
// 它组装所有基础设施组件（配置、MySQL、Redis、RabbitMQ），
// 并启动 Gin HTTP 服务器 + WebSocket 处理器 + MQ 消费者。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
	goredisv9 "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/api"
	"github.com/goim/goim/internal/config"
	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/consumer"
	"github.com/goim/goim/internal/infra"
	"github.com/goim/goim/internal/llm"
	"github.com/goim/goim/internal/middleware"
	"github.com/goim/goim/internal/repository"
	goredis "github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/service"
	"github.com/goim/goim/internal/ws"
)

func main() {
	configPath := flag.String("c", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	// ── 加载配置 ──
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// ── 初始化日志 ──
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Sync()

	// ── 初始化 MySQL ──
	db, err := infra.NewMySQLPool(&cfg.MySQL)
	if err != nil {
		logger.Fatal("连接 MySQL 失败", zap.Error(err))
	}
	if err := db.Ping(); err != nil {
		logger.Fatal("Ping MySQL 失败", zap.Error(err))
	}
	logger.Info("MySQL 已连接")

	// ── 初始化 Redis ──
	rdb, err := infra.NewRedisClient(&cfg.Redis)
	if err != nil {
		logger.Fatal("连接 Redis 失败", zap.Error(err))
	}
	logger.Info("Redis 已连接")

	// ── 初始化 RabbitMQ ──
	mqConn, mqCh, err := infra.NewRabbitMQConn(&cfg.RabbitMQ)
	if err != nil {
		logger.Fatal("连接 RabbitMQ 失败", zap.Error(err))
	}
	defer mqConn.Close()

	if err := infra.DeclareQueues(mqCh); err != nil {
		logger.Fatal("声明 RabbitMQ 队列失败", zap.Error(err))
	}
	logger.Info("RabbitMQ 已连接，队列已声明")

	// ── 加载 Lua 脚本到 Redis ──
	ctx := context.Background()
	if err := goredis.LoadLuaScripts(rdb, ctx); err != nil {
		logger.Fatal("加载 Redis Lua 脚本失败", zap.Error(err))
	}
	logger.Info("Redis Lua 脚本已加载")

	// ── 构建仓库层 ──
	mysqlRepo := repository.NewMySQLRepo(db)
	redisRepo := repository.NewRedisRepo(rdb)
	mqRepo := repository.NewMQRepo(mqCh)

	// ── 初始化连接管理器 ──
	cm := conn.NewConnectionManager()

	// ── 初始化 LLM 客户端 ──
	llmClient := llm.NewLLMClient(cfg.LLM)

	// ── 初始化服务层 ──
	msgSvc := service.NewMsgService(redisRepo, mqRepo, cm, logger)
	authSvc := service.NewAuthService(mysqlRepo, cfg.JWT.Secret, cfg.JWT.AccessExpHours, cfg.JWT.RefreshExpDays)
	friendSvc := service.NewFriendService(mysqlRepo, redisRepo, logger)
	groupSvc := service.NewGroupService(mysqlRepo, redisRepo, logger)
	momentSvc := service.NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)
	aiSvc := service.NewAIService(mysqlRepo, redisRepo, llmClient, logger)
	msgOpSvc := service.NewMsgOpService(mysqlRepo, redisRepo, logger)
	settingsSvc := service.NewSettingsService(mysqlRepo, logger)

	// ── 初始化 WebSocket 分发器 ──
	dispatcher := ws.NewMessageDispatcher(msgSvc, friendSvc, aiSvc)

	// ── 初始化 HTTP 处理器 ──
	authHandler := api.NewAuthHandler(authSvc)
	friendHandler := api.NewFriendHandler(friendSvc)
	groupHandler := api.NewGroupHandler(groupSvc)
	momentHandler := api.NewMomentHandler(momentSvc)
	aiHandler := api.NewAIHandler(aiSvc)
	msgOpHandler := api.NewMsgOpHandler(msgOpSvc)
	settingsHandler := api.NewSettingsHandler(settingsSvc)

	// ── 设置 Gin 路由 ──
	router := setupRouter(
		cfg, mysqlRepo, redisRepo, rdb, mqCh, cm,
		dispatcher, logger,
		authHandler, friendHandler, groupHandler,
		momentHandler, aiHandler, msgOpHandler, settingsHandler,
	)

	// ── 启动 MQ 消费者 ──
	privateMsgConsumer := consumer.NewPrivateMsgConsumer(mqCh, mysqlRepo, redisRepo, cm, logger)
	groupMsgConsumer := consumer.NewGroupMsgConsumer(mqCh, mysqlRepo, redisRepo, cm, logger)
	momentFeedConsumer := consumer.NewMomentFeedConsumer(mqCh, mysqlRepo, redisRepo, logger, cfg.Moment.BigUserFriendThreshold, cfg.Moment.TimelineMaxLen)

	if err := privateMsgConsumer.Start(ctx); err != nil {
		logger.Fatal("启动私聊消息消费者失败", zap.Error(err))
	}
	if err := groupMsgConsumer.Start(ctx); err != nil {
		logger.Fatal("启动群聊消息消费者失败", zap.Error(err))
	}
	if err := momentFeedConsumer.Start(ctx); err != nil {
		logger.Fatal("启动动态推送消费者失败", zap.Error(err))
	}
	logger.Info("MQ 消费者已启动")

	// ── 启动清理任务 ──
	infra.StartCleanupTask(rdb, logger, 1*time.Hour)

	// ── 启动 HTTP 服务器 ──
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: router}

	go func() {
		logger.Info("正在启动 HTTP 服务器", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP 服务器启动失败", zap.Error(err))
		}
	}()

	logger.Info("GoIM 服务器已启动", zap.Int("port", cfg.Server.Port))

	// ── 优雅关闭 ──
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("正在关闭服务器...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("服务器强制关闭", zap.Error(err))
	}

	logger.Info("服务器已退出")
}

// ──────────────────────────────────────────────────────
// 路由设置 — 绑定所有 HTTP 路由 + WebSocket 端点
// ──────────────────────────────────────────────────────

func setupRouter(
	cfg *config.Config,
	mysqlRepo repository.MySQLRepo,
	redisRepo repository.RedisRepo,
	rdb *goredisv9.Client,
	mqCh *amqp.Channel,
	cm *conn.ConnectionManager,
	dispatcher *ws.MessageDispatcher,
	logger *zap.Logger,
	authHandler *api.AuthHandler,
	friendHandler *api.FriendHandler,
	groupHandler *api.GroupHandler,
	momentHandler *api.MomentHandler,
	aiHandler *api.AIHandler,
	msgOpHandler *api.MsgOpHandler,
	settingsHandler *api.SettingsHandler,
) *gin.Engine {
	r := gin.Default()

	// ── 健康检查 ──
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "goim"})
	})

	// ── 公开路由（无需认证）──
	public := r.Group("/api/v1")
	authHandler.RegisterRoutes(public)

	// ── 受保护路由（需要 JWT 认证）──
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuthMiddleware(cfg.JWT.Secret))
	friendHandler.RegisterRoutes(protected)
	groupHandler.RegisterRoutes(protected)
	momentHandler.RegisterRoutes(protected)
	aiHandler.RegisterRoutes(protected)
	msgOpHandler.RegisterRoutes(protected)
	settingsHandler.RegisterRoutes(protected)

	// ── WebSocket 端点 ──
	wsHandler := ws.ServeWebSocket(cfg.JWT.Secret, rdb, cm, dispatcher.Callback())
	r.GET(cfg.Server.WsPath, wsHandler)

	return r
}
