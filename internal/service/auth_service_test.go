package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"

	"github.com/goim/goim/internal/middleware"
	"github.com/goim/goim/internal/model"
)

// ──────────────────────────────────────────────────────
// Mock MySQLRepo for auth tests
// ──────────────────────────────────────────────────────

type mockAuthRepo struct {
	mu sync.Mutex

	// Stored users keyed by username
	usersByUsername map[string]*model.User
	// Stored users keyed by ID
	usersByID map[int64]*model.User
	// Next auto-increment ID
	nextID int64

	// Error overrides (set before calling service methods)
	getByUsernameErr error
	getByIDErr       error
	createErr        error
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{
		usersByUsername: make(map[string]*model.User),
		usersByID:       make(map[int64]*model.User),
		nextID:          1,
	}
}

func (m *mockAuthRepo) GetUserByUsername(_ context.Context, username string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getByUsernameErr != nil {
		return nil, m.getByUsernameErr
	}
	u, ok := m.usersByUsername[username]
	if !ok {
		return nil, nil // not found
	}
	return u, nil
}

func (m *mockAuthRepo) GetUserByID(_ context.Context, userID int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	u, ok := m.usersByID[userID]
	if !ok {
		return nil, nil // not found
	}
	return u, nil
}

func (m *mockAuthRepo) CreateUser(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	user.ID = m.nextID
	m.nextID++
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.usersByUsername[user.Username] = user
	m.usersByID[user.ID] = user
	return nil
}

// stub out all other MySQLRepo methods that auth tests don't use

func (m *mockAuthRepo) UpdateUser(_ context.Context, _ *model.User) error { return nil }

func (m *mockAuthRepo) InsertPrivateMessage(_ context.Context, _ *model.PrivateMessage) error { return nil }
func (m *mockAuthRepo) InsertGroupMessage(_ context.Context, _ *model.GroupMessage) error       { return nil }
func (m *mockAuthRepo) InsertMsgRevoked(_ context.Context, _ *model.MsgRevoked) error           { return nil }

func (m *mockAuthRepo) CreateFriendRequest(_ context.Context, _ *model.FriendRequest) error { return nil }
func (m *mockAuthRepo) UpdateFriendRequest(_ context.Context, _ *model.FriendRequest) error { return nil }
func (m *mockAuthRepo) GetFriendRequestByID(_ context.Context, _ int64) (*model.FriendRequest, error) {
	return nil, nil
}
func (m *mockAuthRepo) GetFriendRequestsByUser(_ context.Context, _ int64) ([]model.FriendRequest, error) {
	return nil, nil
}
func (m *mockAuthRepo) CreateFriendship(_ context.Context, _ *model.Friendship) error { return nil }
func (m *mockAuthRepo) DeleteFriendship(_ context.Context, _ int64, _ int64) error    { return nil }
func (m *mockAuthRepo) GetFriendList(_ context.Context, _ int64) ([]model.Friendship, error) {
	return nil, nil
}
func (m *mockAuthRepo) IsFriend(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil }
func (m *mockAuthRepo) CreateBlacklist(_ context.Context, _ *model.Blacklist) error { return nil }
func (m *mockAuthRepo) DeleteBlacklist(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockAuthRepo) IsBlocked(_ context.Context, _ int64, _ int64) (bool, error) { return false, nil }

func (m *mockAuthRepo) CreateGroup(_ context.Context, _ *model.Group) (int64, error) { return 0, nil }
func (m *mockAuthRepo) UpdateGroup(_ context.Context, _ *model.Group) error           { return nil }
func (m *mockAuthRepo) GetGroupByID(_ context.Context, _ int64) (*model.Group, error) { return nil, nil }
func (m *mockAuthRepo) AddGroupMember(_ context.Context, _ *model.GroupMember) error  { return nil }
func (m *mockAuthRepo) RemoveGroupMember(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *mockAuthRepo) GetGroupMembers(_ context.Context, _ int64) ([]model.GroupMember, error) {
	return nil, nil
}
func (m *mockAuthRepo) UpdateGroupMemberRole(_ context.Context, _ int, _ int, _ int) error {
	return nil
}

func (m *mockAuthRepo) CreateMoment(_ context.Context, _ *model.Moment) error          { return nil }
func (m *mockAuthRepo) GetMomentByID(_ context.Context, _ int64) (*model.Moment, error) { return nil, nil }
func (m *mockAuthRepo) GetMomentsByUser(_ context.Context, _ int64, _ int, _ int) ([]model.Moment, error) {
	return nil, nil
}
func (m *mockAuthRepo) CreateMomentLike(_ context.Context, _ *model.MomentLike) error    { return nil }
func (m *mockAuthRepo) DeleteMomentLike(_ context.Context, _ int64, _ int64) error       { return nil }
func (m *mockAuthRepo) CreateMomentComment(_ context.Context, _ *model.MomentComment) error { return nil }
func (m *mockAuthRepo) DeleteMomentComment(_ context.Context, _ int64) error              { return nil }

func (m *mockAuthRepo) CreateAISummary(_ context.Context, _ *model.AISummary) error        { return nil }
func (m *mockAuthRepo) CreateAIProfileItem(_ context.Context, _ *model.AIProfileItem) error { return nil }
func (m *mockAuthRepo) GetAIProfileByUser(_ context.Context, _ int64) ([]model.AIProfileItem, error) {
	return nil, nil
}

// ──────────────────────────────────────────────────────
// Helper: new test AuthService
// ──────────────────────────────────────────────────────

func newTestAuthService(repo *mockAuthRepo) *AuthService {
	return NewAuthService(repo, "test-secret-key", 2, 7)
}

// ──────────────────────────────────────────────────────
// Register tests
// ──────────────────────────────────────────────────────

