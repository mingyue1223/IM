package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/redis"
	"github.com/goim/goim/internal/repository"
)

// ──────────────────────────────────────────────────────
// Mock MySQLRepo for group tests
// ──────────────────────────────────────────────────────

type mockGroupMySQLRepo struct {
	mu sync.Mutex

	// Stored groups keyed by ID
	groupsByID map[int64]*model.Group
	// Stored group members keyed by composite "groupID:userID"
	membersByKey map[string]*model.GroupMember
	// Member lists keyed by groupID
	membersByGroup map[int64][]model.GroupMember
	// Next auto-increment group ID
	nextGroupID int64
	// Next auto-increment member ID
	nextMemberID int64

	// Error overrides
	createGroupErr    error
	getGroupByIDErr   error
	addGroupMemberErr error
}

func newMockGroupMySQLRepo() *mockGroupMySQLRepo {
	return &mockGroupMySQLRepo{
		groupsByID:      make(map[int64]*model.Group),
		membersByKey:    make(map[string]*model.GroupMember),
		membersByGroup:  make(map[int64][]model.GroupMember),
		nextGroupID:     1,
		nextMemberID:    1,
	}
}

func (m *mockGroupMySQLRepo) CreateGroup(_ context.Context, group *model.Group) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createGroupErr != nil {
		return 0, m.createGroupErr
	}
	group.ID = m.nextGroupID
	m.nextGroupID++
	group.MaxMembers = 500
	group.CreatedAt = time.Now()
	group.UpdatedAt = time.Now()
	m.groupsByID[group.ID] = group
	return group.ID, nil
}

func (m *mockGroupMySQLRepo) UpdateGroup(_ context.Context, group *model.Group) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.groupsByID[group.ID]
	if !ok {
		return fmt.Errorf("group not found")
	}
	stored.Name = group.Name
	stored.Notice = group.Notice
	stored.UpdatedAt = time.Now()
	return nil
}

func (m *mockGroupMySQLRepo) GetGroupByID(_ context.Context, groupID int64) (*model.Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getGroupByIDErr != nil {
		return nil, m.getGroupByIDErr
	}
	g, ok := m.groupsByID[groupID]
	if !ok {
		return nil, nil
	}
	return g, nil
}

func (m *mockGroupMySQLRepo) AddGroupMember(_ context.Context, member *model.GroupMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.addGroupMemberErr != nil {
		return m.addGroupMemberErr
	}
	key := fmt.Sprintf("%d:%d", member.GroupID, member.UserID)
	if _, exists := m.membersByKey[key]; exists {
		return fmt.Errorf("duplicate member")
	}
	member.ID = m.nextMemberID
	m.nextMemberID++
	member.JoinedAt = time.Now()
	m.membersByKey[key] = member
	m.membersByGroup[member.GroupID] = append(m.membersByGroup[member.GroupID], *member)
	return nil
}

func (m *mockGroupMySQLRepo) RemoveGroupMember(_ context.Context, groupID, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d:%d", groupID, userID)
	if _, exists := m.membersByKey[key]; !exists {
		return nil // no-op if not found
	}
	delete(m.membersByKey, key)
	members := m.membersByGroup[groupID]
	filtered := make([]model.GroupMember, 0, len(members))
	for _, gm := range members {
		if gm.UserID != userID {
			filtered = append(filtered, gm)
		}
	}
	m.membersByGroup[groupID] = filtered
	return nil
}

func (m *mockGroupMySQLRepo) GetGroupMembers(_ context.Context, groupID int64) ([]model.GroupMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.membersByGroup[groupID], nil
}

func (m *mockGroupMySQLRepo) UpdateGroupMemberRole(_ context.Context, groupID, userID, role int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d:%d", groupID, userID)
	member, ok := m.membersByKey[key]
	if !ok {
		return fmt.Errorf("member not found")
	}
	member.Role = role
	// Update in the slice too
	members := m.membersByGroup[int64(groupID)]
	for i := range members {
		if members[i].UserID == int64(userID) {
			members[i].Role = role
		}
	}
	return nil
}

// ── Stub out all other MySQLRepo methods ──

