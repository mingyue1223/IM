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
	redislua "github.com/goim/goim/internal/redis"
)

// ──────────────────────────────────────────────────────
// Mock MySQLRepo for friend tests
// ──────────────────────────────────────────────────────

type mockFriendRepo struct {
	mu sync.Mutex

	// Stored friend requests keyed by ID
	friendRequests map[int64]*model.FriendRequest
	// Stored friendships keyed by "userID:friendID"
	friendships map[string]*model.Friendship
	// Stored blacklist entries keyed by "userID:blockedID"
	blacklist map[string]*model.Blacklist
	// Stored users keyed by ID (for GetFriendList enrichment)
	users map[int64]*model.User
	// Next auto-increment IDs
	nextRequestID int64
	nextFriendID  int64
	nextBlackID   int64

	// Error overrides
	isFriendErr    error
	isBlockedErr   error
	getRequestsErr error
	createReqErr   error
	getReqByIDErr  error
	updateReqErr   error
	createFsErr    error
	deleteFsErr    error
	getFriendListErr error
	createBlackErr error
	deleteBlackErr error
	getUserByIDErr error
}

func newMockFriendRepo() *mockFriendRepo {
	return &mockFriendRepo{
		friendRequests: make(map[int64]*model.FriendRequest),
		friendships:    make(map[string]*model.Friendship),
		blacklist:      make(map[string]*model.Blacklist),
		users:          make(map[int64]*model.User),
		nextRequestID:  1,
		nextFriendID:   1,
		nextBlackID:    1,
	}
}

func friendKey(userID, friendID int64) string {
	return fmt.Sprintf("%d:%d", userID, friendID)
}

func blackKey(userID, blockedID int64) string {
	return fmt.Sprintf("%d:%d", userID, blockedID)
}

// ── Friend-specific methods ──

func (m *mockFriendRepo) CreateFriendRequest(_ context.Context, req *model.FriendRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createReqErr != nil {
		return m.createReqErr
	}
	req.ID = m.nextRequestID
	m.nextRequestID++
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()
	m.friendRequests[req.ID] = req
	return nil
}

func (m *mockFriendRepo) UpdateFriendRequest(_ context.Context, req *model.FriendRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateReqErr != nil {
		return m.updateReqErr
	}
	if stored, ok := m.friendRequests[req.ID]; ok {
		stored.Status = req.Status
		stored.UpdatedAt = time.Now()
	}
	return nil
}

func (m *mockFriendRepo) GetFriendRequestByID(_ context.Context, id int64) (*model.FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getReqByIDErr != nil {
		return nil, m.getReqByIDErr
	}
	r, ok := m.friendRequests[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockFriendRepo) GetFriendRequestsByUser(_ context.Context, userID int64) ([]model.FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getRequestsErr != nil {
		return nil, m.getRequestsErr
	}
	var results []model.FriendRequest
	for _, r := range m.friendRequests {
		if r.FromUserID == userID || r.ToUserID == userID {
			results = append(results, *r)
		}
	}
	return results, nil
}

func (m *mockFriendRepo) CreateFriendship(_ context.Context, fs *model.Friendship) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createFsErr != nil {
		return m.createFsErr
	}
	fs.ID = m.nextFriendID
	m.nextFriendID++
	fs.CreatedAt = time.Now()
	m.friendships[friendKey(fs.UserID, fs.FriendID)] = fs
	// Bidirectional: also add friend→user
	m.friendships[friendKey(fs.FriendID, fs.UserID)] = &model.Friendship{
		ID:        fs.ID + 1,
		UserID:    fs.FriendID,
		FriendID:  fs.UserID,
		CreatedAt: fs.CreatedAt,
	}
	m.nextFriendID++ // account for the reverse row
	return nil
}

func (m *mockFriendRepo) DeleteFriendship(_ context.Context, userID, friendID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteFsErr != nil {
		return m.deleteFsErr
	}
	delete(m.friendships, friendKey(userID, friendID))
	delete(m.friendships, friendKey(friendID, userID))
	return nil
}

func (m *mockFriendRepo) GetFriendList(_ context.Context, userID int64) ([]model.Friendship, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getFriendListErr != nil {
		return nil, m.getFriendListErr
	}
	var results []model.Friendship
	for _, fs := range m.friendships {
		if fs.UserID == userID {
			results = append(results, *fs)
		}
	}
	return results, nil
}

func (m *mockFriendRepo) IsFriend(_ context.Context, userID, friendID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isFriendErr != nil {
		return false, m.isFriendErr
	}
	_, ok := m.friendships[friendKey(userID, friendID)]
	return ok, nil
}

