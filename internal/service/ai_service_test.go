package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/llm"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/repository"
)

// ──────────────────────────────────────────────────────
// 模拟实现
// ──────────────────────────────────────────────────────

// mockAIRedisRepo 模拟 RedisRepo 接口，用于 AI 测试。
type mockAIRedisRepo struct {
	mu          sync.Mutex
	memoryStore map[string]string // "ai_memory:{userID}:{key}" -> value
	memoryTTLs  map[string]int64
}

func newMockAIRedisRepo() *mockAIRedisRepo {
	return &mockAIRedisRepo{
		memoryStore: make(map[string]string),
		memoryTTLs:  make(map[string]int64),
	}
}

func (m *mockAIRedisRepo) SetWorkingMemory(_ context.Context, userID int64, key string, value string, ttlSeconds int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	redisKey := "ai_memory:" + strconv.FormatInt(userID, 10) + ":" + key
	m.memoryStore[redisKey] = value
	m.memoryTTLs[redisKey] = ttlSeconds
	return nil
}

func (m *mockAIRedisRepo) GetWorkingMemory(_ context.Context, userID int64, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	redisKey := "ai_memory:" + strconv.FormatInt(userID, 10) + ":" + key
	if val, ok := m.memoryStore[redisKey]; ok {
		return val, nil
	}
	return "", nil
}

func (m *mockAIRedisRepo) GetAllWorkingMemory(_ context.Context, userID int64) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]string)
	prefix := "ai_memory:" + strconv.FormatInt(userID, 10) + ":"
	for k, v := range m.memoryStore {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			shortKey := k[len(prefix):]
			result[shortKey] = v
		}
	}
	return result, nil
}

// 存根化所有其他 RedisRepo 方法

func (m *mockAIRedisRepo) WriteInbox(_ context.Context, _ int64, _ *model.InboxMessage) error { return nil }
func (m *mockAIRedisRepo) WriteOutbox(_ context.Context, _ int64, _ *model.InboxMessage) error { return nil }
func (m *mockAIRedisRepo) ReadInbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) ReadOutbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) UpdateConvList(_ context.Context, _ int64, _ string, _ string, _ int64) error {
	return nil
}
func (m *mockAIRedisRepo) GetConvList(_ context.Context, _ int64) ([]model.ConvSummary, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) IncrementUnread(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockAIRedisRepo) ClearUnread(_ context.Context, _ int64, _ string) error     { return nil }
func (m *mockAIRedisRepo) GetUnreadMap(_ context.Context, _ int64) (map[string]int64, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) SetGroupReadPos(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}
func (m *mockAIRedisRepo) GetGroupReadPos(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, nil
}
func (m *mockAIRedisRepo) GetGroupMemberships(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) GetGroupMembers(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) CheckDuplicate(_ context.Context, _ int64, _ string) (bool, error) {
	return false, nil
}
func (m *mockAIRedisRepo) TrimInbox(_ context.Context, _ int64, _ int) error            { return nil }
func (m *mockAIRedisRepo) TrimOutbox(_ context.Context, _ int64, _ int) error           { return nil }
func (m *mockAIRedisRepo) TrimInboxByTime(_ context.Context, _ int64, _ int64) error    { return nil }
func (m *mockAIRedisRepo) TrimOutboxByTime(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockAIRedisRepo) TrimConvListByTime(_ context.Context, _ int64, _ int64) error { return nil }
func (m *mockAIRedisRepo) TrimTimelineByTime(_ context.Context, _ int64, _ int64) error { return nil }
func (m *mockAIRedisRepo) ExecPrivateMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.PrivateMsgCheckResult, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) ExecGroupMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.GroupMsgCheckResult, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) ExecInboxMarkRead(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, nil
}
func (m *mockAIRedisRepo) ExecRevokeMsg(_ context.Context, _ int64, _ string, _ int64, _ string, _ int64) (bool, error) {
	return false, nil
}
func (m *mockAIRedisRepo) AddGroupMemberRedis(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockAIRedisRepo) RemoveGroupMemberRedis(_ context.Context, _ int64, _ int64) error { return nil }
func (m *mockAIRedisRepo) PublishMomentFeed(_ context.Context, _ int64, _ int64, _ int64) error { return nil }
func (m *mockAIRedisRepo) GetMomentFeed(_ context.Context, _ int64, _ int64, _ int) ([]int64, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) FanoutMomentFeed(_ context.Context, _ []int64, _ int64, _ int64, _ int) error { return nil }
func (m *mockAIRedisRepo) AddToOutbox(_ context.Context, _ int64, _ int64, _ int64, _ int) error         { return nil }
func (m *mockAIRedisRepo) MarkBigUser(_ context.Context, _ int64) error                                 { return nil }
func (m *mockAIRedisRepo) FilterBigUsers(_ context.Context, _ []int64) ([]int64, error)                 { return nil, nil }
func (m *mockAIRedisRepo) GetTimelinePage(_ context.Context, _ int64, _ int64, _ int64, _ int) ([]model.FeedEntry, error) {
	return nil, nil
}
func (m *mockAIRedisRepo) GetOutboxPage(_ context.Context, _ int64, _ int64, _ int64, _ int) ([]model.FeedEntry, error) {
	return nil, nil
}

