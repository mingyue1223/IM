package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	redislua "github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/protocol"
)

// ──────────────────────────────────────────────────────
// MockRedisRepo — implements repository.RedisRepo for testing
// ──────────────────────────────────────────────────────

type MockRedisRepo struct {
	mu sync.Mutex

	// Captured data for verification
	inboxWrites      map[int64][]*model.InboxMessage // userID -> messages
	outboxWrites     map[int64][]*model.InboxMessage // groupID -> messages
	convListUpdates  map[int64][]convListEntry        // userID -> entries
	unreadIncrements map[string]int                   // "userID:convID" -> count
	groupMembers     map[int64][]int64                // groupID -> memberIDs

	// Track which users/groups had trim called
	trimmedInboxUsers   []int64
	trimmedOutboxGroups []int64

	// Control: inject errors
	writeInboxErr       error
	writeOutboxErr      error
	getGroupMembersErr  error
}

type convListEntry struct {
	convID    string
	summary   string
	timestamp int64
}

func newMockRedisRepo() *MockRedisRepo {
	return &MockRedisRepo{
		inboxWrites:      make(map[int64][]*model.InboxMessage),
		outboxWrites:     make(map[int64][]*model.InboxMessage),
		convListUpdates:  make(map[int64][]convListEntry),
		unreadIncrements: make(map[string]int),
		groupMembers:     make(map[int64][]int64),
	}
}

func (m *MockRedisRepo) WriteInbox(ctx context.Context, userID int64, msg *model.InboxMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeInboxErr != nil {
		return m.writeInboxErr
	}
	m.inboxWrites[userID] = append(m.inboxWrites[userID], msg)
	return nil
}

func (m *MockRedisRepo) WriteOutbox(ctx context.Context, groupID int64, msg *model.InboxMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeOutboxErr != nil {
		return m.writeOutboxErr
	}
	m.outboxWrites[groupID] = append(m.outboxWrites[groupID], msg)
	return nil
}

func (m *MockRedisRepo) ReadInbox(ctx context.Context, userID int64, lastSyncTime int64, batchSize int) ([]model.InboxMessage, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) ReadOutbox(ctx context.Context, groupID int64, lastSyncTime int64, limit int) ([]model.InboxMessage, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) UpdateConvList(ctx context.Context, userID int64, convID string, summary string, timestamp int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.convListUpdates[userID] = append(m.convListUpdates[userID], convListEntry{
		convID:    convID,
		summary:   summary,
		timestamp: timestamp,
	})
	return nil
}

func (m *MockRedisRepo) GetConvList(ctx context.Context, userID int64) ([]model.ConvSummary, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) IncrementUnread(ctx context.Context, userID int64, convID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d:%s", userID, convID)
	m.unreadIncrements[key]++
	return nil
}

func (m *MockRedisRepo) ClearUnread(ctx context.Context, userID int64, convID string) error {
	return fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) GetUnreadMap(ctx context.Context, userID int64) (map[string]int64, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) SetGroupReadPos(ctx context.Context, userID int64, convID string, seq int64) error {
	return fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) GetGroupReadPos(ctx context.Context, userID int64, convID string) (int64, error) {
	return 0, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) GetGroupMemberships(ctx context.Context, userID int64) ([]int64, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) GetGroupMembers(ctx context.Context, groupID int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getGroupMembersErr != nil {
		return nil, m.getGroupMembersErr
	}
	return m.groupMembers[groupID], nil
}

func (m *MockRedisRepo) CheckDuplicate(ctx context.Context, userID int64, clientMsgID string) (bool, error) {
	return false, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) TrimInbox(ctx context.Context, userID int64, maxCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trimmedInboxUsers = append(m.trimmedInboxUsers, userID)
	return nil
}

func (m *MockRedisRepo) TrimOutbox(ctx context.Context, groupID int64, maxCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trimmedOutboxGroups = append(m.trimmedOutboxGroups, groupID)
	return nil
}

