//go:build e2e

// Package e2e contains end-to-end integration tests for the GoIM server.
// These tests exercise the full stack: HTTP API, WebSocket, MQ consumers,
// Redis, MySQL, and RabbitMQ. They require Docker services to be running
// (as configured in configs/config.test.yaml).
//
// Run with: go test ./tests/... -v -tags e2e -timeout 120s
package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
	goredis "github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/repository"
	"github.com/goim/goim/internal/service"
	"github.com/goim/goim/internal/ws"
)

// ──────────────────────────────────────────────────────
// Test environment setup
// ──────────────────────────────────────────────────────

// testEnv holds the full test environment: server, connections, base URL.
type testEnv struct {
	baseURL  string
	db       *sql.DB
	rdb      *goredisv9.Client
	mqConn   *amqp.Connection
	mqCh     *amqp.Channel
	logger   *zap.Logger
	server   *httptest.Server
	cancel   context.CancelFunc
}

var env *testEnv

// TestMain sets up the full GoIM server for E2E testing.
// It requires MySQL, Redis, and RabbitMQ to be running on localhost
// (as configured in configs/config.test.yaml).
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// Load test config
	cfgPath := "configs/config.test.yaml"
	if v := os.Getenv("GOIM_CONFIG"); v != "" {
		cfgPath = v
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	// Override server port (httptest will pick its own)
	cfg.Server.Port = 0

	// Init logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	// ── Connect MySQL ──
	db, err := infra.NewMySQLPool(&cfg.MySQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect MySQL: %v\n", err)
		os.Exit(1)
	}
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "ping MySQL: %v\n", err)
		os.Exit(1)
	}
	logger.Info("MySQL connected for E2E")

	// ── Connect Redis ──
	rdb, err := infra.NewRedisClient(&cfg.Redis)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect Redis: %v\n", err)
		os.Exit(1)
	}
	logger.Info("Redis connected for E2E")

	// ── Connect RabbitMQ ──
	mqConn, mqCh, err := infra.NewRabbitMQConn(&cfg.RabbitMQ)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect RabbitMQ: %v\n", err)
		os.Exit(1)
	}
	if err := infra.DeclareQueues(mqCh); err != nil {
		fmt.Fprintf(os.Stderr, "declare queues: %v\n", err)
		os.Exit(1)
	}
	logger.Info("RabbitMQ connected for E2E")

	// ── Load Lua scripts ──
	ctx := context.Background()
	if err := goredis.LoadLuaScripts(rdb, ctx); err != nil {
		fmt.Fprintf(os.Stderr, "load Lua scripts: %v\n", err)
		os.Exit(1)
	}

	// ── Build repos ──
	mysqlRepo := repository.NewMySQLRepo(db)
	redisRepo := repository.NewRedisRepo(rdb)
	mqRepo := repository.NewMQRepo(mqCh)

	// ── Build ConnectionManager ──
	cm := conn.NewConnectionManager()

	// ── Build LLM client ──
	llmClient := llm.NewLLMClient(cfg.LLM)

	// ── Build services ──
	msgSvc := service.NewMsgService(redisRepo, mqRepo, cm, logger)
	authSvc := service.NewAuthService(mysqlRepo, cfg.JWT.Secret, cfg.JWT.AccessExpHours, cfg.JWT.RefreshExpDays)
	friendSvc := service.NewFriendService(mysqlRepo, redisRepo, logger)
	groupSvc := service.NewGroupService(mysqlRepo, redisRepo, logger)
	momentSvc := service.NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)
	aiSvc := service.NewAIService(mysqlRepo, redisRepo, llmClient, logger)
	msgOpSvc := service.NewMsgOpService(mysqlRepo, redisRepo, logger)
	settingsSvc := service.NewSettingsService(mysqlRepo, logger)

	// ── Build WS dispatcher ──
	dispatcher := ws.NewMessageDispatcher(msgSvc, friendSvc, aiSvc)

	// ── Build HTTP handlers ──
	authHandler := api.NewAuthHandler(authSvc)
	friendHandler := api.NewFriendHandler(friendSvc)
	groupHandler := api.NewGroupHandler(groupSvc)
	momentHandler := api.NewMomentHandler(momentSvc)
	aiHandler := api.NewAIHandler(aiSvc)
	msgOpHandler := api.NewMsgOpHandler(msgOpSvc)
	settingsHandler := api.NewSettingsHandler(settingsSvc)

	// ── Build Gin router ──
	router := buildRouter(cfg, rdb, cm, dispatcher, logger,
		authHandler, friendHandler, groupHandler,
		momentHandler, aiHandler, msgOpHandler, settingsHandler)

	// ── Start MQ consumers ──
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	privateMsgConsumer := consumer.NewPrivateMsgConsumer(mqCh, mysqlRepo, redisRepo, cm, logger)
	groupMsgConsumer := consumer.NewGroupMsgConsumer(mqCh, mysqlRepo, redisRepo, cm, logger)
	momentFeedConsumer := consumer.NewMomentFeedConsumer(mqCh, mysqlRepo, redisRepo, logger)

	if err := privateMsgConsumer.Start(consumerCtx); err != nil {
		fmt.Fprintf(os.Stderr, "start private msg consumer: %v\n", err)
		os.Exit(1)
	}
	if err := groupMsgConsumer.Start(consumerCtx); err != nil {
		fmt.Fprintf(os.Stderr, "start group msg consumer: %v\n", err)
		os.Exit(1)
	}
	if err := momentFeedConsumer.Start(consumerCtx); err != nil {
		fmt.Fprintf(os.Stderr, "start moment feed consumer: %v\n", err)
		os.Exit(1)
	}
	logger.Info("MQ consumers started for E2E")

	// ── Start cleanup task ──
	infra.StartCleanupTask(rdb, logger, 1*time.Hour)

	// ── Start httptest server ──
	server := httptest.NewServer(router)

	env = &testEnv{
		baseURL:  server.URL,
		db:       db,
		rdb:      rdb,
		mqConn:   mqConn,
		mqCh:     mqCh,
		logger:   logger,
		server:   server,
		cancel:   consumerCancel,
	}

	logger.Info("GoIM E2E server started", zap.String("url", env.baseURL))

	// Run tests
	code := m.Run()

	// ── Cleanup ──
	consumerCancel()
	server.Close()
	mqConn.Close()
	rdb.Close()
	db.Close()
	logger.Sync()
	os.Exit(code)
}

