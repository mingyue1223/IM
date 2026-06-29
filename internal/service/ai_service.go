package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/llm"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── Error constants ──

const (
	ErrLLMCallFailed    = "LLM call failed"
	ErrNoLLMResponse    = "LLM returned empty response"
	ErrStoreMsgFailed   = "failed to store message"
	ErrStoreSummaryFailed = "failed to store summary"
	ErrStoreProfileFailed = "failed to store profile item"
)

// Default TTL for working memory entries (30 minutes)
const workingMemoryTTLSeconds = 1800

// AIService handles AI-related operations: chat, profile, and summary generation.
// It implements the 4-layer memory architecture:
//   - Layer 0: Raw messages (MySQL private_messages)
//   - Layer 1: Structured summaries (MySQL ai_summaries)
//   - Layer 2: Confidence-graded profile (MySQL ai_user_profiles)
//   - Layer 3: Working memory (Redis ai_memory:{userID}:{key})
type AIService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	llmClient *llm.LLMClient
	logger    *zap.Logger
}

// NewAIService creates an AIService with all required dependencies.
func NewAIService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, llmClient *llm.LLMClient, logger *zap.Logger) *AIService {
	return &AIService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		llmClient: llmClient,
		logger:    logger,
	}
}

// ──────────────────────────────────────────────────────
// SendAIMessage
// ──────────────────────────────────────────────────────

