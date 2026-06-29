package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	redislua "github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/protocol"
)

// ──────────────────────────────────────────────────────
// Mock implementations
// ──────────────────────────────────────────────────────

// mockRedisRepo implements repository.RedisRepo for testing.
type mockRedisRepo struct {
	mu sync.Mutex

	// Configurable return values for Lua scripts
	privateCheckResult *redislua.PrivateMsgCheckResult
	privateCheckErr    error
	groupCheckResult   *redislua.GroupMsgCheckResult
	groupCheckErr      error
	inboxMarkReadCount int64
	inboxMarkReadErr   error
	revokeResult       bool
	revokeErr          error

	// Captured data
	inboxMessages  map[int64][]model.InboxMessage // userID -> messages
	outboxMessages map[int64][]model.InboxMessage // groupID -> messages
	groupMembers   map[int64][]int64              // userID -> groupIDs
	groupReadPos   map[string]int64               // "userID:convID" -> seq
	unreadMap      map[string]int64               // convID -> count
	convList       map[int64][]model.ConvSummary  // userID -> conv summaries

	// Method call tracking
	privateCheckCalled bool
	groupCheckCalled   bool
	publishCalled      bool
	markReadCalled     bool
	revokeCalled       bool
}

func newMockRedisRepo() *mockRedisRepo {
	return &mockRedisRepo{
		inboxMessages:  make(map[int64][]model.InboxMessage),
		outboxMessages: make(map[int64][]model.InboxMessage),
		groupMembers:   make(map[int64][]int64),
		groupReadPos:   make(map[string]int64),
		unreadMap:      make(map[string]int64),
		convList:       make(map[int64][]model.ConvSummary),
	}
}

func (m *mockRedisRepo) WriteInbox(_ context.Context, userID int64, msg *model.InboxMessage) error {
	m.mu.Lock()
	m.inboxMessages[userID] = append(m.inboxMessages[userID], *msg)
	m.mu.Unlock()
	return nil
}

func (m *mockRedisRepo) WriteOutbox(_ context.Context, groupID int64, msg *model.InboxMessage) error {
	m.mu.Lock()
	m.outboxMessages[groupID] = append(m.outboxMessages[groupID], *msg)
	m.mu.Unlock()
	return nil
}

func (m *mockRedisRepo) ReadInbox(_ context.Context, userID int64, lastSyncTime int64, batchSize int) ([]model.InboxMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.inboxMessages[userID]
	var result []model.InboxMessage
	for _, msg := range msgs {
		if msg.Timestamp >= lastSyncTime {
			result = append(result, msg)
			if len(result) >= batchSize {
				break
			}
		}
	}
	return result, nil
}

func (m *mockRedisRepo) ReadOutbox(_ context.Context, groupID int64, lastSyncTime int64, limit int) ([]model.InboxMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.outboxMessages[groupID]
	var result []model.InboxMessage
	for _, msg := range msgs {
		if msg.Timestamp >= lastSyncTime {
			result = append(result, msg)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockRedisRepo) UpdateConvList(_ context.Context, userID int64, convID string, summary string, timestamp int64) error {
	m.mu.Lock()
	m.convList[userID] = append(m.convList[userID], model.ConvSummary{ConvID: convID, LastMsgTime: timestamp})
	m.mu.Unlock()
	return nil
}

func (m *mockRedisRepo) GetConvList(_ context.Context, userID int64) ([]model.ConvSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.convList[userID], nil
}

func (m *mockRedisRepo) IncrementUnread(_ context.Context, userID int64, convID string) error {
	m.mu.Lock()
	m.unreadMap[convID]++
	m.mu.Unlock()
	return nil
}

func (m *mockRedisRepo) ClearUnread(_ context.Context, userID int64, convID string) error {
	m.mu.Lock()
	m.unreadMap[convID] = 0
	m.mu.Unlock()
	return nil
}

func (m *mockRedisRepo) GetUnreadMap(_ context.Context, userID int64) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]int64)
	for k, v := range m.unreadMap {
		result[k] = v
	}
	return result, nil
}

func (m *mockRedisRepo) SetGroupReadPos(_ context.Context, userID int64, convID string, seq int64) error {
	m.mu.Lock()
	m.groupReadPos[convID] = seq
	m.mu.Unlock()
	return nil
}