func TestAuth_Register_Success(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	userID, username, err := svc.Register(context.Background(), "alice", "password123")
	assert.NoError(t, err)
	assert.Equal(t, "alice", username)
	assert.Equal(t, int64(1), userID)

	// Verify stored user
	stored, ok := repo.usersByUsername["alice"]
	assert.True(t, ok)
	assert.NotEmpty(t, stored.PasswordHash)

	// Verify bcrypt hash is valid
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("password123")))
}

func TestAuth_Register_DuplicateUsername(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	_, _, err := svc.Register(context.Background(), "bob", "password123")
	assert.NoError(t, err)

	_, _, err = svc.Register(context.Background(), "bob", "different123")
	assert.Error(t, err)
	assert.Equal(t, ErrUsernameTaken, err.Error())
}

func TestAuth_Register_UsernameTooShort(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	_, _, err := svc.Register(context.Background(), "ab", "password123")
	assert.Error(t, err)
	assert.Equal(t, ErrUsernameTooShort, err.Error())
}

func TestAuth_Register_UsernameTooLong(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	longName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 51 chars
	_, _, err := svc.Register(context.Background(), longName, "password123")
	assert.Error(t, err)
	assert.Equal(t, ErrUsernameTooShort, err.Error())
}

func TestAuth_Register_PasswordTooShort(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	_, _, err := svc.Register(context.Background(), "charlie", "12345")
	assert.Error(t, err)
	assert.Equal(t, ErrPasswordTooShort, err.Error())
}

func TestAuth_Register_RepoCreateError(t *testing.T) {
	repo := newMockAuthRepo()
	repo.createErr = fmt.Errorf("db error")
	svc := newTestAuthService(repo)

	_, _, err := svc.Register(context.Background(), "dave", "password123")
	assert.Error(t, err)
	// should wrap the underlying error
	assert.Contains(t, err.Error(), "create user")
}

// ──────────────────────────────────────────────────────
// Login tests
// ──────────────────────────────────────────────────────

func TestAuth_Login_Success(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	// Register a user first
	userID, _, err := svc.Register(context.Background(), "loginuser", "mypassword")
	assert.NoError(t, err)

	// Now login
	accessToken, refreshToken, expiresIn, err := svc.Login(context.Background(), "loginuser", "mypassword")
	assert.NoError(t, err)
	assert.NotEmpty(t, accessToken)
	assert.NotEmpty(t, refreshToken)
	assert.Equal(t, int64(2*3600), expiresIn)

	// Validate the access token contains correct userID
	_, claims, err := middleware.ParseToken(accessToken, "test-secret-key")
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, "loginuser", claims.Username)

	// Validate the refresh token contains correct userID
	_, claims, err = middleware.ParseToken(refreshToken, "test-secret-key")
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
}

func TestAuth_Login_WrongPassword(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	_, _, err := svc.Register(context.Background(), "loginuser", "mypassword")
	assert.NoError(t, err)

	_, _, _, err = svc.Login(context.Background(), "loginuser", "wrongpassword")
	assert.Error(t, err)
	assert.Equal(t, ErrWrongPassword, err.Error())
}

func TestAuth_Login_NonexistentUser(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	_, _, _, err := svc.Login(context.Background(), "ghost", "password123")
	assert.Error(t, err)
	assert.Equal(t, ErrUserNotFound, err.Error())
}

// ──────────────────────────────────────────────────────
// Refresh tests
// ──────────────────────────────────────────────────────

func TestAuth_Refresh_Success(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	// Register and login to get a refresh token
	userID, _, err := svc.Register(context.Background(), "refreshuser", "password123")
	assert.NoError(t, err)
	_, refreshToken, _, err := svc.Login(context.Background(), "refreshuser", "password123")
	assert.NoError(t, err)

	// Refresh
	accessToken, expiresIn, err := svc.Refresh(context.Background(), refreshToken)
	assert.NoError(t, err)
	assert.NotEmpty(t, accessToken)
	assert.Equal(t, int64(2*3600), expiresIn)

	// Validate new access token
	_, claims, err := middleware.ParseToken(accessToken, "test-secret-key")
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, "refreshuser", claims.Username)
}

func TestAuth_Refresh_InvalidToken(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	_, _, err := svc.Refresh(context.Background(), "garbage-token-string")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err.Error())
}

func TestAuth_Refresh_UserDeleted(t *testing.T) {
	repo := newMockAuthRepo()
	svc := newTestAuthService(repo)

	// Register and login
	_, _, err := svc.Register(context.Background(), "deleteduser", "password123")
	assert.NoError(t, err)
	_, refreshToken, _, err := svc.Login(context.Background(), "deleteduser", "password123")
	assert.NoError(t, err)

	// Simulate user deletion — remove from mock repo
	repo.mu.Lock()
	delete(repo.usersByID, 1)
	delete(repo.usersByUsername, "deleteduser")
	repo.mu.Unlock()

	// Refresh should fail
	_, _, err = svc.Refresh(context.Background(), refreshToken)
	assert.Error(t, err)
	assert.Equal(t, ErrUserNotFound, err.Error())
}

func TestAuth_Refresh_ExpiredRefreshToken(t *testing.T) {
	repo := newMockAuthRepo()
	// Use very short refresh expiry (0 days = already expired)
	svc := NewAuthService(repo, "test-secret-key", 2, 0)

	_, _, err := svc.Register(context.Background(), "expireduser", "password123")
	assert.NoError(t, err)

	// Generate a refresh token that's already expired (0 days)
	_, refreshToken, _, err := svc.Login(context.Background(), "expireduser", "password123")
	assert.NoError(t, err)

	// The refresh token should be expired; parsing should fail
	_, _, err = middleware.ParseToken(refreshToken, "test-secret-key")
	assert.Error(t, err) // token should be expired
}