func (m *mockFriendRepo) CreateBlacklist(_ context.Context, bl *model.Blacklist) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createBlackErr != nil {
		return m.createBlackErr
	}
	bl.ID = m.nextBlackID
	m.nextBlackID++
	bl.CreatedAt = time.Now()
	m.blacklist[blackKey(bl.UserID, bl.BlockedID)] = bl
	return nil
}

func (m *mockFriendRepo) DeleteBlacklist(_ context.Context, userID, blockedID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteBlackErr != nil {
		return m.deleteBlackErr
	}
	delete(m.blacklist, blackKey(userID, blockedID))
	return nil
}

func (m *mockFriendRepo) IsBlocked(_ context.Context, userID, blockedID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isBlockedErr != nil {
		return false, m.isBlockedErr
	}
	_, ok := m.blacklist[blackKey(userID, blockedID)]
	return ok, nil
}

// ── User methods (for GetFriendList enrichment) ──

func (m *mockFriendRepo) GetUserByID(_ context.Context, userID int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getUserByIDErr != nil {
		return nil, m.getUserByIDErr
	}
	u, ok := m.users[userID]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockFriendRepo) GetUserByUsername(_ context.Context, _ string) (*model.User, error) { return nil, nil }
func (m *mockFriendRepo) CreateUser(_ context.Context, _ *model.User) error                  { return nil }
func (m *mockFriendRepo) UpdateUser(_ context.Context, _ *model.User) error                  { return nil }

// ── Stub out all other MySQLRepo methods ──

func (m *mockFriendRepo) InsertPrivateMessage(_ context.Context, _ *model.PrivateMessage) error { return nil }
func (m *mockFriendRepo) InsertGroupMessage(_ context.Context, _ *model.GroupMessage) error       { return nil }
func (m *mockFriendRepo) InsertMsgRevoked(_ context.Context, _ *model.MsgRevoked) error           { return nil }

func (m *mockFriendRepo) CreateGroup(_ context.Context, _ *model.Group) (int64, error) { return 0, nil }
func (m *mockFriendRepo) UpdateGroup(_ context.Context, _ *model.Group) error           { return nil }
func (m *mockFriendRepo) GetGroupByID(_ context.Context, _ int64) (*model.Group, error) { return nil, nil }
func (m *mockFriendRepo) AddGroupMember(_ context.Context, _ *model.GroupMember) error  { return nil }
func (m *mockFriendRepo) RemoveGroupMember(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockFriendRepo) GetGroupMembers(_ context.Context, _ int64) ([]model.GroupMember, error) {
	return nil, nil
}
func (m *mockFriendRepo) UpdateGroupMemberRole(_ context.Context, _ int, _ int, _ int) error {
	return nil
}

func (m *mockFriendRepo) CreateMoment(_ context.Context, _ *model.Moment) error          { return nil }
func (m *mockFriendRepo) GetMomentByID(_ context.Context, _ int64) (*model.Moment, error) { return nil, nil }
func (m *mockFriendRepo) GetMomentsByUser(_ context.Context, _ int64, _ int, _ int) ([]model.Moment, error) {
	return nil, nil
}
func (m *mockFriendRepo) CreateMomentLike(_ context.Context, _ *model.MomentLike) error    { return nil }
func (m *mockFriendRepo) DeleteMomentLike(_ context.Context, _ int64, _ int64) error       { return nil }
func (m *mockFriendRepo) CreateMomentComment(_ context.Context, _ *model.MomentComment) error { return nil }
func (m *mockFriendRepo) DeleteMomentComment(_ context.Context, _ int64) error              { return nil }

func (m *mockFriendRepo) CreateAISummary(_ context.Context, _ *model.AISummary) error        { return nil }
func (m *mockFriendRepo) CreateAIProfileItem(_ context.Context, _ *model.AIProfileItem) error { return nil }
func (m *mockFriendRepo) GetAIProfileByUser(_ context.Context, _ int64) ([]model.AIProfileItem, error) {
	return nil, nil
}

// ──────────────────────────────────────────────────────
// Mock RedisRepo for friend tests
// ──────────────────────────────────────────────────────

type mockFriendRedisRepo struct{}

