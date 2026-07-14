package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── MsgOpService 错误常量 ──

const (
	ErrMsgNotRevocable   = "消息无法撤回（未找到或已过时效）"
	ErrMsgRevokeNotOwner = "仅发送者可撤回消息"
	ErrMsgDeleteFailed   = "删除消息失败"
)

// MsgOpService 处理消息操作：撤回、删除和搜索。
// RevokeMessage 使用 Redis Lua ExecRevokeMsg 进行原子撤回，并
// 通过 InsertMsgRevoked 将撤回记录持久化到 MySQL。
type MsgOpService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	logger    *zap.Logger
}

type messageHistoryStore interface {
	GetPrivateMessageHistory(ctx context.Context, userID, peerID, beforeID int64, limit int) ([]model.InboxMessage, error)
	GetGroupMessageHistory(ctx context.Context, groupID, beforeID int64, limit int) ([]model.InboxMessage, error)
}

type advancedMessageSearchStore interface {
	SearchPrivateMessagesAdvanced(ctx context.Context, userID, peerID int64, query string, startMs, endMs int64, limit, offset int) ([]model.InboxMessage, error)
	SearchGroupMessagesAdvanced(ctx context.Context, groupIDs []int64, query string, startMs, endMs int64, limit, offset int) ([]model.InboxMessage, error)
}

// NewMsgOpService creates a MsgOpService with all required dependencies.
func NewMsgOpService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, logger *zap.Logger) *MsgOpService {
	return &MsgOpService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}
}

// RevokeMessage revokes a message. It calls the Redis Lua ExecRevokeMsg script
// to atomically replace the original message in the inbox/outbox ZSet with a
// revoked placeholder (msgType=6). If the Lua script succeeds, it persists
// the revoke record to MySQL.
func (s *MsgOpService) RevokeMessage(ctx context.Context, userID int64, convID string, msgID int64) error {
	// Build the revoked placeholder message for the Lua script
	revokedMsg := model.InboxMessage{
		MsgID:     msgID,
		ConvID:    convID,
		FromID:    userID,
		MsgType:   model.MsgTypeRevoked,
		Content:   "This message has been revoked",
		Timestamp: time.Now().UnixMilli(),
	}
	revokedJSON, err := json.Marshal(revokedMsg)
	if err != nil {
		return fmt.Errorf("marshal revoked message: %w", err)
	}

	// Execute the Redis Lua script
	success, err := s.redisRepo.ExecRevokeMsg(ctx, userID, convID, msgID, string(revokedJSON), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("exec revoke lua: %w", err)
	}
	if !success {
		return fmt.Errorf(ErrMsgNotRevocable)
	}

	// Persist the revoke record to MySQL
	revoked := &model.MsgRevoked{
		MsgID:      msgID,
		ConvID:     convID,
		OperatorID: userID,
		RevokedAt:  time.Now(),
	}
	if err := s.mysqlRepo.InsertMsgRevoked(ctx, revoked); err != nil {
		s.logger.Error("failed to persist revoke record to MySQL",
			zap.Int64("msgID", msgID),
			zap.String("convID", convID),
			zap.Error(err),
		)
		// Redis already succeeded, so we don't fail the request; just log the error
	}

	s.logger.Debug("message revoked",
		zap.Int64("userID", userID),
		zap.String("convID", convID),
		zap.Int64("msgID", msgID),
	)

	return nil
}

// DeleteMessage soft-deletes a message by marking it as deleted in Redis.
// The message is not actually removed from the inbox/outbox; instead a
// deleted placeholder replaces it.
func (s *MsgOpService) DeleteMessage(ctx context.Context, userID int64, convID string, msgID int64) error {
	// Soft delete: mark as deleted by replacing the message with a placeholder
	deletedMsg := model.InboxMessage{
		MsgID:     msgID,
		ConvID:    convID,
		FromID:    userID,
		MsgType:   model.MsgTypeSystem,
		Content:   "This message has been deleted",
		Timestamp: time.Now().UnixMilli(),
	}
	deletedJSON, err := json.Marshal(deletedMsg)
	if err != nil {
		return fmt.Errorf(ErrMsgDeleteFailed+": %w", err)
	}

	// Use the same ExecRevokeMsg Lua to atomically swap the message
	success, err := s.redisRepo.ExecRevokeMsg(ctx, userID, convID, msgID, string(deletedJSON), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf(ErrMsgDeleteFailed+": %w", err)
	}
	if !success {
		return fmt.Errorf(ErrMsgDeleteFailed)
	}

	s.logger.Debug("message deleted (soft)",
		zap.Int64("userID", userID),
		zap.String("convID", convID),
		zap.Int64("msgID", msgID),
	)

	return nil
}