func (m *mockGroupMySQLRepo) GetUserByID(_ context.Context, _ int64) (*model.User, error)    { return nil, nil }
func (m *mockGroupMySQLRepo) GetUserByUsername(_ context.Context, _ string) (*model.User, error) { return nil, nil }
func (m *mockGroupMySQLRepo) CreateUser(_ context.Context, _ *model.User) error               { return nil }
func (m *mockGroupMySQLRepo) UpdateUser(_ context.Context, _ *model.User) error               { return nil }

func (m *mockGroupMySQLRepo) InsertPrivateMessage(_ context.Context, _ *model.PrivateMessage) error { return nil }
func (m *mockGroupMySQLRepo) InsertGroupMessage(_ context.Context, _ *model.GroupMessage) error    { return nil }
func (m *mockGroupMySQLRepo) InsertMsgRevoked(_ context.Context, _ *model.MsgRevoked) error        { return nil }

func (m *mockGroupMySQLRepo) CreateFriendRequest(_ context.Context, _ *model.FriendRequest) error { return nil }
func (m *mockGroupMySQLRepo) UpdateFriendRequest(_ context.Context, _ *model.FriendRequest) error { return nil }
func (m *mockGroupMySQLRepo) GetFriendRequestByID(_ context.Context, _ int64) (*model.FriendRequest, error) {
	return nil, nil
}
func (m *mockGroupMySQLRepo) GetFriendRequestsByUser(_ context.Context, _ int64) ([]model.FriendRequest, error) {
	return nil, nil
}
func (m *mockGroupMySQLRepo) CreateFriendship(_ context.Context, _ *model.Friendship) error { return nil }
func (m *mockGroupMySQLRepo) DeleteFriendship(_ context.Context, _ int64, _ int64) error    { return nil }
func (m *mockGroupMySQLRepo) GetFriendList(_ context.Context, _ int64) ([]model.Friendship, error) {
	return nil, nil
}
func (m *mockGroupMySQLRepo) IsFriend(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil }
func (m *mockGroupMySQLRepo) CreateBlacklist(_ context.Context, _ *model.Blacklist) error { return nil }
func (m *mockGroupMySQLRepo) DeleteBlacklist(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockGroupMySQLRepo) IsBlocked(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil }

func (m *mockGroupMySQLRepo) CreateMoment(_ context.Context, _ *model.Moment) error          { return nil }
func (m *mockGroupMySQLRepo) GetMomentByID(_ context.Context, _ int64) (*model.Moment, error) { return nil, nil }
func (m *mockGroupMySQLRepo) GetMomentsByUser(_ context.Context, _ int64, _ int, _ int) ([]model.Moment, error) {
	return nil, nil
}
func (m *mockGroupMySQLRepo) CreateMomentLike(_ context.Context, _ *model.MomentLike) error    { return nil }
func (m *mockGroupMySQLRepo) DeleteMomentLike(_ context.Context, _ int64, _ int64) error       { return nil }
func (m *mockGroupMySQLRepo) CreateMomentComment(_ context.Context, _ *model.MomentComment) error { return nil }
func (m *mockGroupMySQLRepo) DeleteMomentComment(_ context.Context, _ int64) error              { return nil }

func (m *mockGroupMySQLRepo) CreateAISummary(_ context.Context, _ *model.AISummary) error        { return nil }
func (m *mockGroupMySQLRepo) CreateAIProfileItem(_ context.Context, _ *model.AIProfileItem) error { return nil }
func (m *mockGroupMySQLRepo) GetAIProfileByUser(_ context.Context, _ int64) ([]model.AIProfileItem, error) {
	return nil, nil
}
func (m *mockGroupMySQLRepo) GetMomentCommentByID(_ context.Context, _ int64) (*model.MomentComment, error) {
	return nil, nil
}

// ──────────────────────────────────────────────────────
// Mock RedisRepo for group tests
// ──────────────────────────────────────────────────────

type mockGroupRedisRepo struct {
	mu sync.Mutex

	// group_members:{groupID} -> set of userID strings
	groupMembersSets map[int64]map[string]bool
	// user_groups:{userID} -> set of groupID strings
	userGroupsSets map[int64]map[string]bool

	// Error overrides
	addGroupMemberErr    error
	removeGroupMemberErr error
}

