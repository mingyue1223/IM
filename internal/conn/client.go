package conn

import (
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket connection constants
const (
	maxMessageSize = 4096           // max bytes for a single message
	writeWait      = 10 * time.Second // write deadline timeout
	pongWait       = 60 * time.Second // time to wait for pong from client
	pingPeriod     = 30 * time.Second // interval between pings (must be < pongWait)
)

// ClientConnection represents a single user's WebSocket connection.
type ClientConnection struct {
	UserID   int64
	Conn     *websocket.Conn
	SendCh   chan []byte      // buffered channel for outbound messages (cap 256)
	CloseCh  chan struct{}     // signaled when connection should be closed
	LastPing time.Time         // tracks last received ping/pong for health monitoring
}

// NewClientConnection creates a new ClientConnection wrapping a websocket.Conn.
func NewClientConnection(userID int64, conn *websocket.Conn) *ClientConnection {
	return &ClientConnection{
		UserID:   userID,
		Conn:     conn,
		SendCh:   make(chan []byte, 256),
		CloseCh:  make(chan struct{}),
		LastPing: time.Now(),
	}
}

// ReadPump runs in a goroutine, reading messages from the WebSocket connection.
// It forwards each message to the msgHandler callback and handles pong/timeout.
// Exits when the connection is closed or a read error occurs.
func (c *ClientConnection) ReadPump(msgHandler func(*ClientConnection, []byte)) {
	defer c.Conn.Close()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(appData string) error {
		c.LastPing = time.Now()
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	c.Conn.SetPingHandler(func(appData string) error {
		c.LastPing = time.Now()
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		// Send pong response using WriteControl (same as default handler)
		err := c.Conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(writeWait))
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
		msgHandler(c, message)
	}
}

// WritePump runs in a goroutine, writing messages from SendCh to the WebSocket connection.
// It sends periodic ping messages and handles CloseCh for graceful shutdown.
// Exits when CloseCh is signaled, SendCh is closed, or a write error occurs.
func (c *ClientConnection) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.SendCh:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// SendCh closed — send close frame and exit
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteMessage(websocket.TextMessage, msg)
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.CloseCh:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
	}
}

// Close closes the underlying WebSocket connection.
func (c *ClientConnection) Close() {
	c.Conn.Close()
}
