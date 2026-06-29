package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/service"
)

// FriendHandler provides Gin HTTP handlers for friend-related endpoints.
type FriendHandler struct {
	friendSvc *service.FriendService
}

// NewFriendHandler creates a FriendHandler wrapping the given FriendService.
func NewFriendHandler(friendSvc *service.FriendService) *FriendHandler {
	return &FriendHandler{friendSvc: friendSvc}
}

// ── Request / response DTOs ──

type sendFriendRequestReq struct {
	ToUserID int64  `json:"to_user_id" binding:"required"`
	Message  string `json:"message"`
}

type sendFriendRequestResp struct {
	RequestID int64  `json:"request_id"`
	FromUserID int64 `json:"from_user_id"`
	ToUserID   int64 `json:"to_user_id"`
	Status     int   `json:"status"`
}

type acceptFriendRequestReq struct {
	RequestID int64 `json:"request_id" binding:"required"`
}

type acceptFriendRequestResp struct {
	UserID   int64 `json:"user_id"`
	FriendID int64 `json:"friend_id"`
}

type rejectFriendRequestReq struct {
	RequestID int64 `json:"request_id" binding:"required"`
}

type blockUserReq struct {
	BlockedID int64 `json:"blocked_id" binding:"required"`
}

type unblockUserReq struct {
	BlockedID int64 `json:"blocked_id" binding:"required"`
}

// ── Handlers ──

// SendFriendRequest handles POST /friend/request.
func (h *FriendHandler) SendFriendRequest(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req sendFriendRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to_user_id is required"})
		return
	}

	friendReq, err := h.friendSvc.SendFriendRequest(c.Request.Context(), userID, req.ToUserID, req.Message)
	if err != nil {
		switch err.Error() {
		case service.ErrSelfRequest:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case service.ErrAlreadyFriends:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case service.ErrFriendBlocked:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrDuplicateRequest:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusCreated, sendFriendRequestResp{
		RequestID:  friendReq.ID,
		FromUserID: friendReq.FromUserID,
		ToUserID:   friendReq.ToUserID,
		Status:     friendReq.Status,
	})
}

// AcceptFriendRequest handles POST /friend/accept.
func (h *FriendHandler) AcceptFriendRequest(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req acceptFriendRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id is required"})
		return
	}

	fs, err := h.friendSvc.AcceptFriendRequest(c.Request.Context(), userID, req.RequestID)
	if err != nil {
		switch err.Error() {
		case service.ErrRequestNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case service.ErrNotRequestTarget:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, acceptFriendRequestResp{
		UserID:   fs.UserID,
		FriendID: fs.FriendID,
	})
}

// RejectFriendRequest handles POST /friend/reject.
func (h *FriendHandler) RejectFriendRequest(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req rejectFriendRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id is required"})
		return
	}

	err := h.friendSvc.RejectFriendRequest(c.Request.Context(), userID, req.RequestID)
	if err != nil {
		switch err.Error() {
		case service.ErrRequestNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case service.ErrNotRequestTarget:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "friend request rejected"})
}

// GetFriendRequests handles GET /friend/requests.
func (h *FriendHandler) GetFriendRequests(c *gin.Context) {
	userID := c.GetInt64("userID")

	requests, err := h.friendSvc.GetFriendRequests(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"requests": requests})
}

// GetFriendList handles GET /friend/list.
func (h *FriendHandler) GetFriendList(c *gin.Context) {
	userID := c.GetInt64("userID")

	friends, err := h.friendSvc.GetFriendList(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"friends": friends})
}

// DeleteFriend handles DELETE /friend/:friendID.
func (h *FriendHandler) DeleteFriend(c *gin.Context) {
	userID := c.GetInt64("userID")

	friendIDStr := c.Param("friendID")
	friendID, err := strconv.ParseInt(friendIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid friendID"})
		return
	}

	err = h.friendSvc.DeleteFriend(c.Request.Context(), userID, friendID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "friend deleted"})
}

// BlockUser handles POST /friend/block.
func (h *FriendHandler) BlockUser(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req blockUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blocked_id is required"})
		return
	}

	err := h.friendSvc.BlockUser(c.Request.Context(), userID, req.BlockedID)
	if err != nil {
		switch err.Error() {
		case service.ErrAlreadyBlocked:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user blocked"})
}

// UnblockUser handles POST /friend/unblock.
func (h *FriendHandler) UnblockUser(c *gin.Context) {
	userID := c.GetInt64("userID")

	var req unblockUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blocked_id is required"})
		return
	}

	err := h.friendSvc.UnblockUser(c.Request.Context(), userID, req.BlockedID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user unblocked"})
}

// RegisterRoutes registers all friend HTTP routes on the given Gin router group.
func (h *FriendHandler) RegisterRoutes(rg *gin.RouterGroup) {
	friend := rg.Group("/friend")
	friend.POST("/request", h.SendFriendRequest)
	friend.POST("/accept", h.AcceptFriendRequest)
	friend.POST("/reject", h.RejectFriendRequest)
	friend.GET("/requests", h.GetFriendRequests)
	friend.GET("/list", h.GetFriendList)
	friend.DELETE("/:friendID", h.DeleteFriend)
	friend.POST("/block", h.BlockUser)
	friend.POST("/unblock", h.UnblockUser)
}
