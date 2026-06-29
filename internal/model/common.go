package model

import (
	"encoding/json"
	"fmt"
)

// Message type constants
const (
	MsgTypeText    = 1
	MsgTypeImage   = 2
	MsgTypeVideo   = 3
	MsgTypeAI      = 4
	MsgTypeSystem  = 5
	MsgTypeRevoked = 6

	ConvTypePrivate = 1
	ConvTypeGroup   = 2

	RoleMember = 0
	RoleAdmin  = 1
	RoleOwner  = 2

	AI_SYSTEM_ID = 0
)

// BuildConvID generates conversation ID:
//   private: p_{smallerID}_{largerID}
//   group:   g_{groupID}
func BuildConvID(convType int, id1, id2 int64) string {
	if convType == ConvTypeGroup {
		return fmt.Sprintf("g_%d", id1)
	}
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	return fmt.Sprintf("p_%d_%d", id1, id2)
}

// WsMessage — universal WebSocket message envelope
type WsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// WsError — error payload sent via WebSocket
type WsError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
