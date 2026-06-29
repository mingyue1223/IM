package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/service"
)

// SettingsHandler provides Gin HTTP handlers for user settings endpoints.
type SettingsHandler struct {
	settingsSvc *service.SettingsService
}

// NewSettingsHandler creates a SettingsHandler wrapping the given SettingsService.
func NewSettingsHandler(settingsSvc *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{settingsSvc: settingsSvc}
}

// ── Request / response DTOs ──

type updateSettingsReq struct {
	NotificationEnabled bool   `json:"notification_enabled"`
	MsgPreviewEnabled   bool   `json:"msg_preview_enabled"`
	MuteList            string `json:"mute_list"`
}

type muteConvReq struct {
	ConvID string `json:"convId" binding:"required"`
}

// ── Handlers ──

// GetSettings handles GET /settings.
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	userID := c.GetInt64("userID")

	settings, err := h.settingsSvc.GetSettings(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateSettings handles PUT /settings.
func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req updateSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	settings := &model.UserSettings{
		NotificationEnabled: req.NotificationEnabled,
		MsgPreviewEnabled:   req.MsgPreviewEnabled,
		MuteList:            req.MuteList,
	}

	err := h.settingsSvc.UpdateSettings(c.Request.Context(), userID, settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings updated"})
}

// AddMuteConv handles POST /settings/mute.
func (h *SettingsHandler) AddMuteConv(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req muteConvReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convId is required"})
		return
	}

	err := h.settingsSvc.AddMuteConv(c.Request.Context(), userID, req.ConvID)
	if err != nil {
		switch err.Error() {
		case service.ErrMuteConvExists:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "conversation muted"})
}

// RemoveMuteConv handles DELETE /settings/mute/:convID.
func (h *SettingsHandler) RemoveMuteConv(c *gin.Context) {
	userID := c.GetInt64("userID")

	convID := c.Param("convID")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convID is required"})
		return
	}

	err := h.settingsSvc.RemoveMuteConv(c.Request.Context(), userID, convID)
	if err != nil {
		switch err.Error() {
		case service.ErrMuteConvNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "conversation unmuted"})
}

// RegisterRoutes registers all user settings HTTP routes on the given Gin router group.
func (h *SettingsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	settings := rg.Group("/settings")
	settings.GET("", h.GetSettings)
	settings.PUT("", h.UpdateSettings)
	settings.POST("/mute", h.AddMuteConv)
	settings.DELETE("/mute/:convID", h.RemoveMuteConv)
}
