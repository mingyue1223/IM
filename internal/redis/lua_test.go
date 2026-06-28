package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRedis connects to a local Redis instance for integration tests.
// Requires a Redis server running at localhost:6379 (e.g. via docker-compose).
func setupTestRedis(t *testing.T) *goredis.Client {
	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	err := rdb.Ping(context.Background()).Err()
	if err != nil {
		t.Skipf("Redis not available at localhost:6379: %v", err)
	}
	return rdb
}

// cleanupKeys deletes all keys created during a test to avoid cross-test interference.
func cleanupKeys(t *testing.T, rdb *goredis.Client, ctx context.Context, keys ...string) {
	for _, key := range keys {
		rdb.Del(ctx, key)
	}
}

// ========== Private Message Check ==========

func TestPrivateMsgCheck(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: create bidirectional friendship, receiver online, no blacklist
	rdb.Set(ctx, "friend:1:2", "1", 0)
	rdb.Set(ctx, "friend:2:1", "1", 0)
	rdb.Set(ctx, "online:2", "1", 0)

	defer cleanupKeys(t, rdb, ctx, "friend:1:2", "friend:2:1", "online:2", "msg_id_global")

	result, err := ExecPrivateMsgCheck(rdb, ctx, 1, 2, "client-msg-001")
	require.NoError(t, err)

	assert.Equal(t, PMErrOK, result.ErrCode, "should succeed with valid friendship")
	assert.Greater(t, result.MsgID, int64(0), "msgID should be allocated")
	assert.True(t, result.IsOnline, "receiver should be online")
	assert.True(t, result.IsFriend, "should be friends")
	assert.False(t, result.IsBlocked, "should not be blocked")

	// Cleanup dedup key
	rdb.Del(ctx, "msg_dedup:1:client-msg-001")
}

func TestPrivateMsgCheckNotFriend(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// No friendship keys set up

	defer cleanupKeys(t, rdb, ctx, "msg_id_global")

	result, err := ExecPrivateMsgCheck(rdb, ctx, 1, 2, "client-msg-002")
	require.NoError(t, err)

	assert.Equal(t, PMErrNotFriend, result.ErrCode, "should fail with not_friend")
	assert.Equal(t, int64(0), result.MsgID, "msgID should be 0 on failure")
}

func TestPrivateMsgCheckBlocked(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: friendship exists but receiver has sender in blacklist
	rdb.Set(ctx, "friend:1:2", "1", 0)
	rdb.Set(ctx, "friend:2:1", "1", 0)
	rdb.SAdd(ctx, "blacklist:2", "1")

	defer cleanupKeys(t, rdb, ctx, "friend:1:2", "friend:2:1", "blacklist:2", "msg_id_global")

	result, err := ExecPrivateMsgCheck(rdb, ctx, 1, 2, "client-msg-003")
	require.NoError(t, err)

	assert.Equal(t, PMErrBlocked, result.ErrCode, "should fail with blocked")
	assert.True(t, result.IsBlocked, "should be blocked")
}

func TestPrivateMsgCheckDuplicate(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: friendship + no blacklist
	rdb.Set(ctx, "friend:1:2", "1", 0)
	rdb.Set(ctx, "friend:2:1", "1", 0)
	// Simulate a previous message with the same clientMsgID (dedup key exists)
	rdb.Set(ctx, "msg_dedup:1:client-msg-dup", "1", 0)

	defer cleanupKeys(t, rdb, ctx, "friend:1:2", "friend:2:1", "msg_dedup:1:client-msg-dup", "msg_id_global")

	result, err := ExecPrivateMsgCheck(rdb, ctx, 1, 2, "client-msg-dup")
	require.NoError(t, err)

	assert.Equal(t, PMErrDuplicate, result.ErrCode, "should fail with duplicate")
}

func TestPrivateMsgCheckOffline(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: friendship exists, receiver offline
	rdb.Set(ctx, "friend:1:2", "1", 0)
	rdb.Set(ctx, "friend:2:1", "1", 0)
	// No online:2 key → receiver offline

	defer cleanupKeys(t, rdb, ctx, "friend:1:2", "friend:2:1", "msg_id_global")

	result, err := ExecPrivateMsgCheck(rdb, ctx, 1, 2, "client-msg-offline")
	require.NoError(t, err)

	assert.Equal(t, PMErrOK, result.ErrCode, "should succeed")
	assert.False(t, result.IsOnline, "receiver should be offline")

	// Cleanup dedup key
	rdb.Del(ctx, "msg_dedup:1:client-msg-offline")
}

