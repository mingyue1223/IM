package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/service"
)

// MomentHandler 提供朋友圈的 Gin HTTP 处理函数。
type MomentHandler struct {
	momentSvc *service.MomentService
}

// NewMomentHandler 创建一个包装给定 MomentService 的 MomentHandler。
func NewMomentHandler(momentSvc *service.MomentService) *MomentHandler {
	return &MomentHandler{momentSvc: momentSvc}
}

// ── 请求/响应 DTO ──

type publishMomentRequest struct {
	Content    string  `json:"content" binding:"required"`
	MediaUrls  *string `json:"media_urls,omitempty"` // 可空的 JSON 字符串
	Visibility int     `json:"visibility"`            // 1=全部, 2=好友, 3=私密
}

type publishMomentResponse struct {
	MomentID int64 `json:"moment_id"`
}

type getMomentResponse struct {
	ID         int64     `json:"id"`
	AuthorID   int64     `json:"author_id"`
	Content    string    `json:"content"`
	MediaUrls  *string   `json:"media_urls,omitempty"`
	Visibility int       `json:"visibility"`
	CreatedAt  string    `json:"created_at"` // RFC3339 格式
}

type likeMomentResponse struct {
	Ok bool `json:"ok"`
}

type unlikeMomentResponse struct {
	Ok bool `json:"ok"`
}

type commentMomentRequest struct {
	Content string `json:"content" binding:"required"`
}

type commentMomentResponse struct {
	CommentID int64 `json:"comment_id"`
}

type deleteCommentResponse struct {
	Ok bool `json:"ok"`
}

type momentFeedResponse struct {
	Moments []getMomentResponse `json:"moments"`
}

// ── 处理函数 ──

// PublishMoment 处理 POST /api/v1/moment 请求。
func (h *MomentHandler) PublishMoment(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req publishMomentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容不能为空"})
		return
	}

	// 如果未指定，默认可见性为 1（全部可见）
	visibility := req.Visibility
	if visibility == 0 {
		visibility = 1
	}

	momentID, err := h.momentSvc.PublishMoment(c.Request.Context(), userID.(int64), req.Content, req.MediaUrls, visibility)
	if err != nil {
		switch err.Error() {
		case service.ErrMomentContentEmpty:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case service.ErrInvalidVisibility:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusCreated, publishMomentResponse{MomentID: momentID})
}

// GetMoment 处理 GET /api/v1/moment/:momentID 请求。
func (h *MomentHandler) GetMoment(c *gin.Context) {
	momentIDStr := c.Param("momentID")
	momentID, err := strconv.ParseInt(momentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的动态ID"})
		return
	}

	moment, err := h.momentSvc.GetMoment(c.Request.Context(), momentID)
	if err != nil {
		switch err.Error() {
		case service.ErrMomentNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusOK, getMomentResponse{
		ID:         moment.ID,
		AuthorID:   moment.AuthorID,
		Content:    moment.Content,
		MediaUrls:  moment.MediaUrls,
		Visibility: moment.Visibility,
		CreatedAt:  moment.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// GetUserMoments 处理 GET /api/v1/moment/user/:userID 请求。
func (h *MomentHandler) GetUserMoments(c *gin.Context) {
	userIDStr := c.Param("userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	limit := 20
	offset := 0
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(c.DefaultQuery("offset", "0")); err == nil && o >= 0 {
		offset = o
	}

	moments, err := h.momentSvc.GetUserMoments(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	response := make([]getMomentResponse, 0, len(moments))
	for _, m := range moments {
		response = append(response, getMomentResponse{
			ID:         m.ID,
			AuthorID:   m.AuthorID,
			Content:    m.Content,
			MediaUrls:  m.MediaUrls,
			Visibility: m.Visibility,
			CreatedAt:  m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	c.JSON(http.StatusOK, gin.H{"moments": response})
}

// LikeMoment 处理 POST /api/v1/moment/:momentID/like 请求。
func (h *MomentHandler) LikeMoment(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	momentIDStr := c.Param("momentID")
	momentID, err := strconv.ParseInt(momentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的动态ID"})
		return
	}

	if err := h.momentSvc.LikeMoment(c.Request.Context(), userID.(int64), momentID); err != nil {
		switch err.Error() {
		case service.ErrMomentNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case service.ErrAlreadyLiked:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusOK, likeMomentResponse{Ok: true})
}

// UnlikeMoment 处理 DELETE /api/v1/moment/:momentID/like 请求。
func (h *MomentHandler) UnlikeMoment(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	momentIDStr := c.Param("momentID")
	momentID, err := strconv.ParseInt(momentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的动态ID"})
		return
	}

	if err := h.momentSvc.UnlikeMoment(c.Request.Context(), userID.(int64), momentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	c.JSON(http.StatusOK, unlikeMomentResponse{Ok: true})
}

// CommentMoment 处理 POST /api/v1/moment/:momentID/comment 请求。
func (h *MomentHandler) CommentMoment(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	momentIDStr := c.Param("momentID")
	momentID, err := strconv.ParseInt(momentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的动态ID"})
		return
	}

	var req commentMomentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容不能为空"})
		return
	}

	commentID, err := h.momentSvc.CommentMoment(c.Request.Context(), userID.(int64), momentID, req.Content)
	if err != nil {
		switch err.Error() {
		case service.ErrMomentContentEmpty:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case service.ErrMomentNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusCreated, commentMomentResponse{CommentID: commentID})
}

// DeleteComment 处理 DELETE /api/v1/moment/comment/:commentID 请求。
func (h *MomentHandler) DeleteComment(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	commentIDStr := c.Param("commentID")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的评论ID"})
		return
	}

	if err := h.momentSvc.DeleteComment(c.Request.Context(), userID.(int64), commentID); err != nil {
		switch err.Error() {
		case service.ErrNotCommentOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrCommentNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusOK, deleteCommentResponse{Ok: true})
}

// GetFeed 处理 GET /api/v1/moment/feed 请求。
func (h *MomentHandler) GetFeed(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	lastSyncTime := int64(0)
	if lst, err := strconv.ParseInt(c.DefaultQuery("last_sync_time", "0"), 10, 64); err == nil {
		lastSyncTime = lst
	}

	limit := 20
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 {
		limit = l
	}

	moments, err := h.momentSvc.GetFeed(c.Request.Context(), userID.(int64), lastSyncTime, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	response := make([]getMomentResponse, 0, len(moments))
	for _, m := range moments {
		response = append(response, getMomentResponse{
			ID:         m.ID,
			AuthorID:   m.AuthorID,
			Content:    m.Content,
			MediaUrls:  m.MediaUrls,
			Visibility: m.Visibility,
			CreatedAt:  m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	c.JSON(http.StatusOK, momentFeedResponse{Moments: response})
}

// RegisterRoutes 在给定的 Gin 路由组上注册所有朋友圈 HTTP 路由。
func (h *MomentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	moment := rg.Group("/moment")
	moment.POST("", h.PublishMoment)
	moment.GET("/:momentID", h.GetMoment)
	moment.GET("/user/:userID", h.GetUserMoments)
	moment.POST("/:momentID/like", h.LikeMoment)
	moment.DELETE("/:momentID/like", h.UnlikeMoment)
	moment.POST("/:momentID/comment", h.CommentMoment)
	moment.DELETE("/comment/:commentID", h.DeleteComment)
	moment.GET("/feed", h.GetFeed)
}
