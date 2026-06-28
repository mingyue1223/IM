// Package main is the GoIM server entry point.
// It wires all infrastructure components (config, MySQL, Redis, RabbitMQ)
// and starts a Gin HTTP server with a health-check endpoint.
// Components from future tasks (ConnectionManager, WebSocket upgrade,
// MQ consumers, cleanup task, API routes) are stubbed with TODO markers.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/config"
	"github.com/goim/goim/internal/infra"
	goredis "github.com/goim/goim/internal/redis"
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

	// ── TODO: Init ConnectionManager (Task 7) ──
	// cm := conn.NewConnectionManager()

	// ── TODO: Init services (Tasks 9-17) ──
	// msgSvc := service.NewMessageService(db, rdb, mqCh, logger)
	// authSvc := service.NewAuthService(db, rdb, cfg, logger)
	// friendSvc := service.NewFriendService(db, rdb, logger)
	// groupSvc := service.NewGroupService(db, rdb, mqCh, logger)
	// momentSvc := service.NewMomentService(db, rdb, mqCh, logger)
	// aiSvc := service.NewAIService(db, rdb, cfg.LLM, logger)
	// msgOpSvc := service.NewMsgOpService(db, rdb, logger)
	// userSettingsSvc := service.NewUserSettingsService(db, rdb, logger)

	// ── Setup Gin router with health-check ──
	router := setupRouter()
	// TODO: Replace with api.SetupRouter(cfg, db, rdb, mqCh, cm, logger) (Task 18)

	// ── Start HTTP server ──
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	go func() {
		logger.Info("starting HTTP server", zap.String("addr", addr))
		if err := router.Run(addr); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// ── TODO: Start MQ consumers (Task 10) ──
	// consumer.StartAll(mqCh, db, rdb, cm, logger)

	// ── TODO: Start cleanup task (Task 11) ──
	// infra.StartCleanupTask(rdb, logger)

	logger.Info("GoIM server started", zap.Int("port", cfg.Server.Port))

	// Block forever — goroutines handle all work.
	select {}
}

// setupRouter creates a minimal Gin router with a health-check endpoint.
// This will be replaced by api.SetupRouter in Task 18.
func setupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"service": "goim",
		})
	})

	return r
}