func newMockGroupRedisRepo() *mockGroupRedisRepo {
	return &mockGroupRedisRepo{
		groupMembersSets: make(map[int64]map[string]bool),
		userGroupsSets:   make(map[int64]map[string]bool),
	}
}

func (r *mockGroupRedisRepo) GetGroupMemberships(_ context.Context, userID int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.userGroupsSets[userID]
	if !ok {
		return []int64{}, nil
	}
	result := make([]int64, 0, len(set))
	for k := range set {
		id, _ := fmt.Sscanf(k, "%d", new(int64))
		_ = id
		result = append(result, 0) // simplified
	}
	return result, nil
}

func (r *mockGroupRedisRepo) GetGroupMembers(_ context.Context, groupID int64) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.groupMembersSets[groupID]
	if !ok {
		return []int64{}, nil
	}
	result := make([]int64, 0, len(set))
	for k := range set {
		var id int64
		fmt.Sscanf(k, "%d", &id)
		result = append(result, id)
	}
	return result, nil
}

func (r *mockGroupRedisRepo) AddGroupMemberRedis(_ context.Context, groupID, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.addGroupMemberErr != nil {
		return r.addGroupMemberErr
	}
	userIDStr := fmt.Sprintf("%d", userID)
	groupIDStr := fmt.Sprintf("%d", groupID)

	if r.groupMembersSets[groupID] == nil {
		r.groupMembersSets[groupID] = make(map[string]bool)
	}
	r.groupMembersSets[groupID][userIDStr] = true

	if r.userGroupsSets[userID] == nil {
		r.userGroupsSets[userID] = make(map[string]bool)
	}
	r.userGroupsSets[userID][groupIDStr] = true
	return nil
}

func (r *mockGroupRedisRepo) RemoveGroupMemberRedis(_ context.Context, groupID, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.removeGroupMemberErr != nil {
		return r.removeGroupMemberErr
	}
	userIDStr := fmt.Sprintf("%d", userID)
	groupIDStr := fmt.Sprintf("%d", groupID)

	delete(r.groupMembersSets[groupID], userIDStr)
	delete(r.userGroupsSets[userID], groupIDStr)
	return nil
}

// ── Stub out all other RedisRepo methods ──

func (r *mockGroupRedisRepo) WriteInbox(_ context.Context, _ int64, _ *model.InboxMessage) error { return nil }
func (r *mockGroupRedisRepo) WriteOutbox(_ context.Context, _ int64, _ *model.InboxMessage) error { return nil }
func (r *mockGroupRedisRepo) ReadInbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	return nil, nil
}
func (r *mockGroupRedisRepo) ReadOutbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	return nil, nil
}
func (r *mockGroupRedisRepo) UpdateConvList(_ context.Context, _ int64, _ string, _ string, _ int64) error {
	return nil
}
func (r *mockGroupRedisRepo) GetConvList(_ context.Context, _ int64) ([]model.ConvSummary, error) {
	return nil, nil
}
func (r *mockGroupRedisRepo) IncrementUnread(_ context.Context, _ int64, _ string) error { return nil }
func (r *mockGroupRedisRepo) ClearUnread(_ context.Context, _ int64, _ string) error      { return nil }
func (r *mockGroupRedisRepo) GetUnreadMap(_ context.Context, _ int64) (map[string]int64, error) {
	return nil, nil
}
func (r *mockGroupRedisRepo) SetGroupReadPos(_ context.Context, _ int64, _ string, _ int64) error { return nil }
func (r *mockGroupRedisRepo) GetGroupReadPos(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, nil
}
func (r *mockGroupRedisRepo) CheckDuplicate(_ context.Context, _ int64, _ string) (bool, error) { return false, nil }
func (r *mockGroupRedisRepo) TrimInbox(_ context.Context, _ int64, _ int) error         { return nil }
func (r *mockGroupRedisRepo) TrimOutbox(_ context.Context, _ int64, _ int) error        { return nil }
func (r *mockGroupRedisRepo) TrimInboxByTime(_ context.Context, _ int64, _ int64) error  { return nil }
func (r *mockGroupRedisRepo) TrimOutboxByTime(_ context.Context, _ int64, _ int64) error { return nil }
func (r *mockGroupRedisRepo) TrimConvListByTime(_ context.Context, _ int64, _ int64) error { return nil }
func (r *mockGroupRedisRepo) TrimTimelineByTime(_ context.Context, _ int64, _ int64) error { return nil }