// buildRouter creates the Gin router with all routes wired for E2E testing.
func buildRouter(
	cfg *config.Config,
	rdb *goredisv9.Client,
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
	r := gin.New()
	r.Use(gin.Recovery())

	// ── Health check ──
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "goim"})
	})

	// ── Public routes ──
	public := r.Group("/api/v1")
	authHandler.RegisterRoutes(public)

	// ── Protected routes ──
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

// ──────────────────────────────────────────────────────
// E2E Tests
// ──────────────────────────────────────────────────────

// TestE2E_HealthCheck verifies the /health endpoint returns ok.
func TestE2E_HealthCheck(t *testing.T) {
	status, body := doRequest(t, http.MethodGet, env.baseURL, "/health", nil, "")
	if status != http.StatusOK {
		t.Fatalf("health check: status=%d body=%s", status, body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", resp["status"])
	}
}

// TestE2E_AuthFlow tests the full auth lifecycle:
// register → login → refresh → use refreshed token.
func TestE2E_AuthFlow(t *testing.T) {
	username := fmt.Sprintf("e2e_auth_%d", time.Now().UnixNano())
	password := "testpass123"

	// Step 1: Register
	userID := registerUser(t, env.baseURL, username, password)
	t.Logf("registered user: id=%d username=%s", userID, username)
	if userID == 0 {
		t.Fatal("expected non-zero user ID")
	}

	// Step 2: Login
	loginResp := loginUserFull(t, env.baseURL, username, password)
	t.Logf("logged in: token=%s...", loginResp.AccessToken[:20])

	// Step 3: Use access token on protected endpoint
	status, _ := doAuthedRequest(t, http.MethodGet, env.baseURL,
		"/api/v1/friend/list", nil, loginResp.AccessToken)
	if status == http.StatusUnauthorized {
		t.Fatal("access token should work on protected endpoints")
	}

	// Step 4: Refresh token
	refreshResp := refreshToken(t, env.baseURL, loginResp.RefreshToken)
	t.Logf("refreshed token: %s...", refreshResp.AccessToken[:20])

	// Step 5: Use refreshed token
	status, _ = doAuthedRequest(t, http.MethodGet, env.baseURL,
		"/api/v1/friend/list", nil, refreshResp.AccessToken)
	if status == http.StatusUnauthorized {
		t.Fatal("refreshed token should work on protected endpoints")
	}

	// Step 6: Duplicate registration should fail
	status, _ = doRequest(t, http.MethodPost, env.baseURL, "/api/v1/auth/register",
		map[string]string{"username": username, "password": password}, "")
	if status != http.StatusConflict {
		t.Fatalf("duplicate registration should return 409, got %d", status)
	}
}

// TestE2E_FriendFlow tests the full friend lifecycle:
// send request → accept → list → block → unblock → delete.
func TestE2E_FriendFlow(t *testing.T) {
	u1Name := fmt.Sprintf("e2e_fr1_%d", time.Now().UnixNano())
	u2Name := fmt.Sprintf("e2e_fr2_%d", time.Now().UnixNano())
	u1ID := registerUser(t, env.baseURL, u1Name, "pass1234")
	u2ID := registerUser(t, env.baseURL, u2Name, "pass1234")
	u1Token := loginUser(t, env.baseURL, u1Name, "pass1234")
	u2Token := loginUser(t, env.baseURL, u2Name, "pass1234")

	// Step 1: Send friend request (u1 → u2)
	status, body := doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/request",
		map[string]interface{}{"to_user_id": u2ID, "message": "let's be friends"}, u1Token)
	if status != http.StatusCreated {
		t.Fatalf("send friend request: status=%d body=%s", status, body)
	}
	var frResp friendRequestResp
	if err := json.Unmarshal(body, &frResp); err != nil {
		t.Fatalf("unmarshal friend request: %v", err)
	}
	t.Logf("friend request: id=%d from=%d to=%d", frResp.RequestID, frResp.FromUserID, frResp.ToUserID)

	// Step 2: Accept (u2)
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/accept",
		map[string]interface{}{"request_id": frResp.RequestID}, u2Token)
	if status != http.StatusOK {
		t.Fatalf("accept friend request: status=%d body=%s", status, body)
	}
	var acceptResp acceptFriendResp
	if err := json.Unmarshal(body, &acceptResp); err != nil {
		t.Fatalf("unmarshal accept: %v", err)
	}
	t.Logf("friend accepted: user=%d friend=%d", acceptResp.UserID, acceptResp.FriendID)

	// Step 3: Get friend list (u1)
	status, body = doAuthedRequest(t, http.MethodGet, env.baseURL, "/api/v1/friend/list", nil, u1Token)
	if status != http.StatusOK {
		t.Fatalf("get friend list: status=%d body=%s", status, body)
	}
	t.Logf("friend list for u1: %s", body)

	// Step 4: Block (u1 blocks u2)
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/block",
		map[string]interface{}{"blocked_id": u2ID}, u1Token)
	if status != http.StatusOK {
		t.Fatalf("block user: status=%d body=%s", status, body)
	}

	// Step 5: Unblock
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/unblock",
		map[string]interface{}{"blocked_id": u2ID}, u1Token)
	if status != http.StatusOK {
		t.Fatalf("unblock user: status=%d body=%s", status, body)
	}

	// Step 6: Delete friend
	status, body = doAuthedRequest(t, http.MethodDelete, env.baseURL,
		fmt.Sprintf("/api/v1/friend/%d", u2ID), nil, u1Token)
	if status != http.StatusOK {
		t.Fatalf("delete friend: status=%d body=%s", status, body)
	}
	t.Logf("deleted friend %d", u2ID)
}