func (m *mockRedisRepo) GetGroupReadPos(_ context.Context, userID int64, convID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.groupReadPos[convID], nil
}

func (m *mockRedisRepo) GetGroupMemberships(_ context.Context, userID int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.groupMembers[userID], nil
}

func (m *mockRedisRepo) GetGroupMembers(_ context.Context, groupID int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return empty list — group member fan-out is handled by the MQ consumer, not MsgService
	return []int64{}, nil
}

func (m *mockRedisRepo) CheckDuplicate(_ context.Context, userID int64, clientMsgID string) (bool, error) {
	return false, nil
}

func (m *mockRedisRepo) TrimInbox(_ context.Context, userID int64, maxCount int) error { return nil }
func (m *mockRedisRepo) TrimOutbox(_ context.Context, groupID int64, maxCount int) error { return nil }
func (m *mockRedisRepo) TrimInboxByTime(_ context.Context, userID int64, beforeTimestamp int64) error { return nil }
func (m *mockRedisRepo) TrimOutboxByTime(_ context.Context, groupID int64, beforeTimestamp int64) error { return nil }
func (m *mockRedisRepo) TrimConvListByTime(_ context.Context, userID int64, beforeTimestamp int64) error { return nil }
func (m *mockRedisRepo) TrimTimelineByTime(_ context.Context, userID int64, beforeTimestamp int64) error { return nil }

func (m *mockRedisRepo) ExecPrivateMsgCheck(_ context.Context, senderID, receiverID int64, clientMsgID string) (*redislua.PrivateMsgCheckResult, error) {
	m.mu.Lock()
	m.privateCheckCalled = true
	m.mu.Unlock()
	return m.privateCheckResult, m.privateCheckErr
}

func (m *mockRedisRepo) ExecGroupMsgCheck(_ context.Context, groupID, senderID int64, clientMsgID string) (*redislua.GroupMsgCheckResult, error) {
	m.mu.Lock()
	m.groupCheckCalled = true
	m.mu.Unlock()
	return m.groupCheckResult, m.groupCheckErr
}

func (m *mockRedisRepo) ExecInboxMarkRead(_ context.Context, userID int64, convID string) (int64, error) {
	m.mu.Lock()
	m.markReadCalled = true
	m.mu.Unlock()
	return m.inboxMarkReadCount, m.inboxMarkReadErr
}

func (m *mockRedisRepo) ExecRevokeMsg(_ context.Context, userID int64, convID string, msgID int64, revokeMsgJSON string, nowTimestamp int64) (bool, error) {
	m.mu.Lock()
	m.revokeCalled = true
	m.mu.Unlock()
	return m.revokeResult, m.revokeErr
}

func (m *mockRedisRepo) PublishMomentFeed(_ context.Context, _ int64, _ int64, _ int64) error {
	return nil
}

func (m *mockRedisRepo) GetMomentFeed(_ context.Context, _ int64, _ int64, _ int) ([]int64, error) {
	return nil, nil
}

// mockMQRepo implements repository.MQRepo for testing.
type mockMQRepo struct {
	mu             sync.Mutex
	privateMsgs    []*model.PrivateMessage
	groupMsgs      []*model.GroupMessage
	publishErr     error
	publishPrivate  bool
	publishGroup    bool
}

func newMockMQRepo() *mockMQRepo {
	return &mockMQRepo{}
}

func (m *mockMQRepo) PublishPrivateMsg(_ context.Context, msg *model.PrivateMessage) error {
	m.mu.Lock()
	m.privateMsgs = append(m.privateMsgs, msg)
	m.publishPrivate = true
	m.mu.Unlock()
	return m.publishErr
}

func (m *mockMQRepo) PublishGroupMsg(_ context.Context, msg *model.GroupMessage) error {
	m.mu.Lock()
	m.groupMsgs = append(m.groupMsgs, msg)
	m.publishGroup = true
	m.mu.Unlock()
	return m.publishErr
}

func (m *mockMQRepo) PublishMomentPush(_ context.Context, _ *model.Moment) error {
	return nil
}

// ──────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────

func testLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

// drainSendCh reads one message from a client's SendCh with a timeout.
func drainSendCh(client *conn.ClientConnection) ([]byte, bool) {
	select {
	case msg := <-client.SendCh:
		return msg, true
	case <-time.After(2 * time.Second):
		return nil, false
	}
}

