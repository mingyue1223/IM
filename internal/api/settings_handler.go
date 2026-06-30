package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/service"
)

// SettingsHandler 提供用户设置端点的 Gin HTTP 处理函数。
type SettingsHandler struct {
	settingsSvc *service.SettingsService
}

// NewSettingsHandler 创建一个包装给定 SettingsService 的 SettingsHandler。
func NewSettingsHandler(settingsSvc *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{settingsSvc: settingsSvc}
}

// ── 请求 / 响应 DTO ──

type updateSettingsReq struct {
	NotificationEnabled bool   `json:"notification_enabled"`
	MsgPreviewEnabled   bool   `json:"msg_preview_enabled"`
	MuteList            string `json:"mute_list"`
}

type muteConvReq struct {
	ConvID string `json:"convId" binding:"required"`
}

// ── 处理函数 ──

// GetSettings 处理 GET /settings 请求。
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	userID := c.GetInt64("userID")

	settings, err := h.settingsSvc.GetSettings(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateSettings 处理 PUT /settings 请求。
func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req updateSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	settings := &model.UserSettings{
		NotificationEnabled: req.NotificationEnabled,
		MsgPreviewEnabled:   req.MsgPreviewEnabled,
		MuteList:            req.MuteList,
	}

	err := h.settingsSvc.UpdateSettings(c.Request.Context(), userID, settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "设置已更新"})
}

// AddMuteConv 处理 POST /settings/mute 请求。
func (h *SettingsHandler) AddMuteConv(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req muteConvReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convId 为必填项"})
		return
	}

	err := h.settingsSvc.AddMuteConv(c.Request.Context(), userID, req.ConvID)
	if err != nil {
		switch err.Error() {
		case service.ErrMuteConvExists:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "会话已静音"})
}

// RemoveMuteConv 处理 DELETE /settings/mute/:convID 请求。
func (h *SettingsHandler) RemoveMuteConv(c *gin.Context) {
	userID := c.GetInt64("userID")

	convID := c.Param("convID")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convID 为必填项"})
		return
	}

	err := h.settingsSvc.RemoveMuteConv(c.Request.Context(), userID, convID)
	if err != nil {
		switch err.Error() {
		case service.ErrMuteConvNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "会话已取消静音"})
}

// RegisterRoutes 在给定的 Gin 路由组上注册所有用户设置 HTTP 路由。
func (h *SettingsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	settings := rg.Group("/settings")
	settings.GET("", h.GetSettings)
	settings.PUT("", h.UpdateSettings)
	settings.POST("/mute", h.AddMuteConv)
	settings.DELETE("/mute/:convID", h.RemoveMuteConv)
}
