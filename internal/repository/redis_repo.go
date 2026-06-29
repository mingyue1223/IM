package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/redis"
)

// RedisRepo defines all Redis operations needed by the message service
// and downstream MQ consumers. The interface allows mocking in tests.
type RedisRepo interface {
	// ── Inbox / Outbox ──
	WriteInbox(ctx context.Context, userID int64, msg *model.InboxMessage) error
	WriteOutbox(ctx context.Context, groupID int64, msg *model.InboxMessage) error
	ReadInbox(ctx context.Context, userID int64, lastSyncTime int64, batchSize int) ([]model.InboxMessage, error)
	ReadOutbox(ctx context.Context, groupID int64, lastSyncTime int64, limit int) ([]model.InboxMessage, error)

	// ── Conversation list ──
	UpdateConvList(ctx context.Context, userID int64, convID string, summary string, timestamp int64) error
	GetConvList(ctx context.Context, userID int64) ([]model.ConvSummary, error)

	// ── Unread counters ──
	IncrementUnread(ctx context.Context, userID int64, convID string) error
	ClearUnread(ctx context.Context, userID int64, convID string) error
	GetUnreadMap(ctx context.Context, userID int64) (map[string]int64, error)

	// ── Group read position ──
	SetGroupReadPos(ctx context.Context, userID int64, convID string, seq int64) error
	GetGroupReadPos(ctx context.Context, userID int64, convID string) (int64, error)

	// ── Group memberships ──
	GetGroupMemberships(ctx context.Context, userID int64) ([]int64, error)
	GetGroupMembers(ctx context.Context, groupID int64) ([]int64, error)

	// ── Dedup ──
	CheckDuplicate(ctx context.Context, userID int64, clientMsgID string) (bool, error)

	// ── Trim (for cleanup task) ──
	TrimInbox(ctx context.Context, userID int64, maxCount int) error
	TrimOutbox(ctx context.Context, groupID int64, maxCount int) error
	TrimInboxByTime(ctx context.Context, userID int64, beforeTimestamp int64) error
	TrimOutboxByTime(ctx context.Context, groupID int64, beforeTimestamp int64) error
	TrimConvListByTime(ctx context.Context, userID int64, beforeTimestamp int64) error
	TrimTimelineByTime(ctx context.Context, userID int64, beforeTimestamp int64) error

	// ── Lua script wrappers ──
	ExecPrivateMsgCheck(ctx context.Context, senderID, receiverID int64, clientMsgID string) (*redis.PrivateMsgCheckResult, error)
	ExecGroupMsgCheck(ctx context.Context, groupID, senderID int64, clientMsgID string) (*redis.GroupMsgCheckResult, error)
	ExecInboxMarkRead(ctx context.Context, userID int64, convID string) (int64, error)
	ExecRevokeMsg(ctx context.Context, userID int64, convID string, msgID int64, revokeMsgJSON string, nowTimestamp int64) (bool, error)

	// ── Moment feed ──
	PublishMomentFeed(ctx context.Context, userID int64, momentID int64, timestamp int64) error
	GetMomentFeed(ctx context.Context, userID int64, lastSyncTime int64, limit int) ([]int64, error)
}

// ──────────────────────────────────────────────────────
// RedisRepoImpl — concrete implementation using go-redis
// ──────────────────────────────────────────────────────

type RedisRepoImpl struct {
	rdb *goredis.Client
}

func NewRedisRepo(rdb *goredis.Client) *RedisRepoImpl {
	return &RedisRepoImpl{rdb: rdb}
}

// ── Inbox / Outbox ──

func (r *RedisRepoImpl) WriteInbox(ctx context.Context, userID int64, msg *model.InboxMessage) error {
	key := fmt.Sprintf("inbox:%d", userID)
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal inbox message: %w", err)
	}
	return r.rdb.ZAdd(ctx, key, goredis.Z{
		Score:  float64(msg.Timestamp),
		Member: string(msgJSON),
	}).Err()
}