// mockAIMySQLRepo 模拟 MySQLRepo 接口，用于 AI 测试。
type mockAIMySQLRepo struct {
	mu           sync.Mutex
	summaries    []*model.AISummary
	profileItems []*model.AIProfileItem
	privateMsgs  []*model.PrivateMessage
	insertMsgErr error // 可选的错误覆盖
}

func newMockAIMySQLRepo() *mockAIMySQLRepo {
	return &mockAIMySQLRepo{
		summaries:    make([]*model.AISummary, 0),
		profileItems: make([]*model.AIProfileItem, 0),
		privateMsgs:  make([]*model.PrivateMessage, 0),
	}
}

func (m *mockAIMySQLRepo) InsertPrivateMessage(_ context.Context, msg *model.PrivateMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.insertMsgErr != nil {
		return m.insertMsgErr
	}
	m.privateMsgs = append(m.privateMsgs, msg)
	return nil
}

func (m *mockAIMySQLRepo) CreateAISummary(_ context.Context, summary *model.AISummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summaries = append(m.summaries, summary)
	return nil
}

func (m *mockAIMySQLRepo) CreateAIProfileItem(_ context.Context, item *model.AIProfileItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileItems = append(m.profileItems, item)
	return nil
}

func (m *mockAIMySQLRepo) GetAIProfileByUser(_ context.Context, userID int64) ([]model.AIProfileItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.AIProfileItem
	for _, item := range m.profileItems {
		if item.UserID == userID {
			result = append(result, *item)
		}
	}
	// Simulate SQL ORDER BY confidence DESC
	sort.Slice(result, func(i, j int) bool {
		return result[i].Confidence > result[j].Confidence
	})
	return result, nil
}

// Stub out all other MySQLRepo methods