func (m *MockRedisRepo) TrimInboxByTime(ctx context.Context, userID int64, beforeTimestamp int64) error {
	return fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) TrimOutboxByTime(ctx context.Context, groupID int64, beforeTimestamp int64) error {
	return fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) TrimConvListByTime(ctx context.Context, userID int64, beforeTimestamp int64) error {
	return fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) TrimTimelineByTime(ctx context.Context, userID int64, beforeTimestamp int64) error {
	return fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) ExecPrivateMsgCheck(ctx context.Context, senderID, receiverID int64, clientMsgID string) (*redislua.PrivateMsgCheckResult, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) ExecGroupMsgCheck(ctx context.Context, groupID, senderID int64, clientMsgID string) (*redislua.GroupMsgCheckResult, error) {
	return nil, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) ExecInboxMarkRead(ctx context.Context, userID int64, convID string) (int64, error) {
	return 0, fmt.Errorf("stub: not used by consumer")
}

func (m *MockRedisRepo) ExecRevokeMsg(ctx context.Context, userID int64, convID string, msgID int64, revokeMsgJSON string, nowTimestamp int64) (bool, error) {
	return false, fmt.Errorf("stub: not used by consumer")
}
func (m *MockRedisRepo) AddGroupMemberRedis(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *MockRedisRepo) RemoveGroupMemberRedis(_ context.Context, _ int64, _ int64) error { return nil }

// ──────────────────────────────────────────────────────
// Helper: create a ClientConnection without a real websocket.Conn
// We set the fields directly to avoid the need for a real WS connection.
// ──────────────────────────────────────────────────────

func makeTestClientConnection(userID int64) *conn.ClientConnection {
	return &conn.ClientConnection{
		UserID:   userID,
		Conn:     nil, // nil — we only use SendCh in tests
		SendCh:   make(chan []byte, 256),
		CloseCh:  make(chan struct{}),
		LastPing: time.Now(),
	}
}

// ──────────────────────────────────────────────────────
// Tests for PrivateMsgConsumer
// ──────────────────────────────────────────────────────

