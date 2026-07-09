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

// Chat godoc
// @Summary      AI对话
// @Description  向AI发送消息并获取非流式回复
// @Tags         AI
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  aiChatRequest  true  "聊天消息"
// @Success      200   {object}  ApiResponse{data=aiChatResponse}  "对话成功"
// @Failure      400   {object}  ApiResponse  "参数错误"
// @Failure      401   {object}  ApiResponse  "未授权"
// @Router       /ai/chat [post]
func (h *AIHandler) Chat(c *gin.Context) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeMissingParam, "content 字段为必填项")
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		Error(c, http.StatusUnauthorized, CodeUnauthorized, "未授权")
		return
	}

	response, err := h.aiSvc.SendAIMessage(c.Request.Context(), userID.(int64), req.Content)
	if err != nil {
		ServiceError(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, aiChatResponse{Response: response})
}

// ChatStream godoc
// @Summary      AI流式聊天
// @Description  以SSE流式方式与AI助手对话，实时返回生成内容
// @Tags         AI
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        body  body  aiChatRequest  true  "聊天消息"
// @Success      200   {string}  string  "SSE事件流"
// @Failure      400   {object}  ApiResponse  "参数错误"
// @Failure      401   {object}  ApiResponse  "未授权"
// @Router       /ai/chat/stream [post]
func (h *AIHandler) ChatStream(c *gin.Context) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeMissingParam, "content 字段为必填项")
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		Error(c, http.StatusUnauthorized, CodeUnauthorized, "未授权")
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

// Profile godoc
// @Summary      获取AI画像
// @Description  获取当前用户的AI画像条目（第二层记忆）
// @Tags         AI
// @Produce      json
// @Security     BearerAuth
// @Success      200   {object}  ApiResponse{data=aiProfileResponse}  "获取成功"
// @Failure      401   {object}  ApiResponse  "未授权"
// @Router       /ai/profile [get]
func (h *AIHandler) Profile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		Error(c, http.StatusUnauthorized, CodeUnauthorized, "未授权")
		return
	}

	items, err := h.aiSvc.GetAIProfile(c.Request.Context(), userID.(int64))
	if err != nil {
		ServiceError(c, http.StatusInternalServerError, err.Error())
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

	Success(c, aiProfileResponse{Items: dtoItems})
}

// Summary godoc
// @Summary      生成会话摘要
// @Description  为指定会话生成AI摘要
// @Tags         AI
// @Produce      json
// @Security     BearerAuth
// @Param        convID  path  string  true  "会话ID"
// @Success      200     {object}  ApiResponse{data=aiSummaryResponse}  "生成成功"
// @Failure      400     {object}  ApiResponse  "参数错误"
// @Failure      401     {object}  ApiResponse  "未授权"
// @Router       /ai/summary/{convID} [post]
func (h *AIHandler) Summary(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		Error(c, http.StatusUnauthorized, CodeUnauthorized, "未授权")
		return
	}

	convID := c.Param("convID")
	if convID == "" {
		Error(c, http.StatusBadRequest, CodeMissingParam, "convID 为必填参数")
		return
	}

	summary, err := h.aiSvc.GenerateSummary(c.Request.Context(), userID.(int64), convID)
	if err != nil {
		ServiceError(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, gin.H{
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