func (r *mockGroupRedisRepo) ExecPrivateMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.PrivateMsgCheckResult, error) {
	return nil, nil
}
func (r *mockGroupRedisRepo) ExecGroupMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.GroupMsgCheckResult, error) {
	return nil, nil
}
func (r *mockGroupRedisRepo) ExecInboxMarkRead(_ context.Context, _ int64, _ string) (int64, error) { return 0, nil }
func (r *mockGroupRedisRepo) ExecRevokeMsg(_ context.Context, _ int64, _ string, _ int64, _ string, _ int64) (bool, error) {
	return false, nil
}
func (r *mockGroupRedisRepo) PublishMomentFeed(_ context.Context, _ int64, _ int64, _ int64) error { return nil }
func (r *mockGroupRedisRepo) GetMomentFeed(_ context.Context, _ int64, _ int64, _ int) ([]int64, error) {
	return nil, nil
}

// ──────────────────────────────────────────────────────
// Helper: verify mockMySQLRepo satisfies MySQLRepo interface
// ──────────────────────────────────────────────────────

var _ repository.MySQLRepo = (*mockGroupMySQLRepo)(nil)
var _ repository.RedisRepo = (*mockGroupRedisRepo)(nil)

// ──────────────────────────────────────────────────────
// Helper: new test GroupService
// ──────────────────────────────────────────────────────

func newTestGroupService(mysqlRepo *mockGroupMySQLRepo, redisRepo *mockGroupRedisRepo) *GroupService {
	logger := zap.NewNop()
	return NewGroupService(mysqlRepo, redisRepo, logger)
}

// ──────────────────────────────────────────────────────
// CreateGroup tests
// ──────────────────────────────────────────────────────

func TestGroup_CreateGroup_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, err := svc.CreateGroup(context.Background(), 1, "TestGroup", "Welcome!")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), groupID)

	// Verify group stored
	mysqlRepo.mu.Lock()
	group := mysqlRepo.groupsByID[1]
	mysqlRepo.mu.Unlock()
	assert.NotNil(t, group)
	assert.Equal(t, "TestGroup", group.Name)
	assert.Equal(t, int64(1), group.OwnerID)
	assert.Equal(t, 500, group.MaxMembers)

	// Verify owner added as member (role=2)
	mysqlRepo.mu.Lock()
	members := mysqlRepo.membersByGroup[1]
	mysqlRepo.mu.Unlock()
	assert.Len(t, members, 1)
	assert.Equal(t, int64(1), members[0].UserID)
	assert.Equal(t, 2, members[0].Role)

	// Verify Redis cache updated
	redisRepo.mu.Lock()
	groupMembers := redisRepo.groupMembersSets[1]
	userGroups := redisRepo.userGroupsSets[1]
	redisRepo.mu.Unlock()
	assert.True(t, groupMembers["1"])
	assert.True(t, userGroups["1"])
}

// ──────────────────────────────────────────────────────
// UpdateGroup tests
// ──────────────────────────────────────────────────────

func TestGroup_UpdateGroup_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	// Create group first
	groupID, _ := svc.CreateGroup(context.Background(), 1, "OldName", "OldNotice")

	err := svc.UpdateGroup(context.Background(), 1, groupID, "NewName", "NewNotice")
	assert.NoError(t, err)

	mysqlRepo.mu.Lock()
	group := mysqlRepo.groupsByID[groupID]
	mysqlRepo.mu.Unlock()
	assert.Equal(t, "NewName", group.Name)
	assert.Equal(t, "NewNotice", group.Notice)
}

func TestGroup_UpdateGroup_NotOwner(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	// Create group owned by user 1
	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// User 2 (regular member) tries to update
	err := svc.UpdateGroup(context.Background(), 2, groupID, "HackedName", "")
	assert.Error(t, err)
	assert.Equal(t, ErrNotOwnerOrAdmin, err.Error())
}