func (m *mockAIMySQLRepo) InsertGroupMessage(_ context.Context, _ *model.GroupMessage) error { return nil }
func (m *mockAIMySQLRepo) InsertMsgRevoked(_ context.Context, _ *model.MsgRevoked) error     { return nil }
func (m *mockAIMySQLRepo) GetUserByID(_ context.Context, _ int64) (*model.User, error)        { return nil, nil }
func (m *mockAIMySQLRepo) GetUserByUsername(_ context.Context, _ string) (*model.User, error) { return nil, nil }
func (m *mockAIMySQLRepo) CreateUser(_ context.Context, _ *model.User) error                  { return nil }
func (m *mockAIMySQLRepo) UpdateUser(_ context.Context, _ *model.User) error                  { return nil }
func (m *mockAIMySQLRepo) CreateFriendRequest(_ context.Context, _ *model.FriendRequest) error { return nil }
func (m *mockAIMySQLRepo) UpdateFriendRequest(_ context.Context, _ *model.FriendRequest) error { return nil }
func (m *mockAIMySQLRepo) GetFriendRequestByID(_ context.Context, _ int64) (*model.FriendRequest, error) {
	return nil, nil
}
func (m *mockAIMySQLRepo) GetFriendRequestsByUser(_ context.Context, _ int64) ([]model.FriendRequest, error) {
	return nil, nil
}
func (m *mockAIMySQLRepo) CreateFriendship(_ context.Context, _ *model.Friendship) error { return nil }
func (m *mockAIMySQLRepo) DeleteFriendship(_ context.Context, _ int64, _ int64) error    { return nil }
func (m *mockAIMySQLRepo) GetFriendList(_ context.Context, _ int64) ([]model.Friendship, error) {
	return nil, nil
}
func (m *mockAIMySQLRepo) IsFriend(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil }
func (m *mockAIMySQLRepo) CreateBlacklist(_ context.Context, _ *model.Blacklist) error { return nil }
func (m *mockAIMySQLRepo) DeleteBlacklist(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockAIMySQLRepo) IsBlocked(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil }
func (m *mockAIMySQLRepo) CreateGroup(_ context.Context, _ *model.Group) (int64, error) { return 0, nil }
func (m *mockAIMySQLRepo) UpdateGroup(_ context.Context, _ *model.Group) error           { return nil }
func (m *mockAIMySQLRepo) GetGroupByID(_ context.Context, _ int64) (*model.Group, error) { return nil, nil }
func (m *mockAIMySQLRepo) AddGroupMember(_ context.Context, _ *model.GroupMember) error  { return nil }
func (m *mockAIMySQLRepo) RemoveGroupMember(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockAIMySQLRepo) GetGroupMembers(_ context.Context, _ int64) ([]model.GroupMember, error) {
	return nil, nil
}
func (m *mockAIMySQLRepo) UpdateGroupMemberRole(_ context.Context, _ int, _ int, _ int) error {
	return nil
}
func (m *mockAIMySQLRepo) CreateMoment(_ context.Context, _ *model.Moment) error            { return nil }
func (m *mockAIMySQLRepo) GetMomentByID(_ context.Context, _ int64) (*model.Moment, error)  { return nil, nil }
func (m *mockAIMySQLRepo) GetMomentsByUser(_ context.Context, _ int64, _ int, _ int) ([]model.Moment, error) {
	return nil, nil
}
func (m *mockAIMySQLRepo) CreateMomentLike(_ context.Context, _ *model.MomentLike) error    { return nil }
func (m *mockAIMySQLRepo) DeleteMomentLike(_ context.Context, _ int64, _ int64) error       { return nil }
func (m *mockAIMySQLRepo) CreateMomentComment(_ context.Context, _ *model.MomentComment) error { return nil }
func (m *mockAIMySQLRepo) DeleteMomentComment(_ context.Context, _ int64) error              { return nil }
func (m *mockAIMySQLRepo) GetMomentCommentByID(_ context.Context, _ int64) (*model.MomentComment, error) { return nil, nil }
func (m *mockAIMySQLRepo) CountFriends(_ context.Context, _ int64) (int, error)              { return 0, nil }
func (m *mockAIMySQLRepo) GetMomentsByIDs(_ context.Context, _ []int64) ([]model.Moment, error) {
	return nil, nil
}

func (m *mockAIMySQLRepo) GetUserSettings(_ context.Context, _ int64) (*model.UserSettings, error) { return nil, nil }
func (m *mockAIMySQLRepo) CreateOrUpdateUserSettings(_ context.Context, _ *model.UserSettings) error { return nil }
func (m *mockAIMySQLRepo) SearchPrivateMessages(_ context.Context, _ int64, _ string, _ int, _ int) ([]model.PrivateMessage, error) {
	return nil, nil
}

// ──────────────────────────────────────────────────────
// MockLLMClient
// ──────────────────────────────────────────────────────

// LLMCaller is an interface that abstracts the LLM chat call.
// This enables mocking in tests without changing the production AIService struct.
type LLMCaller interface {
	Chat(ctx context.Context, systemPrompt string, messages []llm.ChatMessage) (string, error)
}

// MockLLMClient is a test mock that returns a configurable response.
type MockLLMClient struct {
	mu       sync.Mutex
	Response string // configurable response text
	Err      error  // configurable error
}

func NewMockLLMClient(response string) *MockLLMClient {
	return &MockLLMClient{Response: response}
}