// TestE2E_GroupMessaging tests group creation, member management, and WS messaging.
func TestE2E_GroupMessaging(t *testing.T) {
	u1Name := fmt.Sprintf("e2e_grp1_%d", time.Now().UnixNano())
	u2Name := fmt.Sprintf("e2e_grp2_%d", time.Now().UnixNano())
	u1ID := registerUser(t, env.baseURL, u1Name, "pass1234")
	u2ID := registerUser(t, env.baseURL, u2Name, "pass1234")
	u1Token := loginUser(t, env.baseURL, u1Name, "pass1234")
	u2Token := loginUser(t, env.baseURL, u2Name, "pass1234")

	// Step 1: Create group (u1 is owner)
	status, body := doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/group",
		map[string]interface{}{"name": "E2E Test Group", "notice": "test group"}, u1Token)
	if status != http.StatusCreated {
		t.Fatalf("create group: status=%d body=%s", status, body)
	}
	var grpResp createGroupResp
	if err := json.Unmarshal(body, &grpResp); err != nil {
		t.Fatalf("unmarshal create group: %v", err)
	}
	groupID := grpResp.GroupID
	t.Logf("group created: id=%d", groupID)

	// Step 2: Add member u2
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL,
		fmt.Sprintf("/api/v1/group/%d/member", groupID),
		map[string]interface{}{"member_id": u2ID}, u1Token)
	if status != http.StatusOK {
		t.Fatalf("add member: status=%d body=%s", status, body)
	}

	// Step 3: Get group info
	status, body = doAuthedRequest(t, http.MethodGet, env.baseURL,
		fmt.Sprintf("/api/v1/group/%d", groupID), nil, u1Token)
	if status != http.StatusOK {
		t.Fatalf("get group info: status=%d body=%s", status, body)
	}

	// Step 4: Get members
	status, body = doAuthedRequest(t, http.MethodGet, env.baseURL,
		fmt.Sprintf("/api/v1/group/%d/members", groupID), nil, u1Token)
	if status != http.StatusOK {
		t.Fatalf("get members: status=%d body=%s", status, body)
	}

	// Step 5: Connect u1 via WS, send group message
	wsConn1 := connectWS(t, env.baseURL, u1Token)
	defer closeWS(t, wsConn1)
	time.Sleep(500 * time.Millisecond)

	clientMsgID := fmt.Sprintf("e2e_grp_%d", time.Now().UnixNano())
	sendWSMessage(t, wsConn1, "msg", sendMsgData{
		ClientMsgID: clientMsgID,
		ConvType:    2,
		ToID:        groupID,
		MsgType:     1,
		Content:     "hello group from E2E",
		Timestamp:   time.Now().Unix(),
	})

	// Read serverAck
	ack := readWSMessageType(t, wsConn1, "serverAck", 10*time.Second)
	var ackData serverAckData
	if err := json.Unmarshal(ack.Data, &ackData); err != nil {
		t.Fatalf("unmarshal serverAck: %v", err)
	}
	if ackData.ServerMsgID == 0 {
		t.Fatal("expected non-zero serverMsgID")
	}
	t.Logf("group msg acked: serverMsgID=%d groupSeq=%d", ackData.ServerMsgID, ackData.GroupSeq)

	// Step 6: Connect u2 via WS, sync messages
	wsConn2 := connectWS(t, env.baseURL, u2Token)
	defer closeWS(t, wsConn2)
	time.Sleep(500 * time.Millisecond)

	sendWSMessage(t, wsConn2, "syncReq", map[string]interface{}{
		"lastSyncTime": 0,
		"batchSize":    100,
	})

	// Read sync response
	syncMsg := readWSMessage(t, wsConn2, 10*time.Second)
	dataStr := string(syncMsg.Data)
	if len(dataStr) > 200 {
		dataStr = dataStr[:200]
	}
	t.Logf("u2 sync: type=%s data=%s", syncMsg.Type, dataStr)
}

