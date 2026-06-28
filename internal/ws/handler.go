package ws

import (
	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/service"
)

// MessageDispatcher routes WebSocket messages to appropriate service handlers
// based on the message type field in the WsMessage envelope.
type MessageDispatcher struct {
	MsgSvc    *service.MsgService
	FriendSvc *service.FriendService
	AiSvc     *service.AIService
}

// NewMessageDispatcher creates a dispatcher with references to all service handlers.
// Services that are not yet implemented can be nil — the dispatcher will skip those message types.
func NewMessageDispatcher(msgSvc *service.MsgService, friendSvc *service.FriendService, aiSvc *service.AIService) *MessageDispatcher {
	return &MessageDispatcher{
		MsgSvc:    msgSvc,
		FriendSvc: friendSvc,
		AiSvc:     aiSvc,
	}
}

// HandleMessage is the callback function passed to ReadPump.
// It decodes the raw JSON bytes into a WsMessage envelope and routes
// the message to the appropriate service handler based on msg.Type.
func (d *MessageDispatcher) HandleMessage(c *conn.ClientConnection, rawMsg []byte) {
	msg, err := DecodeMsg(rawMsg)
	if err != nil {
		// Invalid message format, skip
		return
	}

	switch msg.Type {
	case TypeMsg:
		if d.MsgSvc != nil {
			d.MsgSvc.HandleSendMessage(c.UserID, msg.Data)
		}
	case TypeDeliverAck:
		if d.MsgSvc != nil {
			d.MsgSvc.HandleDeliverAck(c.UserID, msg.Data)
		}
	case TypeReadAck:
		if d.MsgSvc != nil {
			d.MsgSvc.HandleReadAck(c.UserID, msg.Data)
		}
	case TypeSyncReq:
		if d.MsgSvc != nil {
			d.MsgSvc.HandleSyncReq(c.UserID, msg.Data)
		}
	case TypeRevokeMsg:
		if d.MsgSvc != nil {
			d.MsgSvc.HandleRevokeMsg(c.UserID, msg.Data)
		}
	case TypeFriendApply:
		if d.FriendSvc != nil {
			d.FriendSvc.HandleFriendApply(c.UserID, msg.Data)
		}
	case TypeAiStream:
		if d.AiSvc != nil {
			d.AiSvc.HandleAiStream(c.UserID, msg.Data)
		}
	case TypePing:
		// Ping is handled by the PingHandler in ReadPump; skip here
	default:
		// Unknown message type, skip
	}
}

// Callback returns a function signature matching the ReadPump msgHandler callback.
// This is convenient for passing to ServeWebSocket and ReadPump.
func (d *MessageDispatcher) Callback() func(*conn.ClientConnection, []byte) {
	return d.HandleMessage
}