func (m *MockLLMClient) Chat(_ context.Context, _ string, _ []llm.ChatMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Response, m.Err
}

// MultiMockLLMClient returns multiple responses in sequence (for tests with multiple LLM calls).
type MultiMockLLMClient struct {
	mu        sync.Mutex
	Responses []string
	Index     int
	Err       error
}

func (m *MultiMockLLMClient) Chat(_ context.Context, _ string, _ []llm.ChatMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return "", m.Err
	}
	if m.Index < len(m.Responses) {
		resp := m.Responses[m.Index]
		m.Index++
		return resp, nil
	}
	return "", nil
}

// ──────────────────────────────────────────────────────
// Test helpers: AIService wrapper with mock LLMCaller
// ──────────────────────────────────────────────────────

// testAIServiceWrapper wraps AIService's logic but uses an LLMCaller interface
// so we can inject mock LLM clients in tests.
type testAIServiceWrapper struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	llmCaller LLMCaller
	logger    *zap.Logger
}

// SendAIMessage delegates to the real AIService logic but uses the mock LLMCaller.
func (w *testAIServiceWrapper) SendAIMessage(ctx context.Context, userID int64, content string) (string, error) {
	now := time.Now()

	userMsg := &model.PrivateMessage{
		ID:         now.UnixMilli(),
		SenderID:   userID,
		ReceiverID: model.AI_SYSTEM_ID,
		Content:    content,
		MsgType:    model.MsgTypeText,
		CreatedAt:  now,
	}
	if err := w.mysqlRepo.InsertPrivateMessage(ctx, userMsg); err != nil {
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	workingMemory, err := w.redisRepo.GetAllWorkingMemory(ctx, userID)
	if err != nil {
		workingMemory = make(map[string]string)
	}

	profileItems, err := w.mysqlRepo.GetAIProfileByUser(ctx, userID)
	if err != nil {
		profileItems = nil
	}

	systemPrompt := buildTestSystemPrompt(workingMemory, profileItems)
	chatMessages := []llm.ChatMessage{{Role: "user", Content: content}}

	response, err := w.llmCaller.Chat(ctx, systemPrompt, chatMessages)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}
	if response == "" {
		return "", fmt.Errorf(ErrNoLLMResponse)
	}

	aiMsg := &model.PrivateMessage{
		ID:         now.UnixMilli() + 1,
		SenderID:   model.AI_SYSTEM_ID,
		ReceiverID: userID,
		Content:    response,
		MsgType:    model.MsgTypeAI,
		CreatedAt:  now,
	}
	if err := w.mysqlRepo.InsertPrivateMessage(ctx, aiMsg); err != nil {
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	exchangeJSON, _ := json.Marshal(map[string]string{
		"user":      content,
		"assistant": response,
		"timestamp": now.Format(time.RFC3339),
	})
	_ = w.redisRepo.SetWorkingMemory(ctx, userID, "last_exchange", string(exchangeJSON), workingMemoryTTLSeconds)

	return response, nil
}

// GenerateSummary delegates to the real AIService logic but uses the mock LLMCaller.
func (w *testAIServiceWrapper) GenerateSummary(ctx context.Context, userID int64, convID string) (*model.AISummary, error) {
	messages, _ := w.redisRepo.ReadInbox(ctx, userID, 0, 50)

	var convMessages []model.InboxMessage
	for _, m := range messages {
		if m.ConvID == convID {
			convMessages = append(convMessages, m)
		}
	}

	summaryPrompt := buildTestSummaryPrompt(convMessages)
	summaryResponse, err := w.llmCaller.Chat(ctx, summaryPrompt, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}

	summaryData, err := parseSummaryResponse(summaryResponse)
	if err != nil {
		summaryData = &parsedSummary{
			Topic:      summaryResponse,
			KeyPoints:  "[]",
			Conclusion: summaryResponse,
			UserIntent: "unknown",
		}
	}

	messageRangeJSON, _ := json.Marshal(map[string]int64{"count": int64(len(convMessages))})
	now := time.Now()

	summary := &model.AISummary{
		UserID:       userID,
		Topic:        summaryData.Topic,
		KeyPoints:    summaryData.KeyPoints,
		Conclusion:   summaryData.Conclusion,
		UserIntent:   summaryData.UserIntent,
		MessageRange: string(messageRangeJSON),
		CreatedAt:    now,
	}
	if err := w.mysqlRepo.CreateAISummary(ctx, summary); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrStoreSummaryFailed, err)
	}

	profilePrompt := buildTestProfileExtractionPrompt(convMessages)
	profileResponse, err := w.llmCaller.Chat(ctx, profilePrompt, nil)
	if err != nil {
		return summary, nil
	}

	profileItems, err := parseProfileResponse(profileResponse)
	if err != nil {
		return summary, nil
	}

	for _, item := range profileItems {
		item.UserID = userID
		item.UpdatedAt = now
		_ = w.mysqlRepo.CreateAIProfileItem(ctx, item)
	}

	return summary, nil
}

