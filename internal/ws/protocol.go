package ws

import (
	"encoding/json"

	"github.com/goim/goim/internal/model"
)

// WsMessage type constants — all possible "type" values in the WebSocket envelope.
const (
	TypeMsg            = "msg"
	TypeServerAck      = "serverAck"
	TypeDeliverAck     = "deliverAck"
	TypeReadAck        = "readAck"
	TypeSyncReq        = "syncReq"
	TypeSyncBatch      = "syncBatch"
	TypeConvSync       = "convSync"
	TypeRevokeMsg      = "revokeMsg"
	TypeMsgRevoked     = "msgRevoked"
	TypeKick           = "kick"
	TypeAiStream       = "aiStream"
	TypeFriendApply    = "friendApply"
	TypeFriendAccepted = "friendAccepted"
	TypePresence       = "presence"
	TypeError          = "error"
	TypePing           = "ping"
	TypePong           = "pong"
)

// EncodeMsg builds a WsMessage envelope and serializes it to JSON bytes.
// msgType is one of the Type* constants; data is the payload to embed in "data".
func EncodeMsg(msgType string, data interface{}) ([]byte, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	envelope := model.WsMessage{Type: msgType, Data: dataBytes}
	return json.Marshal(envelope)
}

// DecodeMsg parses raw JSON bytes into a WsMessage envelope.
// The caller can then inspect msg.Type and further unmarshal msg.Data.
func DecodeMsg(raw []byte) (*model.WsMessage, error) {
	var msg model.WsMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