// decodeWsMessage parses raw bytes into a WsMessage envelope.
func decodeWsMessage(raw []byte) (*model.WsMessage, error) {
	return protocol.DecodeMsg(raw)
}

// ──────────────────────────────────────────────────────
// Tests
// ──────────────────────────────────────────────────────

func TestPrivateMsgSend_Success(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = &redislua.PrivateMsgCheckResult{
		ErrCode:   redislua.PMErrOK,
		MsgID:     1001,
		IsOnline:  true,
		IsFriend:  true,
		IsBlocked: false,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	// Register sender and receiver connections
	sender := conn.NewClientConnection(1, nil)
	receiver := conn.NewClientConnection(2, nil)
	cm.Register(1, sender)
	cm.Register(2, receiver)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	// Build request
	req := model.SendMessage{
		ClientMsgID: "client-msg-1",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	// Verify: Lua check was called
	assert.True(t, redisMock.privateCheckCalled)

	// Verify: MQ publish was called
	assert.True(t, mqMock.publishPrivate)
	assert.Len(t, mqMock.privateMsgs, 1)
	assert.Equal(t, int64(1001), mqMock.privateMsgs[0].ID)
	assert.Equal(t, int64(1), mqMock.privateMsgs[0].SenderID)
	assert.Equal(t, int64(2), mqMock.privateMsgs[0].ReceiverID)
	assert.Equal(t, "hello", mqMock.privateMsgs[0].Content)

	// Verify: serverAck was sent to sender
	msg, ok := drainSendCh(sender)
	assert.True(t, ok, "expected serverAck on sender SendCh")
	wsMsg, err := decodeWsMessage(msg)
	assert.NoError(t, err)
	assert.Equal(t, protocol.TypeServerAck, wsMsg.Type)

	var ack model.ServerAck
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &ack))
	assert.Equal(t, "client-msg-1", ack.ClientMsgID)
	assert.Equal(t, int64(1001), ack.ServerMsgID)

	// Verify: message was pushed to receiver (isOnline=true)
	msg, ok = drainSendCh(receiver)
	assert.True(t, ok, "expected message push on receiver SendCh")
	wsMsg, err = decodeWsMessage(msg)
	assert.NoError(t, err)
	assert.Equal(t, protocol.TypeMsg, wsMsg.Type)

	var inboxMsg model.InboxMessage
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &inboxMsg))
	assert.Equal(t, int64(1001), inboxMsg.MsgID)
	assert.Equal(t, "p_1_2", inboxMsg.ConvID)
	assert.Equal(t, model.ConvTypePrivate, inboxMsg.ConvType)
	assert.Equal(t, int64(1), inboxMsg.FromID)
	assert.Equal(t, int64(2), inboxMsg.ToID)
	assert.Equal(t, "hello", inboxMsg.Content)
}

func TestPrivateMsgSend_NotFriend(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = &redislua.PrivateMsgCheckResult{
		ErrCode:   redislua.PMErrNotFriend,
		MsgID:     0,
		IsOnline:  false,
		IsFriend:  false,
		IsBlocked: false,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-2",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	// Verify: error was sent to sender
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, err := decodeWsMessage(msg)
	assert.NoError(t, err)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, redislua.CodePMNotFriend, wsErr.Code)
	assert.Equal(t, ErrNotFriend, wsErr.Message)

	// Verify: MQ publish was NOT called
	assert.False(t, mqMock.publishPrivate)
}

func TestPrivateMsgSend_Blocked(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = &redislua.PrivateMsgCheckResult{
		ErrCode:   redislua.PMErrBlocked,
		MsgID:     0,
		IsOnline:  false,
		IsFriend:  true,
		IsBlocked: true,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-3",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, redislua.CodePMBlocked, wsErr.Code)
	assert.Equal(t, ErrBlocked, wsErr.Message)
}

func TestPrivateMsgSend_Duplicate(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = &redislua.PrivateMsgCheckResult{
		ErrCode: redislua.PMErrDuplicate,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-dup",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, redislua.CodePMDuplicate, wsErr.Code)
}

func TestPrivateMsgSend_ReceiverOffline(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = &redislua.PrivateMsgCheckResult{
		ErrCode:   redislua.PMErrOK,
		MsgID:     2001,
		IsOnline:  false, // receiver offline
		IsFriend:  true,
		IsBlocked: false,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)
	// No receiver registered in cm (offline)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-offline",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello offline",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	// Verify: serverAck sent to sender
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeServerAck, wsMsg.Type)

	// Verify: MQ still published (will be delivered via sync when receiver comes online)
	assert.True(t, mqMock.publishPrivate)
}

func TestPrivateMsgSend_LuaError(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = nil
	redisMock.privateCheckErr = context.DeadlineExceeded
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-err",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	// Verify: internal error sent to sender
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 500, wsErr.Code)
}

