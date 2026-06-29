package service

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	redislua "github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/protocol"
	"github.com/goim/goim/internal/repository"
)

// ── Error messages for Lua check results ──

const (
	ErrNotFriend  = "not friends with receiver"
	ErrBlocked    = "receiver has blocked you"
	ErrDuplicate  = "duplicate message"
	ErrNotMember  = "not a member of this group"
	ErrMuted      = "you are muted in this group"
	ErrRevokeFail = "cannot revoke this message"
)

// MsgService handles all message-related WebSocket operations: send,
// deliver ack, read ack, offline sync, and message revoke.
type MsgService struct {
	redisRepo repository.RedisRepo
	mqRepo    repository.MQRepo
	cm        *conn.ConnectionManager
	logger    *zap.Logger
}

// NewMsgService creates a MsgService with all required dependencies.
func NewMsgService(redisRepo repository.RedisRepo, mqRepo repository.MQRepo, cm *conn.ConnectionManager, logger *zap.Logger) *MsgService {
	return &MsgService{
		redisRepo: redisRepo,
		mqRepo:    mqRepo,
		cm:        cm,
		logger:    logger,
	}
}

// ──────────────────────────────────────────────────────
// HandleSendMessage
// ──────────────────────────────────────────────────────

// HandleSendMessage processes an incoming chat message from a client.
// Private chat (convType=1) uses the inbox push model.
// Group chat (convType=2) uses the outbox pull model.
func (s *MsgService) HandleSendMessage(userID int64, data []byte) {
	var req model.SendMessage
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("failed to parse SendMessage", zap.Error(err))
		s.sendError(userID, 400, "invalid message format")
		return
	}

	ctx := context.Background()

	switch req.ConvType {
	case model.ConvTypePrivate:
		s.handlePrivateMsg(ctx, userID, &req)
	case model.ConvTypeGroup:
		s.handleGroupMsg(ctx, userID, &req)
	default:
		s.sendError(userID, 400, "unknown convType")
	}
}

// handlePrivateMsg processes a private (1-to-1) message.
// Flow: Lua check → build InboxMessage → publish MQ → send serverAck
func (s *MsgService) handlePrivateMsg(ctx context.Context, senderID int64, req *model.SendMessage) {
	checkResult, err := s.redisRepo.ExecPrivateMsgCheck(ctx, senderID, req.ToID, req.ClientMsgID)
	if err != nil {
		s.logger.Error("ExecPrivateMsgCheck failed", zap.Error(err))
		s.sendError(senderID, 500, "internal server error")
		return
	}

	switch checkResult.ErrCode {
	case redislua.PMErrOK:
		// proceed below
	case redislua.PMErrNotFriend:
		s.sendError(senderID, redislua.PMErrNotFriend, ErrNotFriend)
		return
	case redislua.PMErrBlocked:
		s.sendError(senderID, redislua.PMErrBlocked, ErrBlocked)
		return
	case redislua.PMErrDuplicate:
		s.sendError(senderID, redislua.PMErrDuplicate, ErrDuplicate)
		return
	default:
		s.sendError(senderID, checkResult.ErrCode, "unknown error")
		return
	}

	// Build conversation ID
	convID := model.BuildConvID(model.ConvTypePrivate, senderID, req.ToID)

	// Build InboxMessage (will be persisted to inbox by MQ consumer)
	inboxMsg := model.InboxMessage{
		MsgID:      checkResult.MsgID,
		ConvID:     convID,
		ConvType:   model.ConvTypePrivate,
		FromID:     senderID,
		ToID:       req.ToID,
		MsgType:    req.MsgType,
		Content:    req.Content,
		ReadStatus: 0,
		Timestamp:  req.Timestamp,
	}

	// Build PrivateMessage for MQ publish
	pm := &model.PrivateMessage{
		ID:         checkResult.MsgID,
		SenderID:   senderID,
		ReceiverID: req.ToID,
		Content:    req.Content,
		MsgType:    req.MsgType,
		CreatedAt:  time.UnixMilli(req.Timestamp),
	}

	// Publish to MQ
	if err := s.mqRepo.PublishPrivateMsg(ctx, pm); err != nil {
		s.logger.Error("PublishPrivateMsg failed", zap.Error(err))
		s.sendError(senderID, 500, "message publish failed")
		return
	}

	s.logger.Debug("private message published",
		zap.Int64("msgID", checkResult.MsgID),
		zap.Int64("senderID", senderID),
		zap.Int64("receiverID", req.ToID),
	)

	// Send serverAck to sender
	ack := &model.ServerAck{
		ClientMsgID: req.ClientMsgID,
		ServerMsgID: checkResult.MsgID,
		Timestamp:   time.Now().UnixMilli(),
	}
	s.pushToUser(senderID, protocol.TypeServerAck, ack)

	// If receiver is online, push the InboxMessage directly for real-time delivery
	if checkResult.IsOnline {
		s.pushToUser(req.ToID, protocol.TypeMsg, inboxMsg)
	}
}