// TestE2E_PrivateMessaging tests private messaging between two friends via WS.
func TestE2E_PrivateMessaging(t *testing.T) {
	u1Name := fmt.Sprintf("e2e_pm1_%d", time.Now().UnixNano())
	u2Name := fmt.Sprintf("e2e_pm2_%d", time.Now().UnixNano())
	u1ID := registerUser(t, env.baseURL, u1Name, "pass1234")
	u2ID := registerUser(t, env.baseURL, u2Name, "pass1234")
	u1Token := loginUser(t, env.baseURL, u1Name, "pass1234")
	u2Token := loginUser(t, env.baseURL, u2Name, "pass1234")

	// Make them friends (required for private msg — Lua check validates friendship)
	status, body := doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/request",
		map[string]interface{}{"to_user_id": u2ID, "message": "let's chat"}, u1Token)
	if status != http.StatusCreated {
		t.Fatalf("friend request: status=%d body=%s", status, body)
	}
	var frResp friendRequestResp
	json.Unmarshal(body, &frResp)

	status, _ = doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/accept",
		map[string]interface{}{"request_id": frResp.RequestID}, u2Token)
	if status != http.StatusOK {
		t.Fatalf("accept friend: status=%d", status)
	}

	// Connect u1 and u2 via WS
	wsConn1 := connectWS(t, env.baseURL, u1Token)
	defer closeWS(t, wsConn1)
	time.Sleep(500 * time.Millisecond)

	wsConn2 := connectWS(t, env.baseURL, u2Token)
	defer closeWS(t, wsConn2)
	time.Sleep(500 * time.Millisecond)

	// Send private message from u1 to u2
	clientMsgID := fmt.Sprintf("e2e_pm_%d", time.Now().UnixNano())
	sendWSMessage(t, wsConn1, "msg", sendMsgData{
		ClientMsgID: clientMsgID,
		ConvType:    1,
		ToID:        u2ID,
		MsgType:     1,
		Content:     "hello from E2E private msg",
		Timestamp:   time.Now().Unix(),
	})

	// Read serverAck on u1
	ack := readWSMessageType(t, wsConn1, "serverAck", 10*time.Second)
	var ackData serverAckData
	if err := json.Unmarshal(ack.Data, &ackData); err != nil {
		t.Fatalf("unmarshal serverAck: %v", err)
	}
	if ackData.ServerMsgID == 0 {
		t.Fatal("expected non-zero serverMsgID")
	}
	t.Logf("private msg acked: serverMsgID=%d", ackData.ServerMsgID)

	// u2 should receive the message (push model — consumer pushes to u2's WS)
	msgOnU2 := readWSMessageType(t, wsConn2, "msg", 15*time.Second)
	dataStr := string(msgOnU2.Data)
	if len(dataStr) > 200 {
		dataStr = dataStr[:200]
	}
	t.Logf("u2 received msg: data=%s", dataStr)

	// Send deliverAck from u2
	var receivedMsg struct {
		MsgID    int64  `json:"msgId"`
		ConvID   string `json:"convId"`
	}
	if err := json.Unmarshal(msgOnU2.Data, &receivedMsg); err == nil && receivedMsg.MsgID > 0 {
		sendWSMessage(t, wsConn2, "deliverAck", map[string]interface{}{
			"serverMsgId": receivedMsg.MsgID,
		})
	}

	// Send readAck from u2
	convID := fmt.Sprintf("p_%d_%d", min64(u1ID, u2ID), max64(u1ID, u2ID))
	sendWSMessage(t, wsConn2, "readAck", map[string]interface{}{
		"convId": convID,
	})
	t.Logf("u2 sent readAck for convId=%s", convID)
}

