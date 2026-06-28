package ws

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/middleware"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// upgrader upgrades HTTP connections to WebSocket.
// CheckOrigin allows all origins for development purposes.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// ServeWebSocket returns a Gin handler that:
//  1. Validates JWT token from query parameter (?token=...)
//  2. Upgrades the HTTP connection to WebSocket
//  3. Creates a ClientConnection
//  4. Kicks any existing connection for the same user (single-device policy)
//  5. Updates Redis online status (online:{userID} TTL=60s, conn:{userID} device info)
//  6. Starts ReadPump and WritePump goroutines
func ServeWebSocket(jwtSecret string, rdb *redis.Client, cm *conn.ConnectionManager, msgHandler func(*conn.ClientConnection, []byte)) gin.HandlerFunc {
	return func(c *gin.Context) {
		// JWT auth — token comes from query parameter for WS
		token := c.Query("token")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		claims := &middleware.Claims{}
		parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})
		if err != nil || !parsedToken.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// Upgrade to WebSocket
		wsConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("ws upgrade error: %v", err)
			return
		}

		// Create ClientConnection
		client := conn.NewClientConnection(claims.UserID, wsConn)

		// Kick old connection for this user (single-device policy)
		cm.KickOld(claims.UserID, client)

		// Update Redis online status
		ctx := context.Background()
		onlineKey := fmt.Sprintf("online:%d", claims.UserID)
		connKey := fmt.Sprintf("conn:%d", claims.UserID)
		rdb.Set(ctx, onlineKey, "1", 60*time.Second)
		rdb.Set(ctx, connKey, fmt.Sprintf("ws:%d", time.Now().UnixNano()), 0)

		log.Printf("user %d (%s) connected via WebSocket", claims.UserID, claims.Username)

		// Start pumps
		go client.WritePump()
		go client.ReadPump(msgHandler)
	}
}