func TestPrivateMsgConsumer_Process(t *testing.T) {
	logger := zap.NewNop()
	redisRepo := newMockRedisRepo()
	realCM := conn.NewConnectionManager()

	// Register receiver as online (sender 100 doesn't need to be online for the consumer)
	receiverClient := makeTestClientConnection(200)
	realCM.Register(200, receiverClient)

	var pushedMsgs []string
	go func() {
		for msg := range receiverClient.SendCh {
			pushedMsgs = append(pushedMsgs, string(msg))
		}
	}()

	consumer := &PrivateMsgConsumer{
		ch:        nil,
		mysqlRepo: nil, // MySQL will fail — we verify Redis ops before that point
		redisRepo: redisRepo,
		cm:        realCM,
		logger:    logger,
	}

	msg := &model.PrivateMessage{
		ID:         1001,
		SenderID:   100,
		ReceiverID: 200,
		Content:    "hello",
		MsgType:    model.MsgTypeText,
		CreatedAt:  time.UnixMilli(1700000000000),
	}

	ctx := context.Background()

	// process() will fail at MySQL insert since db is nil,
	// but all Redis operations should succeed before that point.
	err := consumer.process(ctx, msg)
	require.NoError(t, err, "process should succeed even with nil mysqlRepo")

	// Verify Redis operations happened before the MySQL failure
	redisRepo.mu.Lock()

	// 1. Receiver inbox should have message with readStatus=0
	receiverInbox := redisRepo.inboxWrites[200]
	assert.Len(t, receiverInbox, 1, "receiver should have 1 inbox message")
	assert.Equal(t, 0, receiverInbox[0].ReadStatus, "receiver's message should be unread")
	assert.Equal(t, int64(1001), receiverInbox[0].MsgID)
	assert.Equal(t, "p_100_200", receiverInbox[0].ConvID)

	// 2. Sender inbox should have message with readStatus=1
	senderInbox := redisRepo.inboxWrites[100]
	assert.Len(t, senderInbox, 1, "sender should have 1 inbox message")
	assert.Equal(t, 1, senderInbox[0].ReadStatus, "sender's message should be read")
	assert.Equal(t, int64(1001), senderInbox[0].MsgID)

	// 3. conv_list should be updated for both users
	assert.Len(t, redisRepo.convListUpdates[100], 1, "sender conv_list updated")
	assert.Len(t, redisRepo.convListUpdates[200], 1, "receiver conv_list updated")
	assert.Equal(t, "p_100_200", redisRepo.convListUpdates[100][0].convID)
	assert.Equal(t, "p_100_200", redisRepo.convListUpdates[200][0].convID)

	// 4. Unread counter should be incremented for receiver only
	key := "200:p_100_200"
	assert.Equal(t, 1, redisRepo.unreadIncrements[key], "receiver unread incremented")
	senderKey := "100:p_100_200"
	assert.Equal(t, 0, redisRepo.unreadIncrements[senderKey], "sender unread NOT incremented")

	// 5. Both inboxes should be trimmed
	assert.Contains(t, redisRepo.trimmedInboxUsers, int64(200))
	assert.Contains(t, redisRepo.trimmedInboxUsers, int64(100))

	redisRepo.mu.Unlock()

	// 6. Verify WebSocket push to receiver
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, pushedMsgs, 1, "receiver should receive 1 WS push")
	if len(pushedMsgs) > 0 {
		var wsMsg model.WsMessage
		err := json.Unmarshal([]byte(pushedMsgs[0]), &wsMsg)
		require.NoError(t, err)
		assert.Equal(t, protocol.TypeMsg, wsMsg.Type, "push should be type 'msg'")

		var inboxMsg model.InboxMessage
		err = json.Unmarshal(wsMsg.Data, &inboxMsg)
		require.NoError(t, err)
		assert.Equal(t, int64(1001), inboxMsg.MsgID)
		assert.Equal(t, 0, inboxMsg.ReadStatus)
	}
}

func TestPrivateMsgConsumer_Deserialize(t *testing.T) {
	msg := &model.PrivateMessage{
		ID:         42,
		SenderID:   1,
		ReceiverID: 2,
		Content:    "test",
		MsgType:    model.MsgTypeText,
		CreatedAt:  time.Now(),
	}

	body, err := json.Marshal(msg)
	require.NoError(t, err)

	parsed, err := deserializePrivateMsg(body)
	require.NoError(t, err)
	assert.Equal(t, int64(42), parsed.ID)
	assert.Equal(t, int64(1), parsed.SenderID)
	assert.Equal(t, int64(2), parsed.ReceiverID)

	// Test invalid message (missing receiverID)
	invalidMsg := &model.PrivateMessage{SenderID: 1, ReceiverID: 0}
	invalidBody, err := json.Marshal(invalidMsg)
	require.NoError(t, err)
	_, err = deserializePrivateMsg(invalidBody)
	assert.Error(t, err, "should reject message with zero receiverID")

	// Test malformed JSON
	_, err = deserializePrivateMsg([]byte("not json"))
	assert.Error(t, err)
}