// handleGroupMsg processes a group message.
// Flow: Lua check → build InboxMessage → publish MQ → send serverAck with groupSeq
func (s *MsgService) handleGroupMsg(ctx context.Context, senderID int64, req *model.SendMessage) {
	checkResult, err := s.redisRepo.ExecGroupMsgCheck(ctx, req.ToID, senderID, req.ClientMsgID)
	if err != nil {
		s.logger.Error("ExecGroupMsgCheck failed", zap.Error(err))
		s.sendError(senderID, 500, "internal server error")
		return
	}

	switch checkResult.ErrCode {
	case redislua.GMErrOK:
		// proceed below
	case redislua.GMErrNotMember:
		s.sendError(senderID, redislua.GMErrNotMember, ErrNotMember)
		return
	case redislua.GMErrMuted:
		s.sendError(senderID, redislua.GMErrMuted, ErrMuted)
		return
	case redislua.GMErrDuplicate:
		s.sendError(senderID, redislua.GMErrDuplicate, ErrDuplicate)
		return
	default:
		s.sendError(senderID, checkResult.ErrCode, "unknown error")
		return
	}

	// Build GroupMessage for MQ publish
	gm := &model.GroupMessage{
		ID:        checkResult.MsgID,
		GroupID:   req.ToID,
		SenderID:  senderID,
		Content:   req.Content,
		MsgType:   req.MsgType,
		GroupSeq:  checkResult.GroupSeq,
		CreatedAt: time.UnixMilli(req.Timestamp),
	}

	// Publish to MQ
	if err := s.mqRepo.PublishGroupMsg(ctx, gm); err != nil {
		s.logger.Error("PublishGroupMsg failed", zap.Error(err))
		s.sendError(senderID, 500, "message publish failed")
		return
	}

	s.logger.Debug("group message published",
		zap.Int64("msgID", checkResult.MsgID),
		zap.Int64("groupID", req.ToID),
		zap.Int64("groupSeq", checkResult.GroupSeq),
	)

	// Send serverAck to sender (includes groupSeq)
	ack := &model.ServerAck{
		ClientMsgID: req.ClientMsgID,
		ServerMsgID: checkResult.MsgID,
		GroupSeq:    checkResult.GroupSeq,
		Timestamp:   time.Now().UnixMilli(),
	}
	s.pushToUser(senderID, protocol.TypeServerAck, ack)
}

// ──────────────────────────────────────────────────────
// HandleDeliverAck
// ──────────────────────────────────────────────────────

// HandleDeliverAck processes a delivery acknowledgement from a client.
// This is informational — the client confirms it received a message.
func (s *MsgService) HandleDeliverAck(userID int64, data []byte) {
	var ack model.DeliverAck
	if err := json.Unmarshal(data, &ack); err != nil {
		s.logger.Warn("failed to parse DeliverAck", zap.Error(err))
		return
	}

	s.logger.Debug("delivery ack received",
		zap.Int64("userID", userID),
		zap.Int64("serverMsgID", ack.ServerMsgID),
	)
}

// ──────────────────────────────────────────────────────
// HandleReadAck
// ──────────────────────────────────────────────────────

// HandleReadAck processes a read acknowledgement from a client.
// It atomically marks all unread messages in the specified conversation as read
// and clears the unread counter via Lua script.
func (s *MsgService) HandleReadAck(userID int64, data []byte) {
	var req model.ReadAck
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("failed to parse ReadAck", zap.Error(err))
		s.sendError(userID, 400, "invalid readAck format")
		return
	}

	ctx := context.Background()

	count, err := s.redisRepo.ExecInboxMarkRead(ctx, userID, req.ConvID)
	if err != nil {
		s.logger.Error("ExecInboxMarkRead failed",
			zap.Int64("userID", userID),
			zap.String("convID", req.ConvID),
			zap.Error(err),
		)
		s.sendError(userID, 500, "mark read failed")
		return
	}

	s.logger.Debug("inbox marked read",
		zap.Int64("userID", userID),
		zap.String("convID", req.ConvID),
		zap.Int64("count", count),
	)
}