func (m *mockFriendRedisRepo) WriteInbox(_ context.Context, _ int64, _ *model.InboxMessage) error { return nil }
func (m *mockFriendRedisRepo) WriteOutbox(_ context.Context, _ int64, _ *model.InboxMessage) error { return nil }
func (m *mockFriendRedisRepo) ReadInbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	return nil, nil
}
func (m *mockFriendRedisRepo) ReadOutbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	return nil, nil
}
func (m *mockFriendRedisRepo) UpdateConvList(_ context.Context, _ int64, _ string, _ string, _ int64) error {
	return nil
}
func (m *mockFriendRedisRepo) GetConvList(_ context.Context, _ int64) ([]model.ConvSummary, error) {
	return nil, nil
}
func (m *mockFriendRedisRepo) IncrementUnread(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockFriendRedisRepo) ClearUnread(_ context.Context, _ int64, _ string) error     { return nil }
func (m *mockFriendRedisRepo) GetUnreadMap(_ context.Context, _ int64) (map[string]int64, error) {
	return nil, nil
}
func (m *mockFriendRedisRepo) SetGroupReadPos(_ context.Context, _ int64, _ string, _ int64) error { return nil }
func (m *mockFriendRedisRepo) GetGroupReadPos(_ context.Context, _ int64, _ string) (int64, error) { return 0, nil }
func (m *mockFriendRedisRepo) GetGroupMemberships(_ context.Context, _ int64) ([]int64, error)     { return nil, nil }
func (m *mockFriendRedisRepo) GetGroupMembers(_ context.Context, _ int64) ([]int64, error)         { return nil, nil }
func (m *mockFriendRedisRepo) CheckDuplicate(_ context.Context, _ int64, _ string) (bool, error)  { return false, nil }
func (m *mockFriendRedisRepo) TrimInbox(_ context.Context, _ int64, _ int) error                  { return nil }
func (m *mockFriendRedisRepo) TrimOutbox(_ context.Context, _ int64, _ int) error                 { return nil }
func (m *mockFriendRedisRepo) TrimInboxByTime(_ context.Context, _ int64, _ int64) error          { return nil }
func (m *mockFriendRedisRepo) TrimOutboxByTime(_ context.Context, _ int64, _ int64) error         { return nil }
func (m *mockFriendRedisRepo) TrimConvListByTime(_ context.Context, _ int64, _ int64) error       { return nil }
func (m *mockFriendRedisRepo) TrimTimelineByTime(_ context.Context, _ int64, _ int64) error       { return nil }

func (m *mockFriendRedisRepo) ExecPrivateMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redislua.PrivateMsgCheckResult, error) {
	return nil, nil
}
func (m *mockFriendRedisRepo) ExecGroupMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redislua.GroupMsgCheckResult, error) {
	return nil, nil
}
func (m *mockFriendRedisRepo) ExecInboxMarkRead(_ context.Context, _ int64, _ string) (int64, error) { return 0, nil }
func (m *mockFriendRedisRepo) ExecRevokeMsg(_ context.Context, _ int64, _ string, _ int64, _ string, _ int64) (bool, error) {
	return false, nil
}

// ──────────────────────────────────────────────────────
// Helper: new test FriendService
// ──────────────────────────────────────────────────────

func newTestFriendService(repo *mockFriendRepo) *FriendService {
	logger := zap.NewNop()
	return NewFriendService(repo, &mockFriendRedisRepo{}, logger)
}

// ──────────────────────────────────────────────────────
// SendFriendRequest tests
// ──────────────────────────────────────────────────────

func TestFriend_SendFriendRequest_Success(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Add two users
	repo.mu.Lock()
	repo.users[1] = &model.User{ID: 1, Nickname: "alice", AvatarURL: "avatar1"}
	repo.users[2] = &model.User{ID: 2, Nickname: "bob", AvatarURL: "avatar2"}
	repo.mu.Unlock()

	req, err := svc.SendFriendRequest(context.Background(), 1, 2, "let's be friends")
	assert.NoError(t, err)
	assert.NotNil(t, req)
	assert.Equal(t, int64(1), req.ID)
	assert.Equal(t, int64(1), req.FromUserID)
	assert.Equal(t, int64(2), req.ToUserID)
	assert.Equal(t, 0, req.Status)
	assert.Equal(t, "let's be friends", req.Message)
}

func TestFriend_SendFriendRequest_SelfRequest(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	_, err := svc.SendFriendRequest(context.Background(), 1, 1, "")
	assert.Error(t, err)
	assert.Equal(t, ErrSelfRequest, err.Error())
}