func TestGroupMsgSend_Success(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.groupCheckResult = &redislua.GroupMsgCheckResult{
		ErrCode:  redislua.GMErrOK,
		MsgID:    3001,
		GroupSeq: 10,
		IsMember: true,
		IsMuted:  false,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-group-1",
		ConvType:    model.ConvTypeGroup,
		ToID:        100, // groupID
		MsgType:     model.MsgTypeText,
		Content:     "group hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	// Verify: Lua check was called
	assert.True(t, redisMock.groupCheckCalled)

	// Verify: MQ publish was called
	assert.True(t, mqMock.publishGroup)
	assert.Len(t, mqMock.groupMsgs, 1)
	assert.Equal(t, int64(3001), mqMock.groupMsgs[0].ID)
	assert.Equal(t, int64(100), mqMock.groupMsgs[0].GroupID)
	assert.Equal(t, int64(10), mqMock.groupMsgs[0].GroupSeq)

	// Verify: serverAck with groupSeq sent to sender
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, err := decodeWsMessage(msg)
	assert.NoError(t, err)
	assert.Equal(t, protocol.TypeServerAck, wsMsg.Type)

	var ack model.ServerAck
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &ack))
	assert.Equal(t, "client-msg-group-1", ack.ClientMsgID)
	assert.Equal(t, int64(3001), ack.ServerMsgID)
	assert.Equal(t, int64(10), ack.GroupSeq)
}

func TestGroupMsgSend_NotMember(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.groupCheckResult = &redislua.GroupMsgCheckResult{
		ErrCode:  redislua.GMErrNotMember,
		MsgID:    0,
		GroupSeq: 0,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-group-notmem",
		ConvType:    model.ConvTypeGroup,
		ToID:        100,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, redislua.CodeGMNotMember, wsErr.Code)
	assert.Equal(t, ErrNotMember, wsErr.Message)
}

func TestGroupMsgSend_Muted(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.groupCheckResult = &redislua.GroupMsgCheckResult{
		ErrCode:  redislua.GMErrMuted,
		MsgID:    0,
		GroupSeq: 0,
		IsMember: true,
		IsMuted:  true,
	}
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-group-muted",
		ConvType:    model.ConvTypeGroup,
		ToID:        100,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, redislua.CodeGMMuted, wsErr.Code)
	assert.Equal(t, ErrMuted, wsErr.Message)
}

func TestDeliverAck(t *testing.T) {
	redisMock := newMockRedisRepo()
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	ack := model.DeliverAck{ServerMsgID: 1001}
	data, _ := json.Marshal(ack)

	// DeliverAck is informational — just verify it doesn't panic
	svc.HandleDeliverAck(1, data)
	// No error should be sent, no state change expected
}

func TestReadAck_Success(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.inboxMarkReadCount = 3
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.ReadAck{ConvID: "p_1_2"}
	data, _ := json.Marshal(req)

	svc.HandleReadAck(1, data)

	// Verify: Lua mark read was called
	assert.True(t, redisMock.markReadCalled)
}

func TestReadAck_InvalidFormat(t *testing.T) {
	redisMock := newMockRedisRepo()
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	// Send invalid JSON
	svc.HandleReadAck(1, []byte("invalid json"))

	// Verify: error was sent
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)
}