func buildTestSystemPrompt(workingMemory map[string]string, profileItems []model.AIProfileItem) string {
	prompt := "You are an intelligent AI assistant for a chat application. "
	if len(workingMemory) > 0 {
		prompt += "\n\nRecent context (working memory):\n"
		for k, v := range workingMemory {
			prompt += fmt.Sprintf("- %s: %s\n", k, v)
		}
	}
	if len(profileItems) > 0 {
		prompt += "\n\nKnown user profile:\n"
		for _, item := range profileItems {
			prompt += fmt.Sprintf("- %s: %s (confidence: %.2f, source: %s)\n",
				item.FieldName, item.Value, item.Confidence, item.Source)
		}
	}
	prompt += "\n\nRespond concisely and helpfully."
	return prompt
}

func buildTestSummaryPrompt(messages []model.InboxMessage) string {
	prompt := `Analyze the following conversation and extract a structured summary.
Return a JSON object with: "topic", "key_points" (JSON array), "conclusion", "user_intent".
Conversation:
`
	for _, m := range messages {
		role := "User"
		if m.FromID == model.AI_SYSTEM_ID {
			role = "AI"
		}
		prompt += fmt.Sprintf("[%s]: %s\n", role, m.Content)
	}
	return prompt
}

func buildTestProfileExtractionPrompt(messages []model.InboxMessage) string {
	prompt := `Extract user profile info from this conversation. Return a JSON array with: "field_name", "value", "confidence", "source".
`
	for _, m := range messages {
		if m.FromID != model.AI_SYSTEM_ID {
			prompt += fmt.Sprintf("[User]: %s\n", m.Content)
		}
	}
	return prompt
}

// ──────────────────────────────────────────────────────
// Tests
// ──────────────────────────────────────────────────────

func TestAI_SendAIMessage_Success(t *testing.T) {
	mysqlRepo := newMockAIMySQLRepo()
	redisRepo := newMockAIRedisRepo()
	mockLLM := NewMockLLMClient("Hello! How can I help you today?")
	logger := zap.NewNop()

	testSvc := &testAIServiceWrapper{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		llmCaller: mockLLM,
		logger:    logger,
	}

	response, err := testSvc.SendAIMessage(context.Background(), 42, "Hi, AI!")
	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you today?", response)

	// Verify two messages were stored (user + AI)
	mysqlRepo.mu.Lock()
	assert.Len(t, mysqlRepo.privateMsgs, 2)
	userMsg := mysqlRepo.privateMsgs[0]
	assert.Equal(t, int64(42), userMsg.SenderID)
	assert.Equal(t, int64(0), userMsg.ReceiverID)
	assert.Equal(t, "Hi, AI!", userMsg.Content)

	aiMsg := mysqlRepo.privateMsgs[1]
	assert.Equal(t, int64(0), aiMsg.SenderID)
	assert.Equal(t, int64(42), aiMsg.ReceiverID)
	assert.Equal(t, "Hello! How can I help you today?", aiMsg.Content)
	assert.Equal(t, model.MsgTypeAI, aiMsg.MsgType)
	mysqlRepo.mu.Unlock()
}

