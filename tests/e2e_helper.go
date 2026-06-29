//go:build e2e

// Package e2e contains end-to-end integration tests for the GoIM server.
// These tests exercise the full stack: HTTP API, WebSocket, MQ consumers,
// Redis, MySQL, and RabbitMQ. They require Docker services to be running
// (as configured in configs/config.test.yaml).
//
// Run with: go test ./tests/... -v -tags e2e -timeout 120s
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ── Response DTOs ──

type authRegisterResp struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type authLoginResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type authRefreshResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type friendRequestResp struct {
	RequestID  int64 `json:"request_id"`
	FromUserID int64 `json:"from_user_id"`
	ToUserID   int64 `json:"to_user_id"`
	Status     int   `json:"status"`
}

type acceptFriendResp struct {
	UserID   int64 `json:"user_id"`
	FriendID int64 `json:"friend_id"`
}

type createGroupResp struct {
	GroupID int64 `json:"group_id"`
}

type publishMomentResp struct {
	MomentID int64 `json:"moment_id"`
}

type wsEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type serverAckData struct {
	ClientMsgID string `json:"clientMsgId"`
	ServerMsgID int64  `json:"serverMsgId"`
	GroupSeq    int64  `json:"groupSeq,omitempty"`
	Timestamp   int64  `json:"timestamp"`
}

type sendMsgData struct {
	ClientMsgID string `json:"msgId"`
	ConvType    int    `json:"convType"`
	ToID        int64  `json:"toId"`
	MsgType     int    `json:"msgType"`
	Content     string `json:"content"`
	Timestamp   int64  `json:"timestamp"`
}

// ── HTTP helpers ──

// doRequest performs an HTTP request and returns the response body + status code.
func doRequest(t *testing.T, method, baseURL, path string, body interface{}, token string) (statusCode int, responseBody []byte) {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	statusCode = resp.StatusCode
	responseBody, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return statusCode, responseBody
}

// doAuthedRequest performs an authenticated HTTP request.
func doAuthedRequest(t *testing.T, method, baseURL, path string, body interface{}, token string) (int, []byte) {
	t.Helper()
	return doRequest(t, method, baseURL, path, body, token)
}

// registerUser registers a new user and returns userID.
func registerUser(t *testing.T, baseURL, username, password string) int64 {
	t.Helper()
	status, body := doRequest(t, http.MethodPost, baseURL, "/api/v1/auth/register",
		map[string]string{"username": username, "password": password}, "")
	if status != http.StatusCreated {
		t.Fatalf("register user: status=%d body=%s", status, body)
	}
	var resp authRegisterResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	return resp.UserID
}

// loginUser logs in a user and returns the access token.
func loginUser(t *testing.T, baseURL, username, password string) string {
	t.Helper()
	status, body := doRequest(t, http.MethodPost, baseURL, "/api/v1/auth/login",
		map[string]string{"username": username, "password": password}, "")
	if status != http.StatusOK {
		t.Fatalf("login user: status=%d body=%s", status, body)
	}
	var resp authLoginResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	return resp.AccessToken
}

// loginUserFull logs in a user and returns full login response (access + refresh tokens).
func loginUserFull(t *testing.T, baseURL, username, password string) authLoginResp {
	t.Helper()
	status, body := doRequest(t, http.MethodPost, baseURL, "/api/v1/auth/login",
		map[string]string{"username": username, "password": password}, "")
	if status != http.StatusOK {
		t.Fatalf("login user: status=%d body=%s", status, body)
	}
	var resp authLoginResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	return resp
}

// refreshToken refreshes an access token using a refresh token.
func refreshToken(t *testing.T, baseURL, refreshTokenStr string) authRefreshResp {
	t.Helper()
	status, body := doRequest(t, http.MethodPost, baseURL, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refreshTokenStr}, "")
	if status != http.StatusOK {
		t.Fatalf("refresh token: status=%d body=%s", status, body)
	}
	var resp authRefreshResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal refresh response: %v", err)
	}
	return resp
}

// ── WebSocket helpers ──

// connectWS establishes a WebSocket connection with the given JWT token.
func connectWS(t *testing.T, baseURL, token string) *websocket.Conn {
	t.Helper()

	// Convert http:// to ws://
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	u, err := url.Parse(wsURL + "/ws")
	if err != nil {
		t.Fatalf("parse ws url: %v", err)
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	return conn
}

// sendWSMessage sends a typed WebSocket message envelope.
func sendWSMessage(t *testing.T, conn *websocket.Conn, msgType string, data interface{}) {
	t.Helper()
	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal ws data: %v", err)
	}
	envelope := wsEnvelope{Type: msgType, Data: dataBytes}
	msg, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal ws envelope: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("write ws message: %v", err)
	}
}

// readWSMessage reads a WebSocket message envelope with timeout.
func readWSMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) wsEnvelope {
	t.Helper()
	msgChan := make(chan wsEnvelope, 1)
	errChan := make(chan error, 1)

	go func() {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			errChan <- err
			return
		}
		var envelope wsEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			errChan <- err
			return
		}
		msgChan <- envelope
	}()

	select {
	case msg := <-msgChan:
		return msg
	case err := <-errChan:
		t.Fatalf("read ws message: %v", err)
		return wsEnvelope{}
	case <-time.After(timeout):
		t.Fatalf("read ws message: timeout after %v", timeout)
		return wsEnvelope{}
	}
}

// readWSMessageType reads a WS message and filters by expected type.
// Returns only when a message of the expected type is received, skipping
// other types (e.g., pong, kick).
func readWSMessageType(t *testing.T, conn *websocket.Conn, expectedType string, timeout time.Duration) wsEnvelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg := readWSMessage(t, conn, time.Until(deadline))
		if msg.Type == expectedType {
			return msg
		}
		// Skip non-target messages (pong, presence, etc.)
	}
	t.Fatalf("read ws message type %s: timeout after %v", expectedType, timeout)
	return wsEnvelope{}
}

// closeWS closes a WebSocket connection gracefully.
func closeWS(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
		// Connection may already be closed
		return
	}
	conn.Close()
}