func TestGroup_UpdateGroup_GroupNotFound(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	err := svc.UpdateGroup(context.Background(), 1, 999, "Name", "")
	assert.Error(t, err)
	assert.Equal(t, ErrGroupNotFound, err.Error())
}

func TestGroup_UpdateGroup_AdminCanUpdate(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	// Create group owned by user 1
	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// Add user 2 as admin
	mysqlRepo.mu.Lock()
	member := &model.GroupMember{GroupID: groupID, UserID: 2, Role: 1}
	key := fmt.Sprintf("%d:%d", groupID, 2)
	mysqlRepo.membersByKey[key] = member
	mysqlRepo.membersByGroup[groupID] = append(mysqlRepo.membersByGroup[groupID], *member)
	mysqlRepo.mu.Unlock()

	// Admin (user 2) can update
	err := svc.UpdateGroup(context.Background(), 2, groupID, "AdminUpdate", "AdminNotice")
	assert.NoError(t, err)
}

// ──────────────────────────────────────────────────────
// AddMember tests
// ──────────────────────────────────────────────────────

func TestGroup_AddMember_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	err := svc.AddMember(context.Background(), groupID, 1, 2)
	assert.NoError(t, err)

	mysqlRepo.mu.Lock()
	members := mysqlRepo.membersByGroup[groupID]
	mysqlRepo.mu.Unlock()
	assert.Len(t, members, 2) // owner + new member
}

func TestGroup_AddMember_AlreadyMember(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// Owner tries to add themselves again
	err := svc.AddMember(context.Background(), groupID, 1, 1)
	assert.Error(t, err)
	assert.Equal(t, ErrAlreadyMember, err.Error())
}

func TestGroup_AddMember_GroupFull(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// Fill group to 500 members
	mysqlRepo.mu.Lock()
	for i := int64(2); i <= 500; i++ {
		member := model.GroupMember{GroupID: groupID, UserID: i, Role: 0}
		key := fmt.Sprintf("%d:%d", groupID, i)
		member.ID = i
		member.JoinedAt = time.Now()
		mysqlRepo.membersByKey[key] = &member
		mysqlRepo.membersByGroup[groupID] = append(mysqlRepo.membersByGroup[groupID], member)
	}
	mysqlRepo.mu.Unlock()

	// Try to add member 501
	err := svc.AddMember(context.Background(), groupID, 1, 501)
	assert.Error(t, err)
	assert.Equal(t, ErrGroupFull, err.Error())
}

func TestGroup_AddMember_NotOwnerOrAdmin(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// Regular member tries to add someone
	err := svc.AddMember(context.Background(), groupID, 3, 4)
	assert.Error(t, err)
	assert.Equal(t, ErrNotOwnerOrAdmin, err.Error())
}

// ──────────────────────────────────────────────────────
// RemoveMember tests
// ──────────────────────────────────────────────────────

func TestGroup_RemoveMember_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// Add a member first
	svc.AddMember(context.Background(), groupID, 1, 2)

	err := svc.RemoveMember(context.Background(), groupID, 1, 2)
	assert.NoError(t, err)

	mysqlRepo.mu.Lock()
	members := mysqlRepo.membersByGroup[groupID]
	mysqlRepo.mu.Unlock()
	assert.Len(t, members, 1) // only owner left
}

func TestGroup_RemoveMember_CannotRemoveOwner(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	// Try to remove the owner
	err := svc.RemoveMember(context.Background(), groupID, 1, 1)
	assert.Error(t, err)
	assert.Equal(t, ErrCannotRemoveOwner, err.Error())
}

func TestGroup_RemoveMember_SelfLeave(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)

	// User 2 removes themselves (self-leave)
	err := svc.RemoveMember(context.Background(), groupID, 2, 2)
	assert.NoError(t, err)

	mysqlRepo.mu.Lock()
	members := mysqlRepo.membersByGroup[groupID]
	mysqlRepo.mu.Unlock()
	assert.Len(t, members, 1)
}

