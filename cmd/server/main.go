// Package main is the GoIM server entry point.
// It wires all infrastructure components (config, MySQL, Redis, RabbitMQ)
// and starts a Gin HTTP server + WebSocket handler + MQ consumers.
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
	configPath := flag.String("c", "configs/config.yaml", "config file path")
	flag.Parse()

	// ── Load config ──
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// ── Init logger ──
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	// ── Init MySQL ──
	db, err := infra.NewMySQLPool(&cfg.MySQL)
	if err != nil {
		logger.Fatal("failed to connect MySQL", zap.Error(err))
	}
	if err := db.Ping(); err != nil {
		logger.Fatal("failed to ping MySQL", zap.Error(err))
	}
	logger.Info("MySQL connected")

	// ── Init Redis ──
	rdb, err := infra.NewRedisClient(&cfg.Redis)
	if err != nil {
		logger.Fatal("failed to connect Redis", zap.Error(err))
	}
	logger.Info("Redis connected")

	// ── Init RabbitMQ ──
	mqConn, mqCh, err := infra.NewRabbitMQConn(&cfg.RabbitMQ)
	if err != nil {
		logger.Fatal("failed to connect RabbitMQ", zap.Error(err))
	}
	defer mqConn.Close()

	if err := infra.DeclareQueues(mqCh); err != nil {
		logger.Fatal("failed to declare RabbitMQ queues", zap.Error(err))
	}
	logger.Info("RabbitMQ connected and queues declared")

	// ── Load Lua scripts into Redis ──
	ctx := context.Background()
	if err := goredis.LoadLuaScripts(rdb, ctx); err != nil {
		logger.Fatal("failed to load Redis Lua scripts", zap.Error(err))
	}
	logger.Info("Redis Lua scripts loaded")

	// ── Build repositories ──
	mysqlRepo := repository.NewMySQLRepo(db)
	redisRepo := repository.NewRedisRepo(rdb)
	mqRepo := repository.NewMQRepo(mqCh)

	// ── Init ConnectionManager ──
	cm := conn.NewConnectionManager()

	// ── Init LLM client ──
	llmClient := llm.NewLLMClient(cfg.LLM)

	// ── Init services ──
	msgSvc := service.NewMsgService(redisRepo, mqRepo, cm, logger)
	authSvc := service.NewAuthService(mysqlRepo, cfg.JWT.Secret, cfg.JWT.AccessExpHours, cfg.JWT.RefreshExpDays)
	friendSvc := service.NewFriendService(mysqlRepo, redisRepo, logger)
	groupSvc := service.NewGroupService(mysqlRepo, redisRepo, logger)
	momentSvc := service.NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)
	aiSvc := service.NewAIService(mysqlRepo, redisRepo, llmClient, logger)
	msgOpSvc := service.NewMsgOpService(mysqlRepo, redisRepo, logger)
	settingsSvc := service.NewSettingsService(mysqlRepo, logger)

	// ── Init WebSocket dispatcher ──
	dispatcher := ws.NewMessageDispatcher(msgSvc, friendSvc, aiSvc)

	// ── Init HTTP handlers ──
	authHandler := api.NewAuthHandler(authSvc)
	friendHandler := api.NewFriendHandler(friendSvc)
	groupHandler := api.NewGroupHandler(groupSvc)
	momentHandler := api.NewMomentHandler(momentSvc)
	aiHandler := api.NewAIHandler(aiSvc)
	msgOpHandler := api.NewMsgOpHandler(msgOpSvc)
	settingsHandler := api.NewSettingsHandler(settingsSvc)

	// ── Setup Gin router ──
	router := setupRouter(
		cfg, mysqlRepo, redisRepo, rdb, mqCh, cm,
		dispatcher, logger,
		authHandler, friendHandler, groupHandler,
		momentHandler, aiHandler, msgOpHandler, settingsHandler,
	)

	// ── Start MQ consumers ──
	privateMsgConsumer := consumer.NewPrivateMsgConsumer(mqCh, mysqlRepo, redisRepo, cm, logger)
	groupMsgConsumer := consumer.NewGroupMsgConsumer(mqCh, mysqlRepo, redisRepo, cm, logger)
	momentFeedConsumer := consumer.NewMomentFeedConsumer(mqCh, mysqlRepo, redisRepo, logger)

	if err := privateMsgConsumer.Start(ctx); err != nil {
		logger.Fatal("failed to start private msg consumer", zap.Error(err))
	}
	if err := groupMsgConsumer.Start(ctx); err != nil {
		logger.Fatal("failed to start group msg consumer", zap.Error(err))
	}
	if err := momentFeedConsumer.Start(ctx); err != nil {
		logger.Fatal("failed to start moment feed consumer", zap.Error(err))
	}
	logger.Info("MQ consumers started")

	// ── Start cleanup task ──
	infra.StartCleanupTask(rdb, logger, 1*time.Hour)

	// ── Start HTTP server ──
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: router}

	go func() {
		logger.Info("starting HTTP server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	logger.Info("GoIM server started", zap.Int("port", cfg.Server.Port))

	// ── Graceful shutdown ──
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced shutdown", zap.Error(err))
	}

	logger.Info("server exited")
}

// ──────────────────────────────────────────────────────
// Router setup — wires all HTTP routes + WebSocket endpoint
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

	// ── Health check ──
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "goim"})
	})

	// ── Public routes (no auth required) ──
	public := r.Group("/api/v1")
	authHandler.RegisterRoutes(public)

	// ── Protected routes (JWT auth required) ──
	protected := r.Group("/api/v1")
	protected.Use(middleware.JWTAuthMiddleware(cfg.JWT.Secret))
	friendHandler.RegisterRoutes(protected)
	groupHandler.RegisterRoutes(protected)
	momentHandler.RegisterRoutes(protected)
	aiHandler.RegisterRoutes(protected)
	msgOpHandler.RegisterRoutes(protected)
	settingsHandler.RegisterRoutes(protected)

	// ── WebSocket endpoint ──
	wsHandler := ws.ServeWebSocket(cfg.JWT.Secret, rdb, cm, dispatcher.Callback())
	r.GET(cfg.Server.WsPath, wsHandler)

	return r
}