func TestFriend_SendFriendRequest_AlreadyFriends(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Set up an existing friendship
	repo.mu.Lock()
	repo.friendships[friendKey(1, 2)] = &model.Friendship{UserID: 1, FriendID: 2}
	repo.friendships[friendKey(2, 1)] = &model.Friendship{UserID: 2, FriendID: 1}
	repo.mu.Unlock()

	_, err := svc.SendFriendRequest(context.Background(), 1, 2, "")
	assert.Error(t, err)
	assert.Equal(t, ErrAlreadyFriends, err.Error())
}

func TestFriend_SendFriendRequest_Blocked(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Set up a blacklist entry: user 2 blocked user 1
	repo.mu.Lock()
	repo.blacklist[blackKey(2, 1)] = &model.Blacklist{UserID: 2, BlockedID: 1}
	repo.mu.Unlock()

	_, err := svc.SendFriendRequest(context.Background(), 1, 2, "")
	assert.Error(t, err)
	assert.Equal(t, ErrFriendBlocked, err.Error())
}

func TestFriend_SendFriendRequest_BlockedBySender(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Set up a blacklist entry: user 1 blocked user 2
	repo.mu.Lock()
	repo.blacklist[blackKey(1, 2)] = &model.Blacklist{UserID: 1, BlockedID: 2}
	repo.mu.Unlock()

	_, err := svc.SendFriendRequest(context.Background(), 1, 2, "")
	assert.Error(t, err)
	assert.Equal(t, ErrFriendBlocked, err.Error())
}

func TestFriend_SendFriendRequest_DuplicatePending(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// First request succeeds
	_, err := svc.SendFriendRequest(context.Background(), 1, 2, "hello")
	assert.NoError(t, err)

	// Second request from same user to same target should fail
	_, err = svc.SendFriendRequest(context.Background(), 1, 2, "hello again")
	assert.Error(t, err)
	assert.Equal(t, ErrDuplicateRequest, err.Error())
}

func TestFriend_SendFriendRequest_DuplicatePendingReverse(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// User 2 sends request to user 1
	_, err := svc.SendFriendRequest(context.Background(), 2, 1, "hello")
	assert.NoError(t, err)

	// User 1 tries to send request to user 2 — should detect existing pending request
	_, err = svc.SendFriendRequest(context.Background(), 1, 2, "hello back")
	assert.Error(t, err)
	assert.Equal(t, ErrDuplicateRequest, err.Error())
}

// ──────────────────────────────────────────────────────
// AcceptFriendRequest tests
// ──────────────────────────────────────────────────────

func TestFriend_AcceptFriendRequest_Success(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Create a friend request first
	req, err := svc.SendFriendRequest(context.Background(), 1, 2, "hello")
	assert.NoError(t, err)

	// Accept it as the target user (user 2)
	fs, err := svc.AcceptFriendRequest(context.Background(), 2, req.ID)
	assert.NoError(t, err)
	assert.NotNil(t, fs)
	assert.Equal(t, int64(1), fs.UserID)
	assert.Equal(t, int64(2), fs.FriendID)

	// Verify the request status was updated
	repo.mu.Lock()
	storedReq := repo.friendRequests[req.ID]
	repo.mu.Unlock()
	assert.Equal(t, 1, storedReq.Status)

	// Verify the friendship was created bidirectionally
	isFriend, err := repo.IsFriend(context.Background(), 1, 2)
	assert.NoError(t, err)
	assert.True(t, isFriend)
	isFriend, err = repo.IsFriend(context.Background(), 2, 1)
	assert.NoError(t, err)
	assert.True(t, isFriend)
}

func TestFriend_AcceptFriendRequest_WrongTarget(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Create a friend request: user 1 → user 2
	req, err := svc.SendFriendRequest(context.Background(), 1, 2, "hello")
	assert.NoError(t, err)

	// Try to accept it as user 3 (wrong target)
	_, err = svc.AcceptFriendRequest(context.Background(), 3, req.ID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotRequestTarget, err.Error())
}

func TestFriend_AcceptFriendRequest_NotFound(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	_, err := svc.AcceptFriendRequest(context.Background(), 2, 999)
	assert.Error(t, err)
	assert.Equal(t, ErrRequestNotFound, err.Error())
}

// ──────────────────────────────────────────────────────
// RejectFriendRequest tests
// ──────────────────────────────────────────────────────

func TestFriend_RejectFriendRequest_Success(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Create a friend request
	req, err := svc.SendFriendRequest(context.Background(), 1, 2, "hello")
	assert.NoError(t, err)

	// Reject it as the target user (user 2)
	err = svc.RejectFriendRequest(context.Background(), 2, req.ID)
	assert.NoError(t, err)

	// Verify the request status was updated
	repo.mu.Lock()
	storedReq := repo.friendRequests[req.ID]
	repo.mu.Unlock()
	assert.Equal(t, 2, storedReq.Status)
}