func TestAI_GetAIProfile(t *testing.T) {
	mysqlRepo := newMockAIMySQLRepo()

	// Pre-populate profile items
	now := time.Now()
	mysqlRepo.CreateAIProfileItem(context.Background(), &model.AIProfileItem{
		UserID:     42,
		FieldName:  "interests",
		Value:      "programming, gaming",
		Confidence: 0.85,
		Source:     "conversation_inference",
		UpdatedAt:  now,
	})
	mysqlRepo.CreateAIProfileItem(context.Background(), &model.AIProfileItem{
		UserID:     42,
		FieldName:  "language_preference",
		Value:      "English",
		Confidence: 0.9,
		Source:     "conversation_inference",
		UpdatedAt:  now,
	})

	redisRepo := newMockAIRedisRepo()
	logger := zap.NewNop()

	svc := NewAIService(mysqlRepo, redisRepo, nil, logger)

	items, err := svc.GetAIProfile(context.Background(), 42)
	require.NoError(t, err)
	assert.Len(t, items, 2)
	// Items should be ordered by confidence DESC
	assert.Equal(t, "language_preference", items[0].FieldName)
	assert.Equal(t, float32(0.9), items[0].Confidence)
	assert.Equal(t, "interests", items[1].FieldName)
	assert.Equal(t, float32(0.85), items[1].Confidence)
}

func TestAI_GenerateSummary(t *testing.T) {
	mysqlRepo := newMockAIMySQLRepo()
	redisRepo := newMockAIRedisRepo()

	// Mock LLM returns a structured summary response
	summaryJSON := `{
		"topic": "Programming Help",
		"key_points": ["User asked about Go syntax", "AI explained goroutines"],
		"conclusion": "User is learning Go programming",
		"user_intent": "learn_go"
	}`
	profileJSON := `[{
		"field_name": "expertise_level",
		"value": "beginner in Go",
		"confidence": 0.7,
		"source": "conversation_inference"
	}]`

	// We need to mock multiple LLM calls (summary + profile extraction)
	mockLLM := &MultiMockLLMClient{
		Responses: []string{summaryJSON, profileJSON},
	}

	logger := zap.NewNop()
	testSvc := &testAIServiceWrapper{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		llmCaller: mockLLM,
		logger:    logger,
	}

	summary, err := testSvc.GenerateSummary(context.Background(), 42, "p_0_42")
	require.NoError(t, err)
	assert.Equal(t, "Programming Help", summary.Topic)
	assert.Equal(t, "learn_go", summary.UserIntent)

	// Verify summary was stored
	mysqlRepo.mu.Lock()
	assert.Len(t, mysqlRepo.summaries, 1)
	mysqlRepo.mu.Unlock()

	// Verify profile item was stored
	mysqlRepo.mu.Lock()
	assert.Len(t, mysqlRepo.profileItems, 1)
	assert.Equal(t, "expertise_level", mysqlRepo.profileItems[0].FieldName)
	assert.Equal(t, "beginner in Go", mysqlRepo.profileItems[0].Value)
	mysqlRepo.mu.Unlock()
}

// ── 高并发点赞新增接口的 mock 桩 ──

func (m *mockAIMySQLRepo) GetMomentLikers(_ context.Context, _ int64) ([]int64, error)          { return nil, nil }
func (m *mockAIMySQLRepo) BatchUpsertMomentLikes(_ context.Context, _ []model.MomentLike) error { return nil }
func (m *mockAIMySQLRepo) BatchDeleteMomentLikes(_ context.Context, _ []model.MomentLikeKey) error { return nil }

func (m *mockAIRedisRepo) LikeMomentAtomic(_ context.Context, _ int64, _ int64) (bool, int64, error)   { return false, 0, nil }
func (m *mockAIRedisRepo) UnlikeMomentAtomic(_ context.Context, _ int64, _ int64) (bool, int64, error) { return false, 0, nil }
func (m *mockAIRedisRepo) EnsureMomentLikesLoaded(_ context.Context, _ int64, _ func(context.Context) ([]int64, error), _ time.Duration) error { return nil }
func (m *mockAIRedisRepo) GetMomentLikeStats(_ context.Context, _ int64, _ []int64) (map[int64]int64, map[int64]bool, error) { return nil, nil, nil }
