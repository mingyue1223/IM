package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/llm"
	"github.com/goim/goim/internal/service"
)

// AIHandler 为 AI 端点提供 Gin HTTP 处理器。
type AIHandler struct {
	aiSvc *service.AIService
}

// NewAIHandler 创建一个包装了给定 AIService 的 AIHandler。
func NewAIHandler(aiSvc *service.AIService) *AIHandler {
	return &AIHandler{aiSvc: aiSvc}
}

// ── 请求 / 响应 DTO ──

type aiChatRequest struct {
	Content string `json:"content" binding:"required"`
}

type aiChatResponse struct {
	Response string `json:"response"`
}

type aiProfileResponse struct {
	Items []aiProfileItemDTO `json:"items"`
}

type aiProfileItemDTO struct {
	FieldName  string  `json:"field_name"`
	Value      string  `json:"value"`
	Confidence float32 `json:"confidence"`
	Source     string  `json:"source"`
}

type aiSummaryResponse struct {
	Topic        string `json:"topic"`
	KeyPoints    string `json:"key_points"`
	Conclusion   string `json:"conclusion"`
	UserIntent   string `json:"user_intent"`
	MessageRange string `json:"message_range"`
}

// ── SSE 流式 DTO ──

// sseChunkEvent 是流式传输中每条 SSE 数据行所发送的 JSON 载荷。
type sseChunkEvent struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Done             bool   `json:"done,omitempty"`
	FullResponse     string `json:"full_response,omitempty"`
	Error            string `json:"error,omitempty"`
}

// ── 处理器 ──

// Chat 处理 POST /ai/chat。
// 它接收用户消息并返回 AI 响应（非流式）。
func (h *AIHandler) Chat(c *gin.Context) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content 字段为必填项"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	response, err := h.aiSvc.SendAIMessage(c.Request.Context(), userID.(int64), req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, aiChatResponse{Response: response})
}

// ChatStream 处理 POST /ai/chat/stream。
// 它以 SSE 事件形式发送 AI 响应，将 LLM 返回的每个块实时流式传输。
func (h *AIHandler) ChatStream(c *gin.Context) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content 字段为必填项"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 刷新响应头
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	// 定义写入 SSE 事件的块回调函数
	onChunk := func(chunk llm.StreamChunk) {
		event := sseChunkEvent{
			Content:          chunk.Content,
			ReasoningContent: chunk.ReasoningContent,
			Done:             chunk.Done,
		}

		data, err := json.Marshal(event)
		if err != nil {
			// 将错误作为 SSE 事件发送
			errData, _ := json.Marshal(sseChunkEvent{Error: fmt.Sprintf("序列化块数据失败: %v", err)})
			fmt.Fprintf(c.Writer, "data: %s\n\n", errData)
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	// 调用流式服务
	fullResponse, err := h.aiSvc.SendAIMessageStream(c.Request.Context(), userID.(int64), req.Content, onChunk)
	if err != nil {
		// 发送最终的错误事件
		errData, _ := json.Marshal(sseChunkEvent{Error: err.Error(), Done: true})
		fmt.Fprintf(c.Writer, "data: %s\n\n", errData)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	// 发送包含完整组装响应的最终事件
	finalEvent := sseChunkEvent{
		Done:         true,
		FullResponse: fullResponse,
	}
	data, _ := json.Marshal(finalEvent)
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Profile 处理 GET /ai/profile。
// 它返回用户的 AI 画像条目（第二层记忆）。
func (h *AIHandler) Profile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	items, err := h.aiSvc.GetAIProfile(c.Request.Context(), userID.(int64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dtoItems := make([]aiProfileItemDTO, 0, len(items))
	for _, item := range items {
		dtoItems = append(dtoItems, aiProfileItemDTO{
			FieldName:  item.FieldName,
			Value:      item.Value,
			Confidence: item.Confidence,
			Source:     item.Source,
		})
	}

	c.JSON(http.StatusOK, aiProfileResponse{Items: dtoItems})
}

// Summary 处理 POST /ai/summary/:convID。
// 它为指定会话生成 AI 摘要。
func (h *AIHandler) Summary(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	convID := c.Param("convID")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convID 为必填参数"})
		return
	}

	summary, err := h.aiSvc.GenerateSummary(c.Request.Context(), userID.(int64), convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            convID,
		"topic":         summary.Topic,
		"key_points":    summary.KeyPoints,
		"conclusion":    summary.Conclusion,
		"user_intent":   summary.UserIntent,
		"message_range": summary.MessageRange,
	})
}

// RegisterRoutes 在给定的 Gin 路由组上注册所有 AI HTTP 路由。
func (h *AIHandler) RegisterRoutes(rg *gin.RouterGroup) {
	ai := rg.Group("/ai")
	ai.POST("/chat", h.Chat)
	ai.POST("/chat/stream", h.ChatStream)
	ai.GET("/profile", h.Profile)
	ai.POST("/summary/:convID", h.Summary)
}