// ========== Group Message Check ==========

func TestGroupMsgCheck(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: sender is a group member, not muted
	rdb.SAdd(ctx, "group_members:5", "1")
	// memberInfo with muted=false
	memberInfo, _ := json.Marshal(map[string]interface{}{"role": "member", "muted": false})
	rdb.HSet(ctx, "group_member_info:5", "1", memberInfo)

	defer cleanupKeys(t, rdb, ctx, "group_members:5", "group_member_info:5", "group_seq:5", "msg_id_global")

	result, err := ExecGroupMsgCheck(rdb, ctx, 5, 1, "client-grp-001")
	require.NoError(t, err)

	assert.Equal(t, GMErrOK, result.ErrCode, "should succeed")
	assert.Greater(t, result.MsgID, int64(0), "msgID should be allocated")
	assert.Greater(t, result.GroupSeq, int64(0), "groupSeq should be allocated")
	assert.True(t, result.IsMember, "should be a member")
	assert.False(t, result.IsMuted, "should not be muted")

	// Cleanup dedup key
	rdb.Del(ctx, "msg_dedup:1:client-grp-001")
}

func TestGroupMsgCheckNotMember(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Sender not in group_members
	defer cleanupKeys(t, rdb, ctx, "msg_id_global")

	result, err := ExecGroupMsgCheck(rdb, ctx, 5, 99, "client-grp-notmember")
	require.NoError(t, err)

	assert.Equal(t, GMErrNotMember, result.ErrCode, "should fail with not_member")
	assert.Equal(t, int64(0), result.MsgID, "msgID should be 0 on failure")
}

func TestGroupMsgCheckMuted(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: sender is a member but muted
	rdb.SAdd(ctx, "group_members:5", "1")
	memberInfo, _ := json.Marshal(map[string]interface{}{"role": "member", "muted": true})
	rdb.HSet(ctx, "group_member_info:5", "1", memberInfo)

	defer cleanupKeys(t, rdb, ctx, "group_members:5", "group_member_info:5", "msg_id_global")

	result, err := ExecGroupMsgCheck(rdb, ctx, 5, 1, "client-grp-muted")
	require.NoError(t, err)

	assert.Equal(t, GMErrMuted, result.ErrCode, "should fail with muted")
	assert.True(t, result.IsMuted, "should be muted")
}

func TestGroupMsgCheckMutedNoInfo(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	// Setup: sender is a member, no memberInfo hash entry (not muted)
	rdb.SAdd(ctx, "group_members:5", "1")
	// No group_member_info entry → not muted

	defer cleanupKeys(t, rdb, ctx, "group_members:5", "group_seq:5", "msg_id_global")

	result, err := ExecGroupMsgCheck(rdb, ctx, 5, 1, "client-grp-noinfo")
	require.NoError(t, err)

	assert.Equal(t, GMErrOK, result.ErrCode, "should succeed when no mute info")
	assert.False(t, result.IsMuted, "should not be muted when no info")

	// Cleanup dedup key
	rdb.Del(ctx, "msg_dedup:1:client-grp-noinfo")
}

// ========== Inbox Mark Read ==========

