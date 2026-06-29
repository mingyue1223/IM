package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── Group service error constants ──

const (
	ErrNotOwnerOrAdmin    = "only owner or admin can perform this action"
	ErrGroupNotFound      = "group not found"
	ErrAlreadyMember      = "user is already a group member"
	ErrGroupFull          = "group is full (max 500 members)"
	ErrCannotRemoveOwner  = "cannot remove the group owner"
	ErrCannotLeaveAsOwner = "owner cannot leave the group; transfer ownership or dissolve first"
	ErrInvalidRole        = "role must be 0 (member) or 1 (admin)"
)

const maxGroupMembers = 500

// GroupService handles group-related business logic.
type GroupService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	logger    *zap.Logger
}

// NewGroupService creates a GroupService with the given repos and logger.
func NewGroupService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, logger *zap.Logger) *GroupService {
	return &GroupService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}
}

// CreateGroup creates a new group, adds the owner as a member with role=2 (owner),
// and updates Redis caches. Returns the new groupID.
func (s *GroupService) CreateGroup(ctx context.Context, ownerID int64, name, notice string) (int64, error) {
	group := &model.Group{
		Name:    name,
		Notice:  notice,
		OwnerID: ownerID,
	}
	groupID, err := s.mysqlRepo.CreateGroup(ctx, group)
	if err != nil {
		return 0, fmt.Errorf("create group in mysql: %w", err)
	}

	// Add owner as group member with role=2 (owner)
	ownerMember := &model.GroupMember{
		GroupID: groupID,
		UserID:  ownerID,
		Role:    2, // owner
	}
	if err := s.mysqlRepo.AddGroupMember(ctx, ownerMember); err != nil {
		return 0, fmt.Errorf("add owner as group member: %w", err)
	}

	// Update Redis caches
	if err := s.redisRepo.AddGroupMemberRedis(ctx, groupID, ownerID); err != nil {
		s.logger.Warn("failed to add owner to redis group cache", zap.Int64("groupID", groupID), zap.Int64("userID", ownerID), zap.Error(err))
	}

	return groupID, nil
}

// UpdateGroup updates group name and notice. Only owner or admin can update.
func (s *GroupService) UpdateGroup(ctx context.Context, userID, groupID int64, name, notice string) error {
	group, err := s.mysqlRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group: %w", err)
	}
	if group == nil {
		return fmt.Errorf(ErrGroupNotFound)
	}

	// Validate userID is owner or admin
	if !s.isOwnerOrAdmin(ctx, groupID, userID) {
		return fmt.Errorf(ErrNotOwnerOrAdmin)
	}

	group.Name = name
	group.Notice = notice
	if err := s.mysqlRepo.UpdateGroup(ctx, group); err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	return nil
}

// GetGroupInfo returns group details by ID.
func (s *GroupService) GetGroupInfo(ctx context.Context, groupID int64) (*model.Group, error) {
	group, err := s.mysqlRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	if group == nil {
		return nil, fmt.Errorf(ErrGroupNotFound)
	}
	return group, nil
}

// AddMember adds a new member to the group. userID must be owner or admin.
// Checks max_members cap (500) and that newMemberID is not already a member.
func (s *GroupService) AddMember(ctx context.Context, groupID, userID, newMemberID int64) error {
	group, err := s.mysqlRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group: %w", err)
	}
	if group == nil {
		return fmt.Errorf(ErrGroupNotFound)
	}

	// Validate userID is owner or admin
	if !s.isOwnerOrAdmin(ctx, groupID, userID) {
		return fmt.Errorf(ErrNotOwnerOrAdmin)
	}

	// Check group capacity
	members, err := s.mysqlRepo.GetGroupMembers(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group members: %w", err)
	}
	if len(members) >= maxGroupMembers {
		return fmt.Errorf(ErrGroupFull)
	}

	// Check if already a member
	for _, m := range members {
		if m.UserID == newMemberID {
			return fmt.Errorf(ErrAlreadyMember)
		}
	}

	// Add member in MySQL
	newMember := &model.GroupMember{
		GroupID: groupID,
		UserID:  newMemberID,
		Role:    0, // regular member
	}
	if err := s.mysqlRepo.AddGroupMember(ctx, newMember); err != nil {
		return fmt.Errorf("add group member: %w", err)
	}

	// Update Redis caches
	if err := s.redisRepo.AddGroupMemberRedis(ctx, groupID, newMemberID); err != nil {
		s.logger.Warn("failed to add member to redis group cache", zap.Int64("groupID", groupID), zap.Int64("userID", newMemberID), zap.Error(err))
	}

	return nil
}

