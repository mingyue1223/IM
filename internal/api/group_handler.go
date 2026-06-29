package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/goim/goim/internal/service"
)

// GroupHandler provides Gin HTTP handlers for group endpoints.
type GroupHandler struct {
	groupSvc *service.GroupService
}

// NewGroupHandler creates a GroupHandler wrapping the given GroupService.
func NewGroupHandler(groupSvc *service.GroupService) *GroupHandler {
	return &GroupHandler{groupSvc: groupSvc}
}

// ── Request / response DTOs ──

type createGroupRequest struct {
	Name   string `json:"name" binding:"required"`
	Notice string `json:"notice"`
}

type createGroupResponse struct {
	GroupID int64 `json:"group_id"`
}

type updateGroupRequest struct {
	Name   string `json:"name" binding:"required"`
	Notice string `json:"notice"`
}

type addMemberRequest struct {
	MemberID int64 `json:"member_id" binding:"required"`
}

type updateMemberRoleRequest struct {
	Role int64 `json:"role" binding:"required"`
}

// ── Handlers ──

// CreateGroup handles POST /group.
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	groupID, err := h.groupSvc.CreateGroup(c.Request.Context(), userID, req.Name, req.Notice)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusCreated, createGroupResponse{GroupID: groupID})
}

// UpdateGroup handles PUT /group/:groupID.
func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	var req updateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err = h.groupSvc.UpdateGroup(c.Request.Context(), userID, groupID, req.Name, req.Notice)
	if err != nil {
		switch err.Error() {
		case service.ErrNotOwnerOrAdmin:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "group updated"})
}

// GetGroupInfo handles GET /group/:groupID.
func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	group, err := h.groupSvc.GetGroupInfo(c.Request.Context(), groupID)
	if err != nil {
		switch err.Error() {
		case service.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, group)
}

// AddMember handles POST /group/:groupID/member.
func (h *GroupHandler) AddMember(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_id is required"})
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err = h.groupSvc.AddMember(c.Request.Context(), groupID, userID, req.MemberID)
	if err != nil {
		switch err.Error() {
		case service.ErrNotOwnerOrAdmin:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case service.ErrAlreadyMember:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case service.ErrGroupFull:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member added"})
}

// RemoveMember handles DELETE /group/:groupID/member/:memberID.
// If the requesting user is the member being removed, it's a self-leave.
// Otherwise, it's a kick (owner/admin removes another member).
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member_id"})
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err = h.groupSvc.RemoveMember(c.Request.Context(), groupID, userID, memberID)
	if err != nil {
		switch err.Error() {
		case service.ErrNotOwnerOrAdmin:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case service.ErrCannotRemoveOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

// GetMembers handles GET /group/:groupID/members.
func (h *GroupHandler) GetMembers(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	members, err := h.groupSvc.GetMembers(c.Request.Context(), groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// UpdateMemberRole handles PUT /group/:groupID/member/:memberID/role.
func (h *GroupHandler) UpdateMemberRole(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	memberID, err := strconv.ParseInt(c.Param("memberID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member_id"})
		return
	}

	var req updateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role is required"})
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err = h.groupSvc.UpdateMemberRole(c.Request.Context(), groupID, userID, memberID, req.Role)
	if err != nil {
		switch err.Error() {
		case service.ErrNotOwnerOrAdmin:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case service.ErrCannotRemoveOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrInvalidRole:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member role updated"})
}

// LeaveGroup handles POST /group/:groupID/leave.
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group_id"})
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err = h.groupSvc.LeaveGroup(c.Request.Context(), groupID, userID)
	if err != nil {
		switch err.Error() {
		case service.ErrCannotLeaveAsOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case service.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "left group"})
}

// RegisterRoutes registers all group HTTP routes on the given Gin router group.
func (h *GroupHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/group")
	g.POST("", h.CreateGroup)
	g.PUT("/:groupID", h.UpdateGroup)
	g.GET("/:groupID", h.GetGroupInfo)
	g.POST("/:groupID/member", h.AddMember)
	g.DELETE("/:groupID/member/:memberID", h.RemoveMember)
	g.GET("/:groupID/members", h.GetMembers)
	g.PUT("/:groupID/member/:memberID/role", h.UpdateMemberRole)
	g.POST("/:groupID/leave", h.LeaveGroup)
}