func TestSyncReq_WithMessages(t *testing.T) {
	redisMock := newMockRedisRepo()
	// Pre-populate inbox for user 1
	redisMock.inboxMessages[1] = []model.InboxMessage{
		{MsgID: 1, ConvID: "p_1_2", ConvType: 1, FromID: 2, ToID: 1, Content: "hi", Timestamp: 1100000},
		{MsgID: 2, ConvID: "p_1_2", ConvType: 1, FromID: 2, ToID: 1, Content: "there", Timestamp: 1200000},
	}
	// User 1 is in group 100
	redisMock.groupMembers[1] = []int64{100}
	// Group outbox for group 100
	redisMock.outboxMessages[100] = []model.InboxMessage{
		{MsgID: 3, ConvID: "g_100", ConvType: 2, FromID: 5, ToID: 100, Content: "group msg", Timestamp: 1300000, GroupSeq: 5},
	}
	// User 1's read position for g_100 is seq 3
	redisMock.groupReadPos["g_100"] = 3
	// Unread map
	redisMock.unreadMap = map[string]int64{"p_1_2": 2, "g_100": 1}

	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	client := conn.NewClientConnection(1, nil)
	cm.Register(1, client)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SyncReq{LastSyncTime: 1000000, BatchSize: 50}
	data, _ := json.Marshal(req)

	svc.HandleSyncReq(1, data)

	// Should receive two messages: SyncBatch and ConvSync
	msg1, ok := drainSendCh(client)
	assert.True(t, ok, "expected SyncBatch on SendCh")
	wsMsg1, err := decodeWsMessage(msg1)
	assert.NoError(t, err)
	assert.Equal(t, protocol.TypeSyncBatch, wsMsg1.Type)

	var batch model.SyncBatch
	assert.NoError(t, json.Unmarshal(wsMsg1.Data, &batch))
	// Should include inbox messages + group messages where groupSeq > lastReadSeq
	assert.True(t, len(batch.Messages) >= 2) // at least the 2 private + 1 group message

	// Verify the group message with groupSeq=5 > lastReadSeq=3 was included
	groupFound := false
	for _, m := range batch.Messages {
		if m.ConvID == "g_100" && m.GroupSeq == 5 {
			groupFound = true
		}
	}
	assert.True(t, groupFound, "group message with groupSeq > lastReadSeq should be included")

	msg2, ok := drainSendCh(client)
	assert.True(t, ok, "expected ConvSync on SendCh")
	wsMsg2, _ := decodeWsMessage(msg2)
	assert.Equal(t, protocol.TypeConvSync, wsMsg2.Type)

	var convSync model.ConvSync
	assert.NoError(t, json.Unmarshal(wsMsg2.Data, &convSync))
	assert.Equal(t, int64(2), convSync.UnreadMap["p_1_2"])
}

func TestSyncReq_DefaultBatchSize(t *testing.T) {
	redisMock := newMockRedisRepo()
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	client := conn.NewClientConnection(1, nil)
	cm.Register(1, client)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	// Send SyncReq with batchSize=0 (should default to 50)
	req := model.SyncReq{LastSyncTime: 0, BatchSize: 0}
	data, _ := json.Marshal(req)

	svc.HandleSyncReq(1, data)

	// Should still receive SyncBatch
	msg, ok := drainSendCh(client)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeSyncBatch, wsMsg.Type)

	var batch model.SyncBatch
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &batch))
	// When no messages, syncTime is set to current server time (non-zero)
	assert.True(t, batch.SyncTime > 0, "syncTime should be non-zero even with no messages")
	assert.False(t, batch.HasMore)
}

func TestRevokeMsg_Success_Private(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.revokeResult = true
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	receiver := conn.NewClientConnection(2, nil)
	cm.Register(1, sender)
	cm.Register(2, receiver)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.RevokeMsgReq{ConvID: "p_1_2", ServerMsgID: 1001}
	data, _ := json.Marshal(req)

	svc.HandleRevokeMsg(1, data)

	// Verify: Lua revoke was called
	assert.True(t, redisMock.revokeCalled)

	// Verify: sender got msgRevoked notification
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeMsgRevoked, wsMsg.Type)

	var notif model.RevokedNotification
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &notif))
	assert.Equal(t, "p_1_2", notif.ConvID)
	assert.Equal(t, int64(1001), notif.ServerMsgID)
	assert.Equal(t, int64(1), notif.OperatorID)

	// Verify: receiver got msgRevoked notification too
	msg, ok = drainSendCh(receiver)
	assert.True(t, ok)
	wsMsg, _ = decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeMsgRevoked, wsMsg.Type)
}