func TestPrivateMsgConsumer_WriteInboxFailure(t *testing.T) {
	logger := zap.NewNop()
	redisRepo := newMockRedisRepo()
	redisRepo.writeInboxErr = fmt.Errorf("Redis connection error")
	realCM := conn.NewConnectionManager()

	consumer := &PrivateMsgConsumer{
		ch:        nil,
		mysqlRepo: nil,
		redisRepo: redisRepo,
		cm:        realCM,
		logger:    logger,
	}

	msg := &model.PrivateMessage{
		ID:         100,
		SenderID:   1,
		ReceiverID: 2,
		Content:    "test",
		MsgType:    model.MsgTypeText,
		CreatedAt:  time.Now(),
	}

	ctx := context.Background()
	err := consumer.process(ctx, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write receiver inbox")
}

func TestPrivateMsgConsumer_HandleDelivery_Malformed(t *testing.T) {
	// Test that a malformed body fails deserialization
	_, err := deserializePrivateMsg([]byte("bad json"))
	assert.Error(t, err, "malformed body should fail deserialization")
}

// ──────────────────────────────────────────────────────
// Tests for GroupMsgConsumer
// ──────────────────────────────────────────────────────

func TestGroupMsgConsumer_Process(t *testing.T) {
	logger := zap.NewNop()
	redisRepo := newMockRedisRepo()
	// Set up group members: group 5 has members [1, 2, 3]
	redisRepo.groupMembers[5] = []int64{1, 2, 3}

	realCM := conn.NewConnectionManager()

	// Make member 2 and 3 online (sender 1 does not receive push)
	client2 := makeTestClientConnection(2)
	realCM.Register(2, client2)

	client3 := makeTestClientConnection(3)
	realCM.Register(3, client3)

	var pushed2 []string
	var pushed3 []string
	go func() {
		for msg := range client2.SendCh {
			pushed2 = append(pushed2, string(msg))
		}
	}()
	go func() {
		for msg := range client3.SendCh {
			pushed3 = append(pushed3, string(msg))
		}
	}()

	consumer := &GroupMsgConsumer{
		ch:        nil,
		mysqlRepo: nil, // MySQL will fail
		redisRepo: redisRepo,
		cm:        realCM,
		logger:    logger,
	}

	msg := &model.GroupMessage{
		ID:        2001,
		GroupID:   5,
		SenderID:  1,
		Content:   "group hello",
		MsgType:   model.MsgTypeText,
		GroupSeq:  10,
		CreatedAt: time.UnixMilli(1700000000000),
	}

	ctx := context.Background()
	err := consumer.process(ctx, msg)
	require.NoError(t, err, "process should succeed even with nil mysqlRepo")

	// Verify Redis operations happened before the MySQL failure
	redisRepo.mu.Lock()

	// 1. Group outbox should have the message
	outboxWrites := redisRepo.outboxWrites[5]
	assert.Len(t, outboxWrites, 1, "group outbox should have 1 message")
	assert.Equal(t, int64(2001), outboxWrites[0].MsgID)
	assert.Equal(t, model.ConvTypeGroup, outboxWrites[0].ConvType)
	assert.Equal(t, int64(10), outboxWrites[0].GroupSeq)
	assert.Equal(t, "g_5", outboxWrites[0].ConvID)

	// 2. conv_list should be updated for all 3 members
	assert.Len(t, redisRepo.convListUpdates[1], 1, "sender conv_list updated")
	assert.Len(t, redisRepo.convListUpdates[2], 1, "member2 conv_list updated")
	assert.Len(t, redisRepo.convListUpdates[3], 1, "member3 conv_list updated")
	assert.Equal(t, "g_5", redisRepo.convListUpdates[1][0].convID)
	assert.Equal(t, "g_5", redisRepo.convListUpdates[2][0].convID)

	// 3. Unread should be incremented for members 2 and 3 only (not sender 1)
	assert.Equal(t, 1, redisRepo.unreadIncrements["2:g_5"], "member2 unread incremented")
	assert.Equal(t, 1, redisRepo.unreadIncrements["3:g_5"], "member3 unread incremented")
	assert.Equal(t, 0, redisRepo.unreadIncrements["1:g_5"], "sender unread NOT incremented")

	// 4. Outbox should be trimmed
	assert.Contains(t, redisRepo.trimmedOutboxGroups, int64(5))

	redisRepo.mu.Unlock()

	// 5. Verify WebSocket pushes — only members 2 and 3, not sender 1
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, pushed2, 1, "member 2 should receive 1 WS push")
	assert.Len(t, pushed3, 1, "member 3 should receive 1 WS push")

	if len(pushed2) > 0 {
		var wsMsg model.WsMessage
		err := json.Unmarshal([]byte(pushed2[0]), &wsMsg)
		require.NoError(t, err)
		assert.Equal(t, protocol.TypeMsg, wsMsg.Type)

		var inboxMsg model.InboxMessage
		err = json.Unmarshal(wsMsg.Data, &inboxMsg)
		require.NoError(t, err)
		assert.Equal(t, int64(2001), inboxMsg.MsgID)
		assert.Equal(t, int64(10), inboxMsg.GroupSeq)
		assert.Equal(t, model.ConvTypeGroup, inboxMsg.ConvType)
	}
}