// ──────────────────────────────────────────────────────
// LeaveGroup tests
// ──────────────────────────────────────────────────────

func TestGroup_LeaveGroup_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)

	err := svc.LeaveGroup(context.Background(), groupID, 2)
	assert.NoError(t, err)

	mysqlRepo.mu.Lock()
	members := mysqlRepo.membersByGroup[groupID]
	mysqlRepo.mu.Unlock()
	assert.Len(t, members, 1)
}

func TestGroup_LeaveGroup_OwnerCannotLeave(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	err := svc.LeaveGroup(context.Background(), groupID, 1)
	assert.Error(t, err)
	assert.Equal(t, ErrCannotLeaveAsOwner, err.Error())
}

// ──────────────────────────────────────────────────────
// UpdateMemberRole tests
// ──────────────────────────────────────────────────────

func TestGroup_UpdateMemberRole_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)

	err := svc.UpdateMemberRole(context.Background(), groupID, 1, 2, 1) // promote to admin
	assert.NoError(t, err)

	mysqlRepo.mu.Lock()
	key := fmt.Sprintf("%d:%d", groupID, 2)
	member := mysqlRepo.membersByKey[key]
	mysqlRepo.mu.Unlock()
	assert.Equal(t, 1, member.Role)
}

func TestGroup_UpdateMemberRole_NotOwner(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)

	// Admin (user 2) tries to change roles — only owner can
	mysqlRepo.mu.Lock()
	key := fmt.Sprintf("%d:%d", groupID, 2)
	mysqlRepo.membersByKey[key].Role = 1
	for i := range mysqlRepo.membersByGroup[groupID] {
		if mysqlRepo.membersByGroup[groupID][i].UserID == 2 {
			mysqlRepo.membersByGroup[groupID][i].Role = 1
		}
	}
	mysqlRepo.mu.Unlock()

	err := svc.UpdateMemberRole(context.Background(), groupID, 2, 3, 1)
	assert.Error(t, err)
	assert.Equal(t, ErrNotOwnerOrAdmin, err.Error())
}

func TestGroup_UpdateMemberRole_CannotChangeOwnerRole(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")

	err := svc.UpdateMemberRole(context.Background(), groupID, 1, 1, 0)
	assert.Error(t, err)
	assert.Equal(t, ErrCannotRemoveOwner, err.Error())
}

func TestGroup_UpdateMemberRole_InvalidRole(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)

	err := svc.UpdateMemberRole(context.Background(), groupID, 1, 2, 2) // role=2 (owner) not allowed
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidRole, err.Error())
}

func TestGroup_UpdateMemberRole_InvalidRoleNegative(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)

	err := svc.UpdateMemberRole(context.Background(), groupID, 1, 2, -1)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidRole, err.Error())
}

// ──────────────────────────────────────────────────────
// GetGroupInfo tests
// ──────────────────────────────────────────────────────

func TestGroup_GetGroupInfo_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "MyGroup", "MyNotice")

	group, err := svc.GetGroupInfo(context.Background(), groupID)
	assert.NoError(t, err)
	assert.Equal(t, "MyGroup", group.Name)
	assert.Equal(t, int64(1), group.OwnerID)
}

func TestGroup_GetGroupInfo_NotFound(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	group, err := svc.GetGroupInfo(context.Background(), 999)
	assert.Error(t, err)
	assert.Equal(t, ErrGroupNotFound, err.Error())
	assert.Nil(t, group)
}

// ──────────────────────────────────────────────────────
// GetMembers tests
// ──────────────────────────────────────────────────────

func TestGroup_GetMembers_Success(t *testing.T) {
	mysqlRepo := newMockGroupMySQLRepo()
	redisRepo := newMockGroupRedisRepo()
	svc := newTestGroupService(mysqlRepo, redisRepo)

	groupID, _ := svc.CreateGroup(context.Background(), 1, "Group", "")
	svc.AddMember(context.Background(), groupID, 1, 2)
	svc.AddMember(context.Background(), groupID, 1, 3)

	members, err := svc.GetMembers(context.Background(), groupID)
	assert.NoError(t, err)
	assert.Len(t, members, 3) // owner + 2 members
}