// ──────────────────────────────────────────────────────
// HandleSyncReq
// ──────────────────────────────────────────────────────

// HandleSyncReq processes an offline sync request from a client.
// It pulls new messages from the user's inbox (private) and group outboxes,
// merges and sorts them, then pushes a SyncBatch and ConvSync via WebSocket.
func (s *MsgService) HandleSyncReq(userID int64, data []byte) {
	var req model.SyncReq
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("failed to parse SyncReq", zap.Error(err))
		s.sendError(userID, 400, "invalid syncReq format")
		return
	}

	if req.BatchSize <= 0 {
		req.BatchSize = 50 // default batch size
	}

	ctx := context.Background()

	// ── 1. Pull private messages from inbox ──
	privateMsgs, err := s.redisRepo.ReadInbox(ctx, userID, req.LastSyncTime, req.BatchSize+1)
	if err != nil {
		s.logger.Error("ReadInbox failed", zap.Error(err))
		s.sendError(userID, 500, "sync failed")
		return
	}

	// ── 2. Pull group messages from group outboxes ──
	groupIDs, err := s.redisRepo.GetGroupMemberships(ctx, userID)
	if err != nil {
		s.logger.Error("GetGroupMemberships failed", zap.Error(err))
		// Continue with just private messages
	}

	var groupMsgs []model.InboxMessage
	for _, gid := range groupIDs {
		convID := model.BuildConvID(model.ConvTypeGroup, gid, 0)
		lastReadSeq, _ := s.redisRepo.GetGroupReadPos(ctx, userID, convID)

		outboxMsgs, err := s.redisRepo.ReadOutbox(ctx, gid, req.LastSyncTime, req.BatchSize+1)
		if err != nil {
			s.logger.Warn("ReadOutbox failed", zap.Int64("groupID", gid), zap.Error(err))
			continue
		}

		for _, m := range outboxMsgs {
			if m.GroupSeq > lastReadSeq {
				groupMsgs = append(groupMsgs, m)
			}
		}
	}

	// ── 3. Merge and sort all messages ──
	allMsgs := append(privateMsgs, groupMsgs...)
	sort.Slice(allMsgs, func(i, j int) bool {
		return allMsgs[i].Timestamp < allMsgs[j].Timestamp
	})

	// ── 4. Determine hasMore ──
	hasMore := len(allMsgs) > req.BatchSize
	if hasMore {
		allMsgs = allMsgs[:req.BatchSize]
	}

	// ── 5. Determine syncTime ──
	var syncTime int64
	if len(allMsgs) > 0 {
		syncTime = allMsgs[len(allMsgs)-1].Timestamp
	} else {
		syncTime = time.Now().UnixMilli()
	}

	// ── 6. Push SyncBatch ──
	batch := &model.SyncBatch{
		Messages: allMsgs,
		HasMore:  hasMore,
		SyncTime: syncTime,
	}
	s.pushToUser(userID, protocol.TypeSyncBatch, batch)

	// ── 7. Update group read positions ──
	for _, gid := range groupIDs {
		convID := model.BuildConvID(model.ConvTypeGroup, gid, 0)
		maxSeq := int64(0)
		for _, m := range groupMsgs {
			if m.ConvID == convID && m.GroupSeq > maxSeq {
				maxSeq = m.GroupSeq
			}
		}
		if maxSeq > 0 {
			if err := s.redisRepo.SetGroupReadPos(ctx, userID, convID, maxSeq); err != nil {
				s.logger.Warn("SetGroupReadPos failed", zap.Error(err))
			}
		}
	}

	// ── 8. Push ConvSync ──
	convList, err := s.redisRepo.GetConvList(ctx, userID)
	if err != nil {
		s.logger.Warn("GetConvList failed", zap.Error(err))
	}

	unreadMap, err := s.redisRepo.GetUnreadMap(ctx, userID)
	if err != nil {
		s.logger.Warn("GetUnreadMap failed", zap.Error(err))
	}

	convSync := &model.ConvSync{
		Conversations: convList,
		UnreadMap:     unreadMap,
	}
	s.pushToUser(userID, protocol.TypeConvSync, convSync)

	s.logger.Debug("sync completed",
		zap.Int64("userID", userID),
		zap.Int("msgCount", len(allMsgs)),
		zap.Bool("hasMore", hasMore),
	)
}