// RemoveMember removes a member from the group.
// userID must be owner/admin, or removeMemberID==userID (self-leave).
// The owner cannot be removed.
func (s *GroupService) RemoveMember(ctx context.Context, groupID, userID, removeMemberID int64) error {
	group, err := s.mysqlRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group: %w", err)
	}
	if group == nil {
		return fmt.Errorf(ErrGroupNotFound)
	}

	// Self-leave is allowed
	if userID != removeMemberID {
		// Not self-leave — must be owner or admin
		if !s.isOwnerOrAdmin(ctx, groupID, userID) {
			return fmt.Errorf(ErrNotOwnerOrAdmin)
		}
	}

	// Cannot remove the owner
	if removeMemberID == group.OwnerID {
		return fmt.Errorf(ErrCannotRemoveOwner)
	}

	// Remove from MySQL
	if err := s.mysqlRepo.RemoveGroupMember(ctx, groupID, removeMemberID); err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}

	// Remove from Redis
	if err := s.redisRepo.RemoveGroupMemberRedis(ctx, groupID, removeMemberID); err != nil {
		s.logger.Warn("failed to remove member from redis group cache", zap.Int64("groupID", groupID), zap.Int64("userID", removeMemberID), zap.Error(err))
	}

	return nil
}

// KickMember removes a member and sends a kick notification via WebSocket.
// userID must be owner/admin. The owner cannot be kicked.
func (s *GroupService) KickMember(ctx context.Context, groupID, userID, kickMemberID int64, cm interface{ Get(int64) (interface{}, bool) }) error {
	// Delegate removal logic to RemoveMember
	if err := s.RemoveMember(ctx, groupID, userID, kickMemberID); err != nil {
		return err
	}

	// Send kick notification via WebSocket if the kicked user has an active connection
	if cm != nil {
		client, ok := cm.Get(kickMemberID)
		if ok {
			// Type assertion to access SendCh — the interface is flexible
			// The actual ConnectionManager returns *ClientConnection
			if sendCh, hasCh := tryGetSendCh(client); hasCh {
				kickMsg := []byte(`{"type":"group_kick","group_id":` + fmt.Sprintf("%d", groupID) + `}`)
				select {
				case sendCh <- kickMsg:
				default:
					s.logger.Warn("kick notification dropped: send buffer full", zap.Int64("userID", kickMemberID))
				}
			}
		}
	}

	return nil
}

// tryGetSendCh attempts to extract a []byte send channel from a client connection object.
// Returns the channel and true if extraction succeeds, nil and false otherwise.
func tryGetSendCh(client interface{}) (chan []byte, bool) {
	// We expect a struct with SendCh chan []byte
	type sendChHolder interface {
		GetSendCh() chan []byte
	}
	if h, ok := client.(sendChHolder); ok {
		return h.GetSendCh(), true
	}
	return nil, false
}

// GetMembers returns the list of group members.
func (s *GroupService) GetMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	members, err := s.mysqlRepo.GetGroupMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group members: %w", err)
	}
	return members, nil
}

// UpdateMemberRole changes a member's role. userID must be the group owner.
// targetUserID must not be the owner. newRole must be 0 (member) or 1 (admin).
func (s *GroupService) UpdateMemberRole(ctx context.Context, groupID, userID, targetUserID, newRole int64) error {
	group, err := s.mysqlRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group: %w", err)
	}
	if group == nil {
		return fmt.Errorf(ErrGroupNotFound)
	}

	// Only owner can change roles
	if userID != group.OwnerID {
		return fmt.Errorf(ErrNotOwnerOrAdmin)
	}

	// Cannot change owner's role
	if targetUserID == group.OwnerID {
		return fmt.Errorf(ErrCannotRemoveOwner)
	}

	// Validate role value
	if newRole != 0 && newRole != 1 {
		return fmt.Errorf(ErrInvalidRole)
	}

	if err := s.mysqlRepo.UpdateGroupMemberRole(ctx, int(groupID), int(targetUserID), int(newRole)); err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	return nil
}

// LeaveGroup allows a user to leave a group. The owner cannot leave (must transfer or dissolve).
func (s *GroupService) LeaveGroup(ctx context.Context, groupID, userID int64) error {
	group, err := s.mysqlRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group: %w", err)
	}
	if group == nil {
		return fmt.Errorf(ErrGroupNotFound)
	}

	// Owner cannot leave
	if userID == group.OwnerID {
		return fmt.Errorf(ErrCannotLeaveAsOwner)
	}

	// Remove from MySQL
	if err := s.mysqlRepo.RemoveGroupMember(ctx, groupID, userID); err != nil {
		return fmt.Errorf("leave group: %w", err)
	}

	// Remove from Redis
	if err := s.redisRepo.RemoveGroupMemberRedis(ctx, groupID, userID); err != nil {
		s.logger.Warn("failed to remove member from redis group cache on leave", zap.Int64("groupID", groupID), zap.Int64("userID", userID), zap.Error(err))
	}

	return nil
}

// isOwnerOrAdmin checks whether the given userID has owner (role=2) or admin (role=1) status
// in the group. Returns true if so, false otherwise.
func (s *GroupService) isOwnerOrAdmin(ctx context.Context, groupID, userID int64) bool {
	members, err := s.mysqlRepo.GetGroupMembers(ctx, groupID)
	if err != nil {
		s.logger.Error("failed to get group members for permission check", zap.Error(err))
		return false
	}
	for _, m := range members {
		if m.UserID == userID && (m.Role == 1 || m.Role == 2) {
			return true
		}
	}
	return false
}