func TestGroupMsgConsumer_Process_NoMembers(t *testing.T) {
	logger := zap.NewNop()
	redisRepo := newMockRedisRepo()
	// Group 10 has no members
	redisRepo.groupMembers[10] = []int64{}

	realCM := conn.NewConnectionManager()

	consumer := &GroupMsgConsumer{
		ch:        nil,
		mysqlRepo: nil,
		redisRepo: redisRepo,
		cm:        realCM,
		logger:    logger,
	}

	msg := &model.GroupMessage{
		ID:        3001,
		GroupID:   10,
		SenderID:  1,
		Content:   "hello empty group",
		MsgType:   model.MsgTypeText,
		GroupSeq:  1,
		CreatedAt: time.Now(),
	}

	ctx := context.Background()
	err := consumer.process(ctx, msg)
	require.NoError(t, err, "process should succeed even with nil mysqlRepo")

	// Verify outbox was still written (outbox persists even with no members)
	redisRepo.mu.Lock()
	assert.Len(t, redisRepo.outboxWrites[10], 1, "outbox should be written even with no members")
	redisRepo.mu.Unlock()
}

func TestGroupMsgConsumer_GetMembersFailure(t *testing.T) {
	logger := zap.NewNop()
	redisRepo := newMockRedisRepo()
	redisRepo.getGroupMembersErr = fmt.Errorf("Redis unavailable")

	realCM := conn.NewConnectionManager()

	consumer := &GroupMsgConsumer{
		ch:        nil,
		mysqlRepo: nil,
		redisRepo: redisRepo,
		cm:        realCM,
		logger:    logger,
	}

	msg := &model.GroupMessage{
		ID:        4001,
		GroupID:   5,
		SenderID:  1,
		Content:   "test",
		MsgType:   model.MsgTypeText,
		GroupSeq:  1,
		CreatedAt: time.Now(),
	}

	ctx := context.Background()
	err := consumer.process(ctx, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get group members")
}

func TestGroupMsgConsumer_Deserialize(t *testing.T) {
	msg := &model.GroupMessage{
		ID:        42,
		GroupID:   5,
		SenderID:  1,
		Content:   "test",
		MsgType:   model.MsgTypeText,
		GroupSeq:  10,
		CreatedAt: time.Now(),
	}

	body, err := json.Marshal(msg)
	require.NoError(t, err)

	parsed, err := deserializeGroupMsg(body)
	require.NoError(t, err)
	assert.Equal(t, int64(42), parsed.ID)
	assert.Equal(t, int64(5), parsed.GroupID)
	assert.Equal(t, int64(10), parsed.GroupSeq)

	// Test invalid message (missing groupID)
	invalidMsg := &model.GroupMessage{GroupID: 0, SenderID: 1}
	invalidBody, err := json.Marshal(invalidMsg)
	require.NoError(t, err)
	_, err = deserializeGroupMsg(invalidBody)
	assert.Error(t, err, "should reject message with zero groupID")

	// Test malformed JSON
	_, err = deserializeGroupMsg([]byte("not json"))
	assert.Error(t, err)
}

// ──────────────────────────────────────────────────────
// Tests for shared helpers
// ──────────────────────────────────────────────────────

func TestTruncateContent(t *testing.T) {
	assert.Equal(t, "hello", truncateContent("hello", 50))
	assert.Equal(t, "a very long message that needs...", truncateContent("a very long message that needs truncation", 30))
	// Short multiline content is not truncated (total len <= maxLen)
	assert.Equal(t, "first line\nsecond line", truncateContent("first line\nsecond line", 50))
	// Long multiline content where first line itself exceeds maxLen: truncate first line only
	assert.Equal(t, "a very long first line that ex...", truncateContent("a very long first line that exceeds max\nsecond line", 30))
}

func TestBuildPrivateConvSummary(t *testing.T) {
	msg := &model.PrivateMessage{
		ID:         1,
		SenderID:   100,
		ReceiverID: 200,
		Content:    "hello there",
		MsgType:    model.MsgTypeText,
		CreatedAt:  time.UnixMilli(1700000000000),
	}

	convID := model.BuildConvID(model.ConvTypePrivate, 100, 200)
	summary := buildPrivateConvSummary(convID, msg)

	assert.Equal(t, "p_100_200", summary.ConvID)
	assert.Equal(t, model.ConvTypePrivate, summary.ConvType)
	assert.Equal(t, int64(200), summary.TargetID)
	assert.Equal(t, "hello there", summary.LastMsg)
	assert.Equal(t, int64(1700000000000), summary.LastMsgTime)
}

func TestBuildGroupConvSummary(t *testing.T) {
	msg := &model.GroupMessage{
		ID:        1,
		GroupID:   5,
		SenderID:  10,
		Content:   "group hello",
		MsgType:   model.MsgTypeText,
		GroupSeq:  3,
		CreatedAt: time.UnixMilli(1700000000000),
	}

	convID := model.BuildConvID(model.ConvTypeGroup, 5, 0)
	summary := buildGroupConvSummary(convID, msg)

	assert.Equal(t, "g_5", summary.ConvID)
	assert.Equal(t, model.ConvTypeGroup, summary.ConvType)
	assert.Equal(t, int64(5), summary.TargetID)
	assert.Equal(t, "group hello", summary.LastMsg)
	assert.Equal(t, int64(1700000000000), summary.LastMsgTime)
}

func TestPushToConnection_Offline(t *testing.T) {
	logger := zap.NewNop()
	cm := conn.NewConnectionManager()

	// User 999 is not online — push should be silently dropped
	pushToConnection(cm, logger, 999, protocol.TypeMsg, &model.InboxMessage{MsgID: 1})
	// No panic, no error — just silently dropped
}

func TestPushToConnection_Online(t *testing.T) {
	logger := zap.NewNop()
	cm := conn.NewConnectionManager()

	// Create a client connection manually (without a real websocket.Conn)
	client := makeTestClientConnection(100)
	cm.Register(100, client)

	var received []string
	go func() {
		for msg := range client.SendCh {
			received = append(received, string(msg))
		}
	}()

	pushToConnection(cm, logger, 100, protocol.TypeMsg, &model.InboxMessage{MsgID: 42})

	time.Sleep(50 * time.Millisecond)
	assert.Len(t, received, 1)

	if len(received) > 0 {
		var wsMsg model.WsMessage
		err := json.Unmarshal([]byte(received[0]), &wsMsg)
		require.NoError(t, err)
		assert.Equal(t, protocol.TypeMsg, wsMsg.Type)
	}
}

func TestPushToConnection_BufferFull(t *testing.T) {
	logger := zap.NewNop()
	cm := conn.NewConnectionManager()

	// Create a client with a small SendCh buffer
	client := &conn.ClientConnection{
		UserID:   100,
		Conn:     nil,
		SendCh:   make(chan []byte, 2), // tiny buffer
		CloseCh:  make(chan struct{}),
		LastPing: time.Now(),
	}
	cm.Register(100, client)

	// Fill the buffer
	client.SendCh <- []byte("msg1")
	client.SendCh <- []byte("msg2")

	// This push should be dropped (buffer full) — no panic
	pushToConnection(cm, logger, 100, protocol.TypeMsg, &model.InboxMessage{MsgID: 99})
	// No panic — just logged and dropped
}