func TestFriend_RejectFriendRequest_WrongTarget(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Create a friend request: user 1 → user 2
	req, err := svc.SendFriendRequest(context.Background(), 1, 2, "hello")
	assert.NoError(t, err)

	// Try to reject it as user 3 (wrong target)
	err = svc.RejectFriendRequest(context.Background(), 3, req.ID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotRequestTarget, err.Error())
}

func TestFriend_RejectFriendRequest_NotFound(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	err := svc.RejectFriendRequest(context.Background(), 2, 999)
	assert.Error(t, err)
	assert.Equal(t, ErrRequestNotFound, err.Error())
}

// ──────────────────────────────────────────────────────
// GetFriendRequests tests
// ──────────────────────────────────────────────────────

func TestFriend_GetFriendRequests(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Create two friend requests involving user 1
	_, err := svc.SendFriendRequest(context.Background(), 1, 2, "hello")
	assert.NoError(t, err)
	_, err = svc.SendFriendRequest(context.Background(), 3, 1, "hi")
	assert.NoError(t, err)

	requests, err := svc.GetFriendRequests(context.Background(), 1)
	assert.NoError(t, err)
	assert.Len(t, requests, 2)
}

// ──────────────────────────────────────────────────────
// DeleteFriend tests
// ──────────────────────────────────────────────────────

func TestFriend_DeleteFriend_Success(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Set up a friendship
	repo.mu.Lock()
	repo.friendships[friendKey(1, 2)] = &model.Friendship{UserID: 1, FriendID: 2}
	repo.friendships[friendKey(2, 1)] = &model.Friendship{UserID: 2, FriendID: 1}
	repo.mu.Unlock()

	err := svc.DeleteFriend(context.Background(), 1, 2)
	assert.NoError(t, err)

	// Verify bidirectional deletion
	repo.mu.Lock()
	_, ok1 := repo.friendships[friendKey(1, 2)]
	_, ok2 := repo.friendships[friendKey(2, 1)]
	repo.mu.Unlock()
	assert.False(t, ok1)
	assert.False(t, ok2)
}

// ──────────────────────────────────────────────────────
// BlockUser / UnblockUser / IsBlocked tests
// ──────────────────────────────────────────────────────

func TestFriend_BlockUser_Success(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	err := svc.BlockUser(context.Background(), 1, 2)
	assert.NoError(t, err)

	// Verify blocked
	isBlocked, err := svc.IsBlocked(context.Background(), 1, 2)
	assert.NoError(t, err)
	assert.True(t, isBlocked)
}

func TestFriend_BlockUser_AlreadyBlocked(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Block once
	err := svc.BlockUser(context.Background(), 1, 2)
	assert.NoError(t, err)

	// Block again should fail
	err = svc.BlockUser(context.Background(), 1, 2)
	assert.Error(t, err)
	assert.Equal(t, ErrAlreadyBlocked, err.Error())
}

func TestFriend_UnblockUser_Success(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Block first
	err := svc.BlockUser(context.Background(), 1, 2)
	assert.NoError(t, err)

	// Unblock
	err = svc.UnblockUser(context.Background(), 1, 2)
	assert.NoError(t, err)

	// Verify unblocked
	isBlocked, err := svc.IsBlocked(context.Background(), 1, 2)
	assert.NoError(t, err)
	assert.False(t, isBlocked)
}

func TestFriend_IsBlocked_NotBlocked(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	isBlocked, err := svc.IsBlocked(context.Background(), 1, 2)
	assert.NoError(t, err)
	assert.False(t, isBlocked)
}

// ──────────────────────────────────────────────────────
// GetFriendList tests
// ──────────────────────────────────────────────────────

func TestFriend_GetFriendList_WithProfile(t *testing.T) {
	repo := newMockFriendRepo()
	svc := newTestFriendService(repo)

	// Set up friendship and users
	repo.mu.Lock()
	repo.friendships[friendKey(1, 2)] = &model.Friendship{ID: 1, UserID: 1, FriendID: 2, CreatedAt: time.Now()}
	repo.users[2] = &model.User{ID: 2, Nickname: "bob", AvatarURL: "avatar2"}
	repo.mu.Unlock()

	friends, err := svc.GetFriendList(context.Background(), 1)
	assert.NoError(t, err)
	assert.Len(t, friends, 1)
	assert.Equal(t, "bob", friends[0].Nickname)
	assert.Equal(t, "avatar2", friends[0].AvatarURL)
}