func TestInboxMarkRead(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	userID := int64(100)
	convID := "p_100_200"
	inboxKey := fmt.Sprintf("inbox:%d", userID)
	unreadKey := fmt.Sprintf("unread:%d", userID)

	// Setup: add 3 messages to inbox, 2 unread (readStatus=0) for target conv, 1 read
	ts := time.Now().UnixMilli()
	msg1 := map[string]interface{}{
		"msgId":      1,
		"convId":     convID,
		"convType":   1,
		"fromId":     200,
		"toId":       100,
		"msgType":    1,
		"content":    "hello",
		"readStatus": 0, // unread
		"timestamp":  ts,
	}
	msg2 := map[string]interface{}{
		"msgId":      2,
		"convId":     convID,
		"convType":   1,
		"fromId":     200,
		"toId":       100,
		"msgType":    1,
		"content":    "world",
		"readStatus": 0, // unread
		"timestamp":  ts + 1,
	}
	msg3 := map[string]interface{}{
		"msgId":      3,
		"convId":     "p_100_300", // different conversation
		"convType":   1,
		"fromId":     300,
		"toId":       100,
		"msgType":    1,
		"content":    "other conv",
		"readStatus": 0, // unread but different convID — should NOT be marked
		"timestamp":  ts + 2,
	}

	msg1JSON, _ := json.Marshal(msg1)
	msg2JSON, _ := json.Marshal(msg2)
	msg3JSON, _ := json.Marshal(msg3)

	rdb.ZAdd(ctx, inboxKey, goredis.Z{Score: float64(ts), Member: string(msg1JSON)})
	rdb.ZAdd(ctx, inboxKey, goredis.Z{Score: float64(ts + 1), Member: string(msg2JSON)})
	rdb.ZAdd(ctx, inboxKey, goredis.Z{Score: float64(ts + 2), Member: string(msg3JSON)})
	rdb.HSet(ctx, unreadKey, convID, 2) // 2 unread for p_100_200

	defer cleanupKeys(t, rdb, ctx, inboxKey, unreadKey)

	count, err := ExecInboxMarkRead(rdb, ctx, userID, convID)
	require.NoError(t, err)

	assert.Equal(t, int64(2), count, "should mark 2 messages as read")

	// Verify unread counter reset
	unreadVal, _ := rdb.HGet(ctx, unreadKey, convID).Int64()
	assert.Equal(t, int64(0), unreadVal, "unread counter should be 0")

	// Verify the different-conv message is still unread
	// (We can check by scanning the inbox for messages with readStatus=0 in p_100_300)
	remaining, _ := rdb.ZRange(ctx, inboxKey, 0, -1).Result()
	foundUnreadOther := false
	for _, m := range remaining {
		var parsed map[string]interface{}
		json.Unmarshal([]byte(m), &parsed)
		if parsed["convId"] == "p_100_300" {
			rs, _ := strconv.Atoi(fmt.Sprintf("%v", parsed["readStatus"]))
			if rs == 0 {
				foundUnreadOther = true
			}
		}
	}
	assert.True(t, foundUnreadOther, "other conversation messages should remain unread")
}

func TestInboxMarkReadNoUnread(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	userID := int64(100)
	convID := "p_100_200"
	inboxKey := fmt.Sprintf("inbox:%d", userID)

	// Setup: add a message that is already read
	ts := time.Now().UnixMilli()
	msg := map[string]interface{}{
		"msgId":      1,
		"convId":     convID,
		"convType":   1,
		"fromId":     200,
		"toId":       100,
		"msgType":    1,
		"content":    "already read",
		"readStatus": 1, // already read
		"timestamp":  ts,
	}
	msgJSON, _ := json.Marshal(msg)
	rdb.ZAdd(ctx, inboxKey, goredis.Z{Score: float64(ts), Member: string(msgJSON)})

	defer cleanupKeys(t, rdb, ctx, inboxKey)

	count, err := ExecInboxMarkRead(rdb, ctx, userID, convID)
	require.NoError(t, err)

	assert.Equal(t, int64(0), count, "should mark 0 messages when all are already read")
}

// ========== Revoke Message ==========

func TestRevokeMsgPrivate(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	userID := int64(200) // receiver
	convID := "p_100_200"
	msgID := int64(12345)
	inboxKey := fmt.Sprintf("inbox:%d", userID)

	// Setup: add a message to inbox that was sent recently
	ts := time.Now().UnixMilli() - 30000 // 30 seconds ago (within 2-min window)
	msg := map[string]interface{}{
		"msgId":      msgID,
		"convId":     convID,
		"convType":   1,
		"fromId":     100,
		"toId":       200,
		"msgType":    1,
		"content":    "hello",
		"readStatus": 0,
		"timestamp":  ts,
	}
	msgJSON, _ := json.Marshal(msg)
	rdb.ZAdd(ctx, inboxKey, goredis.Z{Score: float64(ts), Member: string(msgJSON)})

	// Revoke message JSON (msgType=6)
	revokeMsg := map[string]interface{}{
		"msgId":      msgID,
		"convId":     convID,
		"convType":   1,
		"fromId":     100,
		"toId":       200,
		"msgType":    6, // revoked type
		"content":    "message revoked",
		"readStatus": 0,
		"timestamp":  ts,
	}
	revokeJSON, _ := json.Marshal(revokeMsg)

	defer cleanupKeys(t, rdb, ctx, inboxKey)

	ok, err := ExecRevokeMsg(rdb, ctx, userID, convID, msgID, string(revokeJSON), time.Now().UnixMilli())
	require.NoError(t, err)

	assert.True(t, ok, "revoke should succeed within 2 minutes")

	// Verify the revoked message is in the inbox
	remaining, _ := rdb.ZRange(ctx, inboxKey, 0, -1).Result()
	assert.Equal(t, 1, len(remaining), "inbox should contain exactly 1 message")
	var parsed map[string]interface{}
	json.Unmarshal([]byte(remaining[0]), &parsed)
	mt, _ := strconv.Atoi(fmt.Sprintf("%v", parsed["msgType"]))
	assert.Equal(t, 6, mt, "message type should be revoked (6)")
}

