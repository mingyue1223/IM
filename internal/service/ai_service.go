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

// ── 错误常量 ──

const (
	ErrLLMCallFailed      = "LLM调用失败"
	ErrNoLLMResponse      = "LLM返回空响应"
	ErrStoreMsgFailed     = "消息存储失败"
	ErrStoreSummaryFailed = "摘要存储失败"
	ErrStoreProfileFailed = "用户画像条目存储失败"
)

// 工作记忆条目的默认TTL（30分钟）
const workingMemoryTTLSeconds = 1800

// AIService 处理AI相关操作：聊天、用户画像和摘要生成。
// 它实现了四层记忆架构：
//   - 第0层：原始消息（MySQL private_messages）
//   - 第1层：结构化摘要（MySQL ai_summaries）
//   - 第2层：带置信度评分的用户画像（MySQL ai_user_profiles）
//   - 第3层：工作记忆（Redis ai_memory:{userID}:{key}）
type AIService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	llmClient *llm.LLMClient
	logger    *zap.Logger
}

// NewAIService 使用所有必需的依赖项创建一个AIService。
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

// SendAIMessage 处理用户向AI助手发送消息。
// 它存储用户消息，从记忆和用户画像构建上下文，
// 调用LLM，存储AI响应，并更新工作记忆。
func (s *AIService) SendAIMessage(ctx context.Context, userID int64, content string) (string, error) {
	now := time.Now()

	// 1. 将用户消息存储为PrivateMessage（senderID=userID, receiverID=AI_SYSTEM_ID=0）
	userMsg := &model.PrivateMessage{
		SenderID:   userID,
		ReceiverID: model.AI_SYSTEM_ID,
		Content:    content,
		MsgType:    model.MsgTypeText,
		CreatedAt:  now,
	}
	// 根据时间戳生成雪花风格的ID（针对AI消息的简化版本）
	userMsg.ID = now.UnixMilli()
	if err := s.mysqlRepo.InsertPrivateMessage(ctx, userMsg); err != nil {
		s.logger.Error("存储用户AI消息失败", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	// 2. 获取工作记忆（第3层）作为上下文
	workingMemory, err := s.redisRepo.GetAllWorkingMemory(ctx, userID)
	if err != nil {
		s.logger.Warn("获取工作记忆失败", zap.Error(err))
		workingMemory = make(map[string]string) // 无记忆时继续执行
	}

	// 3. 获取用户画像（第2层）用于个性化
	profileItems, err := s.mysqlRepo.GetAIProfileByUser(ctx, userID)
	if err != nil {
		s.logger.Warn("获取AI用户画像失败", zap.Error(err))
		profileItems = nil // 无画像时继续执行
	}

	// 4. 使用记忆上下文 + 用户画像构建提示词
	systemPrompt := s.buildSystemPrompt(workingMemory, profileItems)

	chatMessages := []llm.ChatMessage{
		{Role: "user", Content: content},
	}

	// 5. 调用LLM生成响应
	response, err := s.llmClient.Chat(ctx, systemPrompt, chatMessages)
	if err != nil {
		s.logger.Error("LLM聊天调用失败", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}
	if response == "" {
		return "", fmt.Errorf(ErrNoLLMResponse)
	}

	// 6. 将AI响应存储为PrivateMessage（senderID=AI_SYSTEM_ID=0, receiverID=userID）
	aiMsg := &model.PrivateMessage{
		ID:         now.UnixMilli() + 1, // 偏移以避免与用户消息ID冲突
		SenderID:   model.AI_SYSTEM_ID,
		ReceiverID: userID,
		Content:    response,
		MsgType:    model.MsgTypeAI,
		CreatedAt:  now,
	}
	if err := s.mysqlRepo.InsertPrivateMessage(ctx, aiMsg); err != nil {
		s.logger.Error("存储AI响应消息失败", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	// 7. 使用新的对话更新工作记忆
	exchangeJSON, _ := json.Marshal(map[string]string{
		"user":      content,
		"assistant": response,
		"timestamp": now.Format(time.RFC3339),
	})
	if err := s.redisRepo.SetWorkingMemory(ctx, userID, "last_exchange", string(exchangeJSON), workingMemoryTTLSeconds); err != nil {
		s.logger.Warn("设置工作记忆失败", zap.Error(err))
	}

	s.logger.Debug("AI聊天完成",
		zap.Int64("userID", userID),
		zap.Int("responseLen", len(response)),
	)

	return response, nil
}

// ──────────────────────────────────────────────────────
// SendAIMessageStream
// ──────────────────────────────────────────────────────

// SendAIMessageStream 处理流式AI聊天请求。
// 它存储用户消息，构建上下文，以流式方式调用LLM，
// 并通过onChunk转发每个片段。流完成后，存储
// 组装好的AI响应并更新工作记忆。
func (s *AIService) SendAIMessageStream(ctx context.Context, userID int64, content string, onChunk llm.ChunkCallback) (string, error) {
	now := time.Now()

	// 1. 存储用户消息
	userMsg := &model.PrivateMessage{
		SenderID:   userID,
		ReceiverID: model.AI_SYSTEM_ID,
		Content:    content,
		MsgType:    model.MsgTypeText,
		CreatedAt:  now,
	}
	userMsg.ID = now.UnixMilli()
	if err := s.mysqlRepo.InsertPrivateMessage(ctx, userMsg); err != nil {
		s.logger.Error("存储用户AI消息失败（流式）", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	// 2. 获取工作记忆（第3层）
	workingMemory, err := s.redisRepo.GetAllWorkingMemory(ctx, userID)
	if err != nil {
		s.logger.Warn("获取工作记忆失败", zap.Error(err))
		workingMemory = make(map[string]string)
	}

	// 3. 获取用户画像（第2层）
	profileItems, err := s.mysqlRepo.GetAIProfileByUser(ctx, userID)
	if err != nil {
		s.logger.Warn("获取AI用户画像失败", zap.Error(err))
		profileItems = nil
	}

	// 4. 构建提示词
	systemPrompt := s.buildSystemPrompt(workingMemory, profileItems)

	chatMessages := []llm.ChatMessage{
		{Role: "user", Content: content},
	}

	// 5. 以流式方式调用LLM——每个片段通过onChunk转发
	response, err := s.llmClient.ChatStream(ctx, systemPrompt, chatMessages, onChunk)
	if err != nil {
		s.logger.Error("LLM流式调用失败", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}
	if response == "" {
		return "", fmt.Errorf(ErrNoLLMResponse)
	}

	// 6. 存储组装好的AI响应
	aiMsg := &model.PrivateMessage{
		ID:         now.UnixMilli() + 1,
		SenderID:   model.AI_SYSTEM_ID,
		ReceiverID: userID,
		Content:    response,
		MsgType:    model.MsgTypeAI,
		CreatedAt:  now,
	}
	if err := s.mysqlRepo.InsertPrivateMessage(ctx, aiMsg); err != nil {
		s.logger.Error("存储AI响应消息失败（流式）", zap.Error(err))
		return "", fmt.Errorf("%s: %w", ErrStoreMsgFailed, err)
	}

	// 7. 更新工作记忆
	exchangeJSON, _ := json.Marshal(map[string]string{
		"user":      content,
		"assistant": response,
		"timestamp": now.Format(time.RFC3339),
	})
	if err := s.redisRepo.SetWorkingMemory(ctx, userID, "last_exchange", string(exchangeJSON), workingMemoryTTLSeconds); err != nil {
		s.logger.Warn("设置工作记忆失败（流式）", zap.Error(err))
	}

	s.logger.Debug("AI流式聊天完成",
		zap.Int64("userID", userID),
		zap.Int("responseLen", len(response)),
	)

	return response, nil
}

// ──────────────────────────────────────────────────────
// GetAIProfile
// ──────────────────────────────────────────────────────

// GetAIProfile 返回用户的第2层用户画像条目。
func (s *AIService) GetAIProfile(ctx context.Context, userID int64) ([]model.AIProfileItem, error) {
	items, err := s.mysqlRepo.GetAIProfileByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取AI用户画像: %w", err)
	}
	return items, nil
}

// ──────────────────────────────────────────────────────
// GenerateSummary
// ──────────────────────────────────────────────────────

// GenerateSummary 分析对话并创建第1层摘要，
// 然后提取用户画像更新并创建第2层用户画像条目。
func (s *AIService) GenerateSummary(ctx context.Context, userID int64, convID string) (*model.AISummary, error) {
	// 1. 从Redis收件箱获取最近的消息
	messages, err := s.redisRepo.ReadInbox(ctx, userID, 0, 50)
	if err != nil {
		s.logger.Warn("读取收件箱以生成摘要失败", zap.Error(err))
		messages = nil
	}

	// 筛选特定会话的消息
	var convMessages []model.InboxMessage
	for _, m := range messages {
		if m.ConvID == convID {
			convMessages = append(convMessages, m)
		}
	}

	// 2. 调用LLM提取主题、关键点、结论、用户意图
	summaryPrompt := s.buildSummaryPrompt(convMessages)
	summaryResponse, err := s.llmClient.Chat(ctx, summaryPrompt, nil)
	if err != nil {
		s.logger.Error("LLM摘要调用失败", zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrLLMCallFailed, err)
	}

	// 将LLM响应解析为结构化摘要数据
	summaryData, err := parseSummaryResponse(summaryResponse)
	if err != nil {
		s.logger.Warn("解析摘要响应失败", zap.Error(err))
		// 解析失败时使用原始响应作为主题
		summaryData = &parsedSummary{
			Topic:      summaryResponse,
			KeyPoints:  "[]",
			Conclusion: summaryResponse,
			UserIntent: "unknown",
		}
	}

	// 构建 message_range JSON
	messageRangeJSON, _ := json.Marshal(map[string]int64{
		"convID": 0, // 占位符；生产环境中使用实际消息ID
		"count":  int64(len(convMessages)),
	})

	now := time.Now()

	// 3. 将AISummary存储到MySQL（第1层）
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
		s.logger.Error("存储AI摘要失败", zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrStoreSummaryFailed, err)
	}

	// 4. 从对话中提取用户画像更新
	profilePrompt := s.buildProfileExtractionPrompt(convMessages)
	profileResponse, err := s.llmClient.Chat(ctx, profilePrompt, nil)
	if err != nil {
		s.logger.Warn("LLM用户画像提取失败", zap.Error(err))
		// 摘要已成功存储；无画像更新时继续执行
		return summary, nil
	}

	// 5. 创建/更新 AIProfileItem 条目（第2层）
	profileItems, err := parseProfileResponse(profileResponse)
	if err != nil {
		s.logger.Warn("解析用户画像响应失败", zap.Error(err))
		return summary, nil
	}

	for _, item := range profileItems {
		item.UserID = userID
		item.UpdatedAt = now
		if err := s.mysqlRepo.CreateAIProfileItem(ctx, item); err != nil {
			s.logger.Warn("存储AI用户画像条目失败",
				zap.String("field", item.FieldName),
				zap.Error(err),
			)
		}
	}

	s.logger.Debug("AI摘要已生成",
		zap.Int64("userID", userID),
		zap.String("convID", convID),
		zap.String("topic", summaryData.Topic),
	)

	return summary, nil
}

// ──────────────────────────────────────────────────────
// 提示词构建器
// ──────────────────────────────────────────────────────

// buildSystemPrompt 创建一个包含工作记忆和用户画像数据的系统提示词。
func (s *AIService) buildSystemPrompt(workingMemory map[string]string, profileItems []model.AIProfileItem) string {
	prompt := "你是一个智能AI助手，为聊天应用提供服务。 "

	if len(workingMemory) > 0 {
		prompt += "\n\n近期上下文（工作记忆）：\n"
		for k, v := range workingMemory {
			prompt += fmt.Sprintf("- %s: %s\n", k, v)
		}
	}

	if len(profileItems) > 0 {
		prompt += "\n\n已知用户画像：\n"
		for _, item := range profileItems {
			prompt += fmt.Sprintf("- %s: %s（置信度: %.2f, 来源: %s）\n",
				item.FieldName, item.Value, item.Confidence, item.Source)
		}
	}

	prompt += "\n\n请简洁而有帮助地回答。根据用户画像调整你的语气。"
	return prompt
}

// buildSummaryPrompt 创建一个用于从对话消息中提取结构化摘要的提示词。
func (s *AIService) buildSummaryPrompt(messages []model.InboxMessage) string {
	prompt := `分析以下对话并提取结构化摘要。
返回一个包含以下字段的JSON对象：
- "topic"：简短的主题标题（最多100个字符）
- "key_points"：列出关键讨论点的JSON字符串数组
- "conclusion"：简要结论（最多500个字符）
- "user_intent"：用户似乎想要什么（最多200个字符）

对话消息：
`
	for _, m := range messages {
		role := "用户"
		if m.FromID == model.AI_SYSTEM_ID {
			role = "AI"
		}
		prompt += fmt.Sprintf("[%s]: %s\n", role, m.Content)
	}
	return prompt
}

// buildProfileExtractionPrompt 创建一个用于从对话中提取用户画像条目的提示词。
func (s *AIService) buildProfileExtractionPrompt(messages []model.InboxMessage) string {
	prompt := `分析以下对话并提取关于该用户的画像信息。
返回一个包含以下字段的JSON对象数组：
- "field_name"：画像属性（例如："interests", "language_preference", "communication_style", "expertise_level"）
- "value"：推断出的值（最多200个字符）
- "confidence"：介于0.0到1.0之间的浮点数，表示对推断的置信度
- "source"："conversation_inference"

对话消息：
`
	for _, m := range messages {
		if m.FromID != model.AI_SYSTEM_ID {
			prompt += fmt.Sprintf("[用户]: %s\n", m.Content)
		}
	}
	return prompt
}

// ──────────────────────────────────────────────────────
// 响应解析器
// ──────────────────────────────────────────────────────

type parsedSummary struct {
	Topic      string
	KeyPoints  string // JSON字符串
	Conclusion string
	UserIntent string
}

func parseSummaryResponse(raw string) (*parsedSummary, error) {
	// 首先尝试以JSON格式解析
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("解析摘要JSON: %w", err)
	}

	topic, _ := data["topic"].(string)
	conclusion, _ := data["conclusion"].(string)
	userIntent, _ := data["user_intent"].(string)

	// key_points 可能是一个JSON数组字符串，也可能是一个实际的数组
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
		return nil, fmt.Errorf("解析用户画像JSON: %w", err)
	}

	result := make([]*model.AIProfileItem, 0, len(items))
	for _, item := range items {
		fieldName, _ := item["field_name"].(string)
		value, _ := item["value"].(string)
		source, _ := item["source"].(string)
		if source == "" {
			source = "conversation_inference"
		}

		confidence := float32(0.5) // 默认值
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

// HandleAiStream 处理通过WebSocket传入的AI聊天/流式请求。
// 它解析传入的消息内容并委托给SendAIMessage。
func (s *AIService) HandleAiStream(userID int64, data []byte) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("解析AI流式请求失败", zap.Error(err))
		return
	}
	if req.Content == "" {
		s.logger.Warn("AI流式内容为空", zap.Int64("userID", userID))
		return
	}

	ctx := context.Background()
	_, err := s.SendAIMessage(ctx, userID, req.Content)
	if err != nil {
		s.logger.Error("HandleAiStream中SendAIMessage失败",
			zap.Int64("userID", userID),
			zap.Error(err),
		)
	}
}
