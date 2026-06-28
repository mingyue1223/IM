package conn

import (
	"encoding/json"
	"sync"
)

// ConnectionManager manages active WebSocket connections keyed by userID.
// Uses sync.Map for safe concurrent access.
type ConnectionManager struct {
	connections sync.Map // userID (int64) -> *ClientConnection
}

// NewConnectionManager creates a new ConnectionManager.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{}
}

// Register stores a client connection under the given userID.
func (cm *ConnectionManager) Register(userID int64, client *ClientConnection) {
	cm.connections.Store(userID, client)
}

// Get retrieves a client connection by userID.
// Returns the connection and true if found, nil and false otherwise.
func (cm *ConnectionManager) Get(userID int64) (*ClientConnection, bool) {
	val, ok := cm.connections.Load(userID)
	if !ok {
		return nil, false
	}
	return val.(*ClientConnection), true
}

// Delete removes a client connection by userID.
func (cm *ConnectionManager) Delete(userID int64) {
	cm.connections.Delete(userID)
}

// KickOld kicks an existing connection for the given userID and registers the new one.
// Sends a kick JSON message {"type":"kick","reason":"new_login"} to the old connection's SendCh,
// closes its CloseCh, then registers the new client.
// If no existing connection exists, just registers the new one.
func (cm *ConnectionManager) KickOld(userID int64, newClient *ClientConnection) {
	old, ok := cm.Get(userID)
	if ok {
		// Send kick message to old connection
		kickMsg, _ := json.Marshal(map[string]string{"type": "kick", "reason": "new_login"})
		select {
		case old.SendCh <- kickMsg:
		default: // buffer full, drop message — will still close
		}
		close(old.CloseCh)
		cm.Delete(userID)
	}
	cm.Register(userID, newClient)
}