// ──────────────────────────────────────────────────────
// HandleRevokeMsg
// ──────────────────────────────────────────────────────

// HandleRevokeMsg processes a message revoke request from a client.
// It atomically replaces the original message with a "revoked" version
// (msgType=6) in the inbox/outbox ZSet via Lua script, then pushes a
// msgRevoked notification to the other party.
func (s *MsgService) HandleRevokeMsg(userID int64, data []byte) {
	var req model.RevokeMsgReq
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("failed to parse RevokeMsgReq", zap.Error(err))
		s.sendError(userID, 400, "invalid revokeMsg format")
		return
	}

	ctx := context.Background()

	// Determine convType from convID prefix
	var convType int
	if strings.HasPrefix(req.ConvID, "p_") {
		convType = model.ConvTypePrivate
	} else if strings.HasPrefix(req.ConvID, "g_") {
		convType = model.ConvTypeGroup
	} else {
		s.sendError(userID, 400, "invalid convId format")
		return
	}

	// Build the revoked replacement message
	now := time.Now().UnixMilli()
	revokedMsg := model.InboxMessage{
		MsgID:     req.ServerMsgID,
		ConvID:    req.ConvID,
		ConvType:  convType,
		FromID:    userID,
		MsgType:   model.MsgTypeRevoked,
		Content:   "Message revoked",
		Timestamp: now,
	}
	revokeMsgJSON, err := json.Marshal(revokedMsg)
	if err != nil {
		s.logger.Error("marshal revoked message failed", zap.Error(err))
		s.sendError(userID, 500, "internal server error")
		return
	}

	// Execute Lua revoke script
	ok, err := s.redisRepo.ExecRevokeMsg(ctx, userID, req.ConvID, req.ServerMsgID, string(revokeMsgJSON), now)
	if err != nil {
		s.logger.Error("ExecRevokeMsg failed", zap.Error(err))
		s.sendError(userID, 500, "revoke failed")
		return
	}

	if !ok {
		s.sendError(userID, 403, ErrRevokeFail)
		return
	}

	s.logger.Debug("message revoked",
		zap.Int64("userID", userID),
		zap.String("convID", req.ConvID),
		zap.Int64("msgID", req.ServerMsgID),
	)

	// Push msgRevoked notification
	notification := &model.RevokedNotification{
		ConvID:      req.ConvID,
		ServerMsgID: req.ServerMsgID,
		OperatorID:  userID,
	}

	// Notify sender for confirmation
	s.pushToUser(userID, protocol.TypeMsgRevoked, notification)

	// For private chat, push to the other party
	if convType == model.ConvTypePrivate {
		otherID := getOtherPartyID(req.ConvID, userID)
		if otherID > 0 {
			s.pushToUser(otherID, protocol.TypeMsgRevoked, notification)
		}
	}
	// For group chat, members discover revoke via sync or MQ consumer fan-out
}

// ──────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────

// sendError pushes a WsError to the given user via WebSocket.
func (s *MsgService) sendError(userID int64, code int, message string) {
	wsErr := &model.WsError{Code: code, Message: message}
	s.pushToUser(userID, protocol.TypeError, wsErr)
}

// pushToUser encodes a message and pushes it to the user's WebSocket SendCh.
// If the user is not online, the message is silently dropped (they'll get it via sync).
func (s *MsgService) pushToUser(userID int64, msgType string, data interface{}) {
	encoded, err := protocol.EncodeMsg(msgType, data)
	if err != nil {
		s.logger.Error("EncodeMsg failed", zap.String("type", msgType), zap.Error(err))
		return
	}

	client, ok := s.cm.Get(userID)
	if !ok {
		// User not online — they'll get it via sync
		return
	}

	select {
	case client.SendCh <- encoded:
		// sent successfully
	default:
		// buffer full — drop message (user will sync later)
		s.logger.Warn("SendCh buffer full, dropping message",
			zap.Int64("userID", userID),
			zap.String("type", msgType),
		)
	}
}

// getOtherPartyID extracts the other user's ID from a private conversation ID.
// convID format for private: "p_{smallerID}_{largerID}"
func getOtherPartyID(convID string, userID int64) int64 {
	if !strings.HasPrefix(convID, "p_") {
		return 0
	}
	parts := strings.Split(convID[2:], "_")
	if len(parts) != 2 {
		return 0
	}
	id1, err1 := strconv.ParseInt(parts[0], 10, 64)
	id2, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	if id1 == userID {
		return id2
	}
	if id2 == userID {
		return id1
	}
	return 0
}