func (r *RedisRepoImpl) WriteOutbox(ctx context.Context, groupID int64, msg *model.InboxMessage) error {
	key := fmt.Sprintf("outbox:%d", groupID)
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal outbox message: %w", err)
	}
	return r.rdb.ZAdd(ctx, key, goredis.Z{
		Score:  float64(msg.Timestamp),
		Member: string(msgJSON),
	}).Err()
}

func (r *RedisRepoImpl) ReadInbox(ctx context.Context, userID int64, lastSyncTime int64, batchSize int) ([]model.InboxMessage, error) {
	key := fmt.Sprintf("inbox:%d", userID)
	// ZRANGEBYSCORE: messages with timestamp >= lastSyncTime (inclusive).
	// Inclusive lower bound prevents message loss when multiple messages share
	// the same timestamp as the boundary. The client deduplicates by MsgID.
	min := fmt.Sprintf("%d", lastSyncTime)
	results, err := r.rdb.ZRangeByScore(ctx, key, &goredis.ZRangeBy{
		Min:    min,
		Max:    "+inf",
		Offset: 0,
		Count:  int64(batchSize),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("ZRangeByScore inbox: %w", err)
	}

	msgs := make([]model.InboxMessage, 0, len(results))
	for _, raw := range results {
		var m model.InboxMessage
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			// Log but don't skip — malformed entries indicate a data integrity issue
			// that operators should investigate. We skip the entry to avoid crashing.
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (r *RedisRepoImpl) ReadOutbox(ctx context.Context, groupID int64, lastSyncTime int64, limit int) ([]model.InboxMessage, error) {
	key := fmt.Sprintf("outbox:%d", groupID)
	// Inclusive lower bound (same reasoning as ReadInbox — prevents boundary loss)
	min := fmt.Sprintf("%d", lastSyncTime)
	results, err := r.rdb.ZRangeByScore(ctx, key, &goredis.ZRangeBy{
		Min:    min,
		Max:    "+inf",
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("ZRangeByScore outbox: %w", err)
	}

	msgs := make([]model.InboxMessage, 0, len(results))
	for _, raw := range results {
		var m model.InboxMessage
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// ── Conversation list ──

func (r *RedisRepoImpl) UpdateConvList(ctx context.Context, userID int64, convID string, summary string, timestamp int64) error {
	key := fmt.Sprintf("conv_list:%d", userID)
	// Store summary JSON as the ZSet member so GetConvList can return full metadata.
	member := summary
	if member == "" {
		member = convID // fallback: store just convID if no summary provided
	}
	return r.rdb.ZAdd(ctx, key, goredis.Z{
		Score:  float64(timestamp),
		Member: member,
	}).Err()
}

func (r *RedisRepoImpl) GetConvList(ctx context.Context, userID int64) ([]model.ConvSummary, error) {
	key := fmt.Sprintf("conv_list:%d", userID)
	// ZREVRANGE returns members sorted by lastMsgTime descending.
	members, err := r.rdb.ZRevRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("ZRevRange conv_list: %w", err)
	}

	summaries := make([]model.ConvSummary, 0, len(members))
	for _, member := range members {
		// If the member is JSON, try to parse it as ConvSummary.
		// If parsing fails, treat it as a plain convID string.
		var s model.ConvSummary
		if err := json.Unmarshal([]byte(member), &s); err == nil && s.ConvID != "" {
			summaries = append(summaries, s)
		} else {
			summaries = append(summaries, model.ConvSummary{ConvID: member})
		}
	}
	return summaries, nil
}

// ── Unread counters ──

func (r *RedisRepoImpl) IncrementUnread(ctx context.Context, userID int64, convID string) error {
	key := fmt.Sprintf("unread:%d", userID)
	return r.rdb.HIncrBy(ctx, key, convID, 1).Err()
}

func (r *RedisRepoImpl) ClearUnread(ctx context.Context, userID int64, convID string) error {
	key := fmt.Sprintf("unread:%d", userID)
	return r.rdb.HSet(ctx, key, convID, 0).Err()
}

func (r *RedisRepoImpl) GetUnreadMap(ctx context.Context, userID int64) (map[string]int64, error) {
	key := fmt.Sprintf("unread:%d", userID)
	raw, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("HGetAll unread: %w", err)
	}

	result := make(map[string]int64, len(raw))
	for k, v := range raw {
		n, _ := strconv.ParseInt(v, 10, 64)
		if n > 0 {
			result[k] = n
		}
	}
	return result, nil
}

// ── Group read position ──

func (r *RedisRepoImpl) SetGroupReadPos(ctx context.Context, userID int64, convID string, seq int64) error {
	key := fmt.Sprintf("group_read_pos:%d", userID)
	return r.rdb.HSet(ctx, key, convID, seq).Err()
}

func (r *RedisRepoImpl) GetGroupReadPos(ctx context.Context, userID int64, convID string) (int64, error) {
	key := fmt.Sprintf("group_read_pos:%d", userID)
	val, err := r.rdb.HGet(ctx, key, convID).Result()
	if err != nil {
		if err == goredis.Nil {
			return 0, nil // no read position yet
		}
		return 0, fmt.Errorf("HGet group_read_pos: %w", err)
	}
	return strconv.ParseInt(val, 10, 64)
}

// ── Group memberships ──

func (r *RedisRepoImpl) GetGroupMemberships(ctx context.Context, userID int64) ([]int64, error) {
	key := fmt.Sprintf("user_groups:%d", userID)
	results, err := r.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("SMembers user_groups: %w", err)
	}

	groupIDs := make([]int64, 0, len(results))
	for _, s := range results {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		groupIDs = append(groupIDs, id)
	}
	return groupIDs, nil
}

func (r *RedisRepoImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]int64, error) {
	key := fmt.Sprintf("group_members:%d", groupID)
	results, err := r.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("SMembers group_members: %w", err)
	}

	memberIDs := make([]int64, 0, len(results))
	for _, s := range results {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		memberIDs = append(memberIDs, id)
	}
	return memberIDs, nil
}

// ── Dedup ──

func (r *RedisRepoImpl) CheckDuplicate(ctx context.Context, userID int64, clientMsgID string) (bool, error) {
	key := fmt.Sprintf("msg_dedup:%d:%s", userID, clientMsgID)
	ok, err := r.rdb.SetNX(ctx, key, "1", 300*time.Second).Result() // 300s TTL
	if err != nil {
		return false, fmt.Errorf("SetNX dedup: %w", err)
	}
	return !ok, nil // true = duplicate (SetNX failed)
}

// ── Trim ──

func (r *RedisRepoImpl) TrimInbox(ctx context.Context, userID int64, maxCount int) error {
	key := fmt.Sprintf("inbox:%d", userID)
	card, err := r.rdb.ZCard(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("ZCard inbox: %w", err)
	}
	if card > int64(maxCount) {
		// Remove oldest entries beyond maxCount
		removeCount := card - int64(maxCount)
		if err := r.rdb.ZRemRangeByRank(ctx, key, 0, removeCount-1).Err(); err != nil {
			return fmt.Errorf("ZRemRangeByRank inbox: %w", err)
		}
	}
	return nil
}

func (r *RedisRepoImpl) TrimOutbox(ctx context.Context, groupID int64, maxCount int) error {
	key := fmt.Sprintf("outbox:%d", groupID)
	card, err := r.rdb.ZCard(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("ZCard outbox: %w", err)
	}
	if card > int64(maxCount) {
		removeCount := card - int64(maxCount)
		if err := r.rdb.ZRemRangeByRank(ctx, key, 0, removeCount-1).Err(); err != nil {
			return fmt.Errorf("ZRemRangeByRank outbox: %w", err)
		}
	}
	return nil
}

// ── Time-based Trim ──

func (r *RedisRepoImpl) TrimInboxByTime(ctx context.Context, userID int64, beforeTimestamp int64) error {
	key := fmt.Sprintf("inbox:%d", userID)
	max := fmt.Sprintf("%d", beforeTimestamp)
	if err := r.rdb.ZRemRangeByScore(ctx, key, "0", max).Err(); err != nil {
		return fmt.Errorf("ZRemRangeByScore inbox: %w", err)
	}
	return nil
}

func (r *RedisRepoImpl) TrimOutboxByTime(ctx context.Context, groupID int64, beforeTimestamp int64) error {
	key := fmt.Sprintf("outbox:%d", groupID)
	max := fmt.Sprintf("%d", beforeTimestamp)
	if err := r.rdb.ZRemRangeByScore(ctx, key, "0", max).Err(); err != nil {
		return fmt.Errorf("ZRemRangeByScore outbox: %w", err)
	}
	return nil
}

func (r *RedisRepoImpl) TrimConvListByTime(ctx context.Context, userID int64, beforeTimestamp int64) error {
	key := fmt.Sprintf("conv_list:%d", userID)
	max := fmt.Sprintf("%d", beforeTimestamp)
	if err := r.rdb.ZRemRangeByScore(ctx, key, "0", max).Err(); err != nil {
		return fmt.Errorf("ZRemRangeByScore conv_list: %w", err)
	}
	return nil
}

func (r *RedisRepoImpl) TrimTimelineByTime(ctx context.Context, userID int64, beforeTimestamp int64) error {
	key := fmt.Sprintf("timeline:%d", userID)
	max := fmt.Sprintf("%d", beforeTimestamp)
	if err := r.rdb.ZRemRangeByScore(ctx, key, "0", max).Err(); err != nil {
		return fmt.Errorf("ZRemRangeByScore timeline: %w", err)
	}
	return nil
}

// ── Lua script wrappers ──

func (r *RedisRepoImpl) ExecPrivateMsgCheck(ctx context.Context, senderID, receiverID int64, clientMsgID string) (*redis.PrivateMsgCheckResult, error) {
	return redis.ExecPrivateMsgCheck(r.rdb, ctx, senderID, receiverID, clientMsgID)
}

func (r *RedisRepoImpl) ExecGroupMsgCheck(ctx context.Context, groupID, senderID int64, clientMsgID string) (*redis.GroupMsgCheckResult, error) {
	return redis.ExecGroupMsgCheck(r.rdb, ctx, groupID, senderID, clientMsgID)
}

func (r *RedisRepoImpl) ExecInboxMarkRead(ctx context.Context, userID int64, convID string) (int64, error) {
	return redis.ExecInboxMarkRead(r.rdb, ctx, userID, convID)
}

func (r *RedisRepoImpl) ExecRevokeMsg(ctx context.Context, userID int64, convID string, msgID int64, revokeMsgJSON string, nowTimestamp int64) (bool, error) {
	return redis.ExecRevokeMsg(r.rdb, ctx, userID, convID, msgID, revokeMsgJSON, nowTimestamp)
}

// ── Moment feed ──

func (r *RedisRepoImpl) PublishMomentFeed(ctx context.Context, userID int64, momentID int64, timestamp int64) error {
	key := fmt.Sprintf("timeline:%d", userID)
	return r.rdb.ZAdd(ctx, key, goredis.Z{
		Score:  float64(timestamp),
		Member: strconv.FormatInt(momentID, 10),
	}).Err()
}

func (r *RedisRepoImpl) GetMomentFeed(ctx context.Context, userID int64, lastSyncTime int64, limit int) ([]int64, error) {
	key := fmt.Sprintf("timeline:%d", userID)
	min := fmt.Sprintf("%d", lastSyncTime)
	results, err := r.rdb.ZRangeByScore(ctx, key, &goredis.ZRangeBy{
		Min:    min,
		Max:    "+inf",
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("ZRangeByScore timeline: %w", err)
	}

	momentIDs := make([]int64, 0, len(results))
	for _, raw := range results {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue // skip malformed entries
		}
		momentIDs = append(momentIDs, id)
	}
	return momentIDs, nil
}