// TestE2E_MomentFlow tests moment lifecycle: publish → like → comment → feed.
func TestE2E_MomentFlow(t *testing.T) {
	u1Name := fmt.Sprintf("e2e_mom1_%d", time.Now().UnixNano())
	u2Name := fmt.Sprintf("e2e_mom2_%d", time.Now().UnixNano())
	u1ID := registerUser(t, env.baseURL, u1Name, "pass1234")
	u2ID := registerUser(t, env.baseURL, u2Name, "pass1234")
	u1Token := loginUser(t, env.baseURL, u1Name, "pass1234")
	u2Token := loginUser(t, env.baseURL, u2Name, "pass1234")

	// Make them friends (needed for feed fan-out)
	status, body := doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/request",
		map[string]interface{}{"to_user_id": u2ID, "message": "share moments"}, u1Token)
	if status != http.StatusCreated {
		t.Fatalf("friend request: status=%d body=%s", status, body)
	}
	var frResp friendRequestResp
	json.Unmarshal(body, &frResp)
	doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/friend/accept",
		map[string]interface{}{"request_id": frResp.RequestID}, u2Token)

	// Step 1: Publish moment (u1)
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/moment",
		map[string]interface{}{
			"content":    "E2E test moment!",
			"visibility": 2,
		}, u1Token)
	if status != http.StatusCreated {
		t.Fatalf("publish moment: status=%d body=%s", status, body)
	}
	var momResp publishMomentResp
	if err := json.Unmarshal(body, &momResp); err != nil {
		t.Fatalf("unmarshal moment: %v", err)
	}
	momentID := momResp.MomentID
	t.Logf("moment published: id=%d", momentID)

	// Step 2: Get moment by ID
	status, body = doAuthedRequest(t, http.MethodGet, env.baseURL,
		fmt.Sprintf("/api/v1/moment/%d", momentID), nil, u1Token)
	if status != http.StatusOK {
		t.Fatalf("get moment: status=%d body=%s", status, body)
	}

	// Step 3: Like moment (u2)
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL,
		fmt.Sprintf("/api/v1/moment/%d/like", momentID), nil, u2Token)
	if status != http.StatusOK {
		t.Fatalf("like moment: status=%d body=%s", status, body)
	}

	// Step 4: Unlike moment (u2)
	status, body = doAuthedRequest(t, http.MethodDelete, env.baseURL,
		fmt.Sprintf("/api/v1/moment/%d/like", momentID), nil, u2Token)
	if status != http.StatusOK {
		t.Fatalf("unlike moment: status=%d body=%s", status, body)
	}

	// Step 5: Comment (u2)
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL,
		fmt.Sprintf("/api/v1/moment/%d/comment", momentID),
		map[string]interface{}{"content": "nice post!"}, u2Token)
	if status != http.StatusCreated {
		t.Fatalf("comment moment: status=%d body=%s", status, body)
	}
	var commentResp struct {
		CommentID int64 `json:"comment_id"`
	}
	json.Unmarshal(body, &commentResp)
	t.Logf("comment added: id=%d", commentResp.CommentID)

	// Step 6: Get feed (u2) — wait for MQ consumer fan-out
	time.Sleep(2 * time.Second)
	status, body = doAuthedRequest(t, http.MethodGet, env.baseURL,
		"/api/v1/moment/feed?limit=10", nil, u2Token)
	if status != http.StatusOK {
		t.Fatalf("get feed: status=%d body=%s", status, body)
	}
	t.Logf("feed for u2: %s", body)

	// Step 7: Delete comment
	if commentResp.CommentID > 0 {
		status, body = doAuthedRequest(t, http.MethodDelete, env.baseURL,
			fmt.Sprintf("/api/v1/moment/comment/%d", commentResp.CommentID), nil, u2Token)
		if status != http.StatusOK {
			t.Fatalf("delete comment: status=%d body=%s", status, body)
		}
	}
}