// SendAIMessage handles a user sending a message to the AI assistant.
// It stores the user message, builds context from memory and profile,
// calls the LLM, stores the AI response, and updates working memory.
func (s *AIService) SendAIMessage(ctx context.Context, userID int64, content string) (string, error) {
	now := time.Now()

	// 1. Store user message as PrivateMessage (senderID=userID, receiverID=AI_SYSTEM_ID=0)
	userMsg := &model.PrivateMessage{
		SenderID:   userID,
		ReceiverID: model.AI_SYSTEM_ID,
		Content:    content,
		MsgType:    model.MsgTypeText,
		CreatedAt:  now,
	}
	// Generate a snowflake-style ID from timestamp (simplified for AI messages)
	userMsg.ID = now.UnixMilli()
	if err := s.mysqlRepo.InsertPrivateMessage(ctx, userMsg); err != nil {
		s.logger.Error("store user AI message", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	// 2. Get working memory (Layer 3) for context
	workingMemory, err := s.redisRepo.GetAllWorkingMemory(ctx, userID)
	if err != nil {
		s.logger.Warn("get working memory failed", zap.Error(err))
		workingMemory = make(map[string]string) // continue without memory
	}

	// 3. Get user profile (Layer 2) for personalization
	profileItems, err := s.mysqlRepo.GetAIProfileByUser(ctx, userID)
	if err != nil {
		s.logger.Warn("get AI profile failed", zap.Error(err))
		profileItems = nil // continue without profile
	}

	// 4. Build prompt with memory context + profile
	systemPrompt := s.buildSystemPrompt(workingMemory, profileItems)

	chatMessages := []llm.ChatMessage{
		{Role: "user", Content: content},
	}

	// 5. Call LLM to generate response
	response, err := s.llmClient.Chat(ctx, systemPrompt, chatMessages)
	if err != nil {
		s.logger.Error("LLM chat call failed", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}
	if response == "" {
		return "", fmt.Errorf(ErrNoLLMResponse)
	}

	// 6. Store AI response as PrivateMessage (senderID=AI_SYSTEM_ID=0, receiverID=userID)
	aiMsg := &model.PrivateMessage{
		ID:         now.UnixMilli() + 1, // offset to avoid ID collision with user msg
		SenderID:   model.AI_SYSTEM_ID,
		ReceiverID: userID,
		Content:    response,
		MsgType:    model.MsgTypeAI,
		CreatedAt:  now,
	}
	if err := s.mysqlRepo.InsertPrivateMessage(ctx, aiMsg); err != nil {
		s.logger.Error("store AI response message", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	// 7. Update working memory with new exchange
	exchangeJSON, _ := json.Marshal(map[string]string{
		"user":      content,
		"assistant": response,
		"timestamp": now.Format(time.RFC3339),
	})
	if err := s.redisRepo.SetWorkingMemory(ctx, userID, "last_exchange", string(exchangeJSON), workingMemoryTTLSeconds); err != nil {
		s.logger.Warn("set working memory failed", zap.Error(err))
	}

	s.logger.Debug("AI chat completed",
		zap.Int64("userID", userID),
		zap.Int("responseLen", len(response)),
	)

	return response, nil
}

// ──────────────────────────────────────────────────────
// GetAIProfile
// ──────────────────────────────────────────────────────

// GetAIProfile returns the Layer 2 profile items for a user.
func (s *AIService) GetAIProfile(ctx context.Context, userID int64) ([]model.AIProfileItem, error) {
	items, err := s.mysqlRepo.GetAIProfileByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get AI profile: %w", err)
	}
	return items, nil
}

// ──────────────────────────────────────────────────────
// GenerateSummary
// ──────────────────────────────────────────────────────

// GenerateSummary analyzes a conversation and creates a Layer 1 summary,
// then extracts profile updates and creates Layer 2 profile items.
func (s *AIService) GenerateSummary(ctx context.Context, userID int64, convID string) (*model.AISummary, error) {
	// 1. Fetch recent messages from Redis inbox
	messages, err := s.redisRepo.ReadInbox(ctx, userID, 0, 50)
	if err != nil {
		s.logger.Warn("read inbox for summary failed", zap.Error(err))
		messages = nil
	}

	// Filter messages for the specific conversation
	var convMessages []model.InboxMessage
	for _, m := range messages {
		if m.ConvID == convID {
			convMessages = append(convMessages, m)
		}
	}

	// 2. Call LLM to extract topic, key points, conclusion, user intent
	summaryPrompt := s.buildSummaryPrompt(convMessages)
	summaryResponse, err := s.llmClient.Chat(ctx, summaryPrompt, nil)
	if err != nil {
		s.logger.Error("LLM summary call failed", zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}

	// Parse the LLM response as structured summary data
	summaryData, err := parseSummaryResponse(summaryResponse)
	if err != nil {
		s.logger.Warn("parse summary response failed", zap.Error(err))
		// Use raw response as topic if parsing fails
		summaryData = &parsedSummary{
			Topic:      summaryResponse,
			KeyPoints:  "[]",
			Conclusion: summaryResponse,
			UserIntent: "unknown",
		}
	}

	// Build message_range JSON
	messageRangeJSON, _ := json.Marshal(map[string]int64{
		"convID": 0, // placeholder; in production, use actual message IDs
		"count":  int64(len(convMessages)),
	})

	now := time.Now()

	// 3. Store as AISummary in MySQL (Layer 1)
	summary := &model.AISummary{
		UserID:       userID,
		Topic:        summaryData.Topic,
		KeyPoints:    summaryData.KeyPoints,
		Conclusion:   summaryData.Conclusion,
		UserIntent:   summaryData.UserIntent,
		MessageRange: string(messageRangeJSON),
		CreatedAt:    now,
	}
	if err := s.mysqlRepo.CreateAISummary(ctx, summary); err != nil {
		s.logger.Error("store AI summary failed", zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrStoreSummaryFailed, err)
	}

	// 4. Extract profile updates from conversation
	profilePrompt := s.buildProfileExtractionPrompt(convMessages)
	profileResponse, err := s.llmClient.Chat(ctx, profilePrompt, nil)
	if err != nil {
		s.logger.Warn("LLM profile extraction failed", zap.Error(err))
		// Summary was stored successfully; continue without profile updates
		return summary, nil
	}

	// 5. Create/update AIProfileItem entries (Layer 2)
	profileItems, err := parseProfileResponse(profileResponse)
	if err != nil {
		s.logger.Warn("parse profile response failed", zap.Error(err))
		return summary, nil
	}

	for _, item := range profileItems {
		item.UserID = userID
		item.UpdatedAt = now
		if err := s.mysqlRepo.CreateAIProfileItem(ctx, item); err != nil {
			s.logger.Warn("store AI profile item failed",
				zap.String("field", item.FieldName),
				zap.Error(err),
			)
		}
	}

	s.logger.Debug("AI summary generated",
		zap.Int64("userID", userID),
		zap.String("convID", convID),
		zap.String("topic", summaryData.Topic),
	)

	return summary, nil
}

// ──────────────────────────────────────────────────────
// Prompt builders
// ──────────────────────────────────────────────────────

// buildSystemPrompt creates a system prompt enriched with working memory and profile data.
func (s *AIService) buildSystemPrompt(workingMemory map[string]string, profileItems []model.AIProfileItem) string {
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

	prompt += "\n\nRespond concisely and helpfully. Adapt your tone based on the user's profile."
	return prompt
}

// buildSummaryPrompt creates a prompt for extracting a structured summary from conversation messages.
func (s *AIService) buildSummaryPrompt(messages []model.InboxMessage) string {
	prompt := `Analyze the following conversation and extract a structured summary.
Return a JSON object with these fields:
- "topic": a short topic title (max 100 chars)
- "key_points": a JSON array of strings listing key discussion points
- "conclusion": a brief conclusion (max 500 chars)
- "user_intent": what the user seems to want (max 200 chars)

Conversation messages:
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

// buildProfileExtractionPrompt creates a prompt for extracting user profile items from conversation.
func (s *AIService) buildProfileExtractionPrompt(messages []model.InboxMessage) string {
	prompt := `Analyze the following conversation and extract profile information about the user.
Return a JSON array of objects with these fields:
- "field_name": the profile attribute (e.g., "interests", "language_preference", "communication_style", "expertise_level")
- "value": the inferred value (max 200 chars)
- "confidence": a float between 0.0 and 1.0 indicating confidence in the inference
- "source": "conversation_inference"

Conversation messages:
`
	for _, m := range messages {
		if m.FromID != model.AI_SYSTEM_ID {
			prompt += fmt.Sprintf("[User]: %s\n", m.Content)
		}
	}
	return prompt
}

// ──────────────────────────────────────────────────────
// Response parsers
// ──────────────────────────────────────────────────────

type parsedSummary struct {
	Topic      string
	KeyPoints  string // JSON string
	Conclusion string
	UserIntent string
}

func parseSummaryResponse(raw string) (*parsedSummary, error) {
	// Try to parse as JSON first
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("parse summary JSON: %w", err)
	}

	topic, _ := data["topic"].(string)
	conclusion, _ := data["conclusion"].(string)
	userIntent, _ := data["user_intent"].(string)

	// key_points might be a JSON array string or an actual array
	var keyPoints string
	kp, ok := data["key_points"]
	if ok {
		kpJSON, err := json.Marshal(kp)
		if err != nil {
			keyPoints = "[]"
		} else {
			keyPoints = string(kpJSON)
		}
	} else {
		keyPoints = "[]"
	}

	return &parsedSummary{
		Topic:      topic,
		KeyPoints:  keyPoints,
		Conclusion: conclusion,
		UserIntent: userIntent,
	}, nil
}

func parseProfileResponse(raw string) ([]*model.AIProfileItem, error) {
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("parse profile JSON: %w", err)
	}

	result := make([]*model.AIProfileItem, 0, len(items))
	for _, item := range items {
		fieldName, _ := item["field_name"].(string)
		value, _ := item["value"].(string)
		source, _ := item["source"].(string)
		if source == "" {
			source = "conversation_inference"
		}

		confidence := float32(0.5) // default
		if c, ok := item["confidence"].(float64); ok {
			confidence = float32(c)
		}

		if fieldName == "" || value == "" {
			continue
		}

		result = append(result, &model.AIProfileItem{
			FieldName:  fieldName,
			Value:      value,
			Confidence: confidence,
			Source:     source,
		})
	}
	return result, nil
}

// HandleAiStream processes an AI chat/stream request via WebSocket.
// It parses the incoming message content and delegates to SendAIMessage.
func (s *AIService) HandleAiStream(userID int64, data []byte) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("failed to parse AI stream request", zap.Error(err))
		return
	}
	if req.Content == "" {
		s.logger.Warn("empty AI stream content", zap.Int64("userID", userID))
		return
	}

	ctx := context.Background()
	_, err := s.SendAIMessage(ctx, userID, req.Content)
	if err != nil {
		s.logger.Error("SendAIMessage failed in HandleAiStream",
			zap.Int64("userID", userID),
			zap.Error(err),
		)
	}
}