// SearchMessages searches private messages by content for the given user.
func (s *MsgOpService) SearchMessages(ctx context.Context, userID int64, query string, limit, offset int) ([]model.PrivateMessage, error) {
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	msgs, err := s.mysqlRepo.SearchPrivateMessages(ctx, userID, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search private messages: %w", err)
	}

	return msgs, nil
}

func (s *MsgOpService) GetMessageHistory(ctx context.Context, userID int64, convID string, beforeID int64, limit int) ([]model.InboxMessage, bool, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	store, ok := s.mysqlRepo.(messageHistoryStore)
	if !ok {
		return nil, false, fmt.Errorf("message history storage is unavailable")
	}
	convType, targetID, err := s.authorizeConversation(ctx, userID, convID)
	if err != nil {
		return nil, false, err
	}
	var items []model.InboxMessage
	if convType == model.ConvTypePrivate {
		items, err = store.GetPrivateMessageHistory(ctx, userID, targetID, beforeID, limit+1)
	} else {
		items, err = store.GetGroupMessageHistory(ctx, targetID, beforeID, limit+1)
	}
	if err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, hasMore, nil
}

func (s *MsgOpService) SearchMessagesAdvanced(ctx context.Context, userID int64, query, convID string, startMs, endMs int64, limit, offset int) ([]model.InboxMessage, error) {
	query = strings.TrimSpace(query)
	if query == "" && startMs <= 0 && endMs <= 0 {
		return nil, fmt.Errorf("query or time range is required")
	}
	if startMs > 0 && endMs > 0 && startMs > endMs {
		return nil, fmt.Errorf("startTime must not be after endTime")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	store, ok := s.mysqlRepo.(advancedMessageSearchStore)
	if !ok {
		return nil, fmt.Errorf("message search storage is unavailable")
	}

	peerID := int64(0)
	groupIDs := []int64(nil)
	includePrivate := true
	includeGroups := true
	if convID != "" {
		convType, targetID, err := s.authorizeConversation(ctx, userID, convID)
		if err != nil {
			return nil, err
		}
		if convType == model.ConvTypePrivate {
			peerID = targetID
			includeGroups = false
		} else {
			groupIDs = []int64{targetID}
			includePrivate = false
		}
	} else {
		var err error
		groupIDs, err = s.redisRepo.GetGroupMemberships(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get group memberships: %w", err)
		}
	}

	fetchLimit := limit + offset
	items := make([]model.InboxMessage, 0, fetchLimit*2)
	if includePrivate {
		privateItems, err := store.SearchPrivateMessagesAdvanced(ctx, userID, peerID, query, startMs, endMs, fetchLimit, 0)
		if err != nil {
			return nil, err
		}
		items = append(items, privateItems...)
	}
	if includeGroups && len(groupIDs) > 0 {
		groupItems, err := store.SearchGroupMessagesAdvanced(ctx, groupIDs, query, startMs, endMs, fetchLimit, 0)
		if err != nil {
			return nil, err
		}
		items = append(items, groupItems...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Timestamp != items[j].Timestamp {
			return items[i].Timestamp > items[j].Timestamp
		}
		return items[i].MsgID > items[j].MsgID
	})
	if offset >= len(items) {
		return []model.InboxMessage{}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}

func (s *MsgOpService) authorizeConversation(ctx context.Context, userID int64, convID string) (int, int64, error) {
	parts := strings.Split(convID, "_")
	if len(parts) == 3 && parts[0] == "p" {
		first, err1 := strconv.ParseInt(parts[1], 10, 64)
		second, err2 := strconv.ParseInt(parts[2], 10, 64)
		if err1 != nil || err2 != nil || (first != userID && second != userID) {
			return 0, 0, fmt.Errorf("conversation access denied")
		}
		peerID := first
		if peerID == userID {
			peerID = second
		}
		return model.ConvTypePrivate, peerID, nil
	}
	if len(parts) == 2 && parts[0] == "g" {
		groupID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid conversation id")
		}
		memberships, err := s.redisRepo.GetGroupMemberships(ctx, userID)
		if err != nil {
			return 0, 0, err
		}
		for _, membership := range memberships {
			if membership == groupID {
				return model.ConvTypeGroup, groupID, nil
			}
		}
		return 0, 0, fmt.Errorf("conversation access denied")
	}
	return 0, 0, fmt.Errorf("invalid conversation id")
}
