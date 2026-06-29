package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/service"
)

// MsgOpHandler provides Gin HTTP handlers for message operation endpoints.
type MsgOpHandler struct {
	msgOpSvc *service.MsgOpService
}

// NewMsgOpHandler creates a MsgOpHandler wrapping the given MsgOpService.
func NewMsgOpHandler(msgOpSvc *service.MsgOpService) *MsgOpHandler {
	return &MsgOpHandler{msgOpSvc: msgOpSvc}
}

// ── Request / response DTOs ──

type revokeMsgReq struct {
	ConvID string `json:"convId" binding:"required"`
	MsgID  int64  `json:"msgId" binding:"required"`
}

type searchMsgReq struct {
	Query  string `form:"q" binding:"required"`
	Limit  int    `form:"limit,default=20"`
	Offset int    `form:"offset,default=0"`
}

// ── Handlers ──

// RevokeMessage handles POST /msg/revoke.
func (h *MsgOpHandler) RevokeMessage(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req revokeMsgReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convId and msgId are required"})
		return
	}

	err := h.msgOpSvc.RevokeMessage(c.Request.Context(), userID, req.ConvID, req.MsgID)
	if err != nil {
		switch err.Error() {
		case service.ErrMsgNotRevocable:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case service.ErrMsgRevokeNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "message revoked"})
}

// DeleteMessage handles DELETE /msg/:msgID.
func (h *MsgOpHandler) DeleteMessage(c *gin.Context) {
	userID := c.GetInt64("userID")

	msgIDStr := c.Param("msgID")
	msgID, err := strconv.ParseInt(msgIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid msgID"})
		return
	}

	convID := c.Query("convId")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "convId query param is required"})
		return
	}

	err = h.msgOpSvc.DeleteMessage(c.Request.Context(), userID, convID, msgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "message deleted"})
}

// SearchMessages handles GET /msg/search.
func (h *MsgOpHandler) SearchMessages(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req searchMsgReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query (q) is required"})
		return
	}

	msgs, err := h.msgOpSvc.SearchMessages(c.Request.Context(), userID, req.Query, req.Limit, req.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

// RegisterRoutes registers all message operation HTTP routes on the given Gin router group.
func (h *MsgOpHandler) RegisterRoutes(rg *gin.RouterGroup) {
	msg := rg.Group("/msg")
	msg.POST("/revoke", h.RevokeMessage)
	msg.DELETE("/:msgID", h.DeleteMessage)
	msg.GET("/search", h.SearchMessages)
}