// TestE2E_SettingsFlow tests user settings lifecycle.
func TestE2E_SettingsFlow(t *testing.T) {
	username := fmt.Sprintf("e2e_set_%d", time.Now().UnixNano())
	registerUser(t, env.baseURL, username, "pass1234")
	token := loginUser(t, env.baseURL, username, "pass1234")

	// Step 1: Get default settings
	status, body := doAuthedRequest(t, http.MethodGet, env.baseURL, "/api/v1/settings", nil, token)
	if status != http.StatusOK {
		t.Fatalf("get settings: status=%d body=%s", status, body)
	}

	// Step 2: Update settings
	status, body = doAuthedRequest(t, http.MethodPut, env.baseURL, "/api/v1/settings",
		map[string]interface{}{
			"notification_enabled": true,
			"msg_preview_enabled":  false,
			"mute_list":            "",
		}, token)
	if status != http.StatusOK {
		t.Fatalf("update settings: status=%d body=%s", status, body)
	}

	// Step 3: Mute conversation
	convID := "p_1_2"
	status, body = doAuthedRequest(t, http.MethodPost, env.baseURL, "/api/v1/settings/mute",
		map[string]interface{}{"convId": convID}, token)
	if status != http.StatusOK {
		t.Fatalf("mute conv: status=%d body=%s", status, body)
	}

	// Step 4: Unmute conversation
	status, body = doAuthedRequest(t, http.MethodDelete, env.baseURL,
		fmt.Sprintf("/api/v1/settings/mute/%s", convID), nil, token)
	if status != http.StatusOK {
		t.Fatalf("unmute conv: status=%d body=%s", status, body)
	}

	// Step 5: Verify settings
	status, body = doAuthedRequest(t, http.MethodGet, env.baseURL, "/api/v1/settings", nil, token)
	if status != http.StatusOK {
		t.Fatalf("get settings: status=%d body=%s", status, body)
	}
	t.Logf("final settings: %s", body)
}

// ── Helpers ──

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