func TestRevokeMsg_Fail(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.revokeResult = false // 2-minute window expired or not authorized
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.RevokeMsgReq{ConvID: "p_1_2", ServerMsgID: 1001}
	data, _ := json.Marshal(req)

	svc.HandleRevokeMsg(1, data)

	// Verify: error was sent
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 403, wsErr.Code)
	assert.Equal(t, ErrRevokeFail, wsErr.Message)
}

func TestRevokeMsg_Group(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.revokeResult = true
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.RevokeMsgReq{ConvID: "g_100", ServerMsgID: 3001}
	data, _ := json.Marshal(req)

	svc.HandleRevokeMsg(1, data)

	// Verify: Lua revoke was called
	assert.True(t, redisMock.revokeCalled)

	// Verify: sender got msgRevoked notification
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeMsgRevoked, wsMsg.Type)

	// For group chat, no direct push to other members (they discover via sync)
}

func TestRevokeMsg_InvalidConvID(t *testing.T) {
	redisMock := newMockRedisRepo()
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.RevokeMsgReq{ConvID: "x_invalid", ServerMsgID: 1001}
	data, _ := json.Marshal(req)

	svc.HandleRevokeMsg(1, data)

	// Verify: error was sent
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 400, wsErr.Code)
	assert.Equal(t, "invalid convId format", wsErr.Message)
}

func TestHandleSendMessage_InvalidJSON(t *testing.T) {
	redisMock := newMockRedisRepo()
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	svc.HandleSendMessage(1, []byte("not json"))

	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 400, wsErr.Code)
}

func TestHandleSendMessage_UnknownConvType(t *testing.T) {
	redisMock := newMockRedisRepo()
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-unknown",
		ConvType:    99, // unknown
		ToID:        2,
		MsgType:     1,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 400, wsErr.Code)
	assert.Equal(t, "unknown convType", wsErr.Message)
}

func TestPrivateMsgSend_MQPublishFail(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.privateCheckResult = &redislua.PrivateMsgCheckResult{
		ErrCode:   redislua.PMErrOK,
		MsgID:     1001,
		IsOnline:  false,
		IsFriend:  true,
		IsBlocked: false,
	}
	mqMock := newMockMQRepo()
	mqMock.publishErr = context.DeadlineExceeded
	cm := conn.NewConnectionManager()
	logger := testLogger()

	sender := conn.NewClientConnection(1, nil)
	cm.Register(1, sender)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.SendMessage{
		ClientMsgID: "client-msg-mqfail",
		ConvType:    model.ConvTypePrivate,
		ToID:        2,
		MsgType:     model.MsgTypeText,
		Content:     "hello",
		Timestamp:   1000000,
	}
	data, _ := json.Marshal(req)

	svc.HandleSendMessage(1, data)

	// Verify: error was sent to sender
	msg, ok := drainSendCh(sender)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 500, wsErr.Code)
	assert.Equal(t, "message publish failed", wsErr.Message)
}

func TestGetOtherPartyID(t *testing.T) {
	assert.Equal(t, int64(2), getOtherPartyID("p_1_2", 1))
	assert.Equal(t, int64(1), getOtherPartyID("p_1_2", 2))
	assert.Equal(t, int64(0), getOtherPartyID("g_100", 1))     // group convID returns 0
	assert.Equal(t, int64(0), getOtherPartyID("invalid", 1))    // invalid format
	assert.Equal(t, int64(0), getOtherPartyID("p_1", 1))        // missing second ID
}

func TestReadAck_LuaError(t *testing.T) {
	redisMock := newMockRedisRepo()
	redisMock.inboxMarkReadErr = context.DeadlineExceeded
	mqMock := newMockMQRepo()
	cm := conn.NewConnectionManager()
	logger := testLogger()

	client := conn.NewClientConnection(1, nil)
	cm.Register(1, client)

	svc := NewMsgService(redisMock, mqMock, cm, logger)

	req := model.ReadAck{ConvID: "p_1_2"}
	data, _ := json.Marshal(req)

	svc.HandleReadAck(1, data)

	// Verify: error was sent
	msg, ok := drainSendCh(client)
	assert.True(t, ok)
	wsMsg, _ := decodeWsMessage(msg)
	assert.Equal(t, protocol.TypeError, wsMsg.Type)

	var wsErr model.WsError
	assert.NoError(t, json.Unmarshal(wsMsg.Data, &wsErr))
	assert.Equal(t, 500, wsErr.Code)
}