func TestRevokeMsgTooLate(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	userID := int64(200)
	convID := "p_100_200"
	msgID := int64(12346)
	inboxKey := fmt.Sprintf("inbox:%d", userID)

	// Setup: add a message that was sent more than 2 minutes ago
	ts := time.Now().UnixMilli() - 180000 // 3 minutes ago
	msg := map[string]interface{}{
		"msgId":      msgID,
		"convId":     convID,
		"convType":   1,
		"fromId":     100,
		"toId":       200,
		"msgType":    1,
		"content":    "hello",
		"readStatus": 0,
		"timestamp":  ts,
	}
	msgJSON, _ := json.Marshal(msg)
	rdb.ZAdd(ctx, inboxKey, goredis.Z{Score: float64(ts), Member: string(msgJSON)})

	defer cleanupKeys(t, rdb, ctx, inboxKey)

	ok, err := ExecRevokeMsg(rdb, ctx, userID, convID, msgID, "\"revoked\"", time.Now().UnixMilli())
	require.NoError(t, err)

	assert.False(t, ok, "revoke should fail when message is older than 2 minutes")
}

func TestRevokeMsgNotFound(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	userID := int64(200)
	convID := "p_100_200"
	msgID := int64(99999) // non-existent message
	inboxKey := fmt.Sprintf("inbox:%d", userID)

	// Inbox is empty

	defer cleanupKeys(t, rdb, ctx, inboxKey)

	ok, err := ExecRevokeMsg(rdb, ctx, userID, convID, msgID, "\"revoked\"", time.Now().UnixMilli())
	require.NoError(t, err)

	assert.False(t, ok, "revoke should fail when message not found")
}

func TestRevokeMsgGroup(t *testing.T) {
	rdb := setupTestRedis(t)
	ctx := context.Background()

	convID := "g_5"
	groupID := "5"
	msgID := int64(12347)
	outboxKey := fmt.Sprintf("outbox:%s", groupID)

	// For group revoke, userID is used only as a parameter but the ZSet key is outbox:{groupID}
	ts := time.Now().UnixMilli() - 10000 // 10 seconds ago
	msg := map[string]interface{}{
		"msgId":      msgID,
		"groupId":    5,
		"convId":     convID,
		"convType":   2,
		"fromId":     100,
		"msgType":    1,
		"content":    "hello group",
		"timestamp":  ts,
		"groupSeq":   42,
	}
	msgJSON, _ := json.Marshal(msg)
	rdb.ZAdd(ctx, outboxKey, goredis.Z{Score: float64(ts), Member: string(msgJSON)})

	revokeMsg := map[string]interface{}{
		"msgId":      msgID,
		"groupId":    5,
		"convId":     convID,
		"convType":   2,
		"fromId":     100,
		"msgType":    6,
		"content":    "message revoked",
		"timestamp":  ts,
		"groupSeq":   42,
	}
	revokeJSON, _ := json.Marshal(revokeMsg)

	defer cleanupKeys(t, rdb, ctx, outboxKey)

	// userID=0 for group revoke (doesn't affect key resolution, convID determines the ZSet)
	ok, err := ExecRevokeMsg(rdb, ctx, 0, convID, msgID, string(revokeJSON), time.Now().UnixMilli())
	require.NoError(t, err)

	assert.True(t, ok, "group message revoke should succeed within 2 minutes")

	// Verify the revoked message is in the outbox
	remaining, _ := rdb.ZRange(ctx, outboxKey, 0, -1).Result()
	assert.Equal(t, 1, len(remaining), "outbox should contain exactly 1 message")
	var parsed map[string]interface{}
	json.Unmarshal([]byte(remaining[0]), &parsed)
	mt, _ := strconv.Atoi(fmt.Sprintf("%v", parsed["msgType"]))
	assert.Equal(t, 6, mt, "message type should be revoked (6)")
}
