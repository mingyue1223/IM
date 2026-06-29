package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/service"
)

// AIHandler provides Gin HTTP handlers for AI endpoints.
type AIHandler struct {
	aiSvc *service.AIService
}

// NewAIHandler creates an AIHandler wrapping the given AIService.
func NewAIHandler(aiSvc *service.AIService) *AIHandler {
	return &AIHandler{aiSvc: aiSvc}
}

// ── Request / response DTOs ──

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

// ── Handlers ──

// Chat handles POST /ai/chat.
// It receives a user message and returns an AI response.
func (h *AIHandler) Chat(c *gin.Context) {
	var req aiChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	// Extract userID from JWT middleware context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.aiSvc.SendAIMessage(c.Request.Context(), userID.(int64), req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, aiChatResponse{Response: response})
}

// Profile handles GET /ai/profile.
// It returns the user's AI profile items (Layer 2 memory).
func (h *AIHandler) Profile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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

// Summary handles POST /ai/summary/:convID.
// It generates an AI summary for the specified conversation.
func (h *AIHandler) Summary(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	convID := c.Param("convID")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convID is required"})
		return
	}

	summary, err := h.aiSvc.GenerateSummary(c.Request.Context(), userID.(int64), convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	convIDInt, _ := strconv.ParseInt(convID, 10, 64)

	c.JSON(http.StatusOK, gin.H{
		"id":         convIDInt,
		"topic":      summary.Topic,
		"key_points": summary.KeyPoints,
		"conclusion": summary.Conclusion,
		"user_intent": summary.UserIntent,
		"message_range": summary.MessageRange,
	})
}

// RegisterRoutes registers all AI HTTP routes on the given Gin router group.
func (h *AIHandler) RegisterRoutes(rg *gin.RouterGroup) {
	ai := rg.Group("/ai")
	ai.POST("/chat", h.Chat)
	ai.GET("/profile", h.Profile)
	ai.POST("/summary/:convID", h.Summary)
}
