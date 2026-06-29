package ws

// Protocol types and encoding/decoding functions have been moved to
// internal/protocol to break the import cycle between ws and service.
// This file re-exports them for backward compatibility within the ws package.

import (
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/protocol"
)

// Re-export Type constants for backward compatibility within ws package.
const (
	TypeMsg            = protocol.TypeMsg
	TypeServerAck      = protocol.TypeServerAck
	TypeDeliverAck     = protocol.TypeDeliverAck
	TypeReadAck        = protocol.TypeReadAck
	TypeSyncReq        = protocol.TypeSyncReq
	TypeSyncBatch      = protocol.TypeSyncBatch
	TypeConvSync       = protocol.TypeConvSync
	TypeRevokeMsg      = protocol.TypeRevokeMsg
	TypeMsgRevoked     = protocol.TypeMsgRevoked
	TypeKick           = protocol.TypeKick
	TypeAiStream       = protocol.TypeAiStream
	TypeFriendApply    = protocol.TypeFriendApply
	TypeFriendAccepted = protocol.TypeFriendAccepted
	TypePresence       = protocol.TypePresence
	TypeError          = protocol.TypeError
	TypePing           = protocol.TypePing
	TypePong           = protocol.TypePong
)

// EncodeMsg delegates to protocol.EncodeMsg.
func EncodeMsg(msgType string, data interface{}) ([]byte, error) {
	return protocol.EncodeMsg(msgType, data)
}

// DecodeMsg delegates to protocol.DecodeMsg.
func DecodeMsg(raw []byte) (*model.WsMessage, error) {
	return protocol.DecodeMsg(raw)
}
