package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/redis"
)

// ── Mock implementations ──

// mockMySQLRepo implements repository.MySQLRepo for moment tests.
// Only moment-related methods are implemented; others panic.
type mockMySQLRepo struct {
	moments   map[int64]*model.Moment
	likes     map[string]*model.MomentLike // key: "momentID:userID"
	comments  map[int64]*model.MomentComment
	nextID    int64
	likeErr   error // injectable error for CreateMomentLike
	commentErr error // injectable error for CreateMomentComment
}

func newMockMySQLRepo() *mockMySQLRepo {
	return &mockMySQLRepo{
		moments:   make(map[int64]*model.Moment),
		likes:     make(map[string]*model.MomentLike),
		comments:  make(map[int64]*model.MomentComment),
		nextID:    1,
	}
}

func (m *mockMySQLRepo) CreateMoment(ctx context.Context, moment *model.Moment) error {
	moment.ID = m.nextID
	m.nextID++
	m.moments[moment.ID] = moment
	return nil
}

func (m *mockMySQLRepo) GetMomentByID(ctx context.Context, id int64) (*model.Moment, error) {
	moment, ok := m.moments[id]
	if !ok {
		return nil, nil
	}
	return moment, nil
}

func (m *mockMySQLRepo) GetMomentsByUser(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error) {
	var result []model.Moment
	for _, moment := range m.moments {
		if moment.AuthorID == userID {
			result = append(result, *moment)
		}
	}
	return result, nil
}

func (m *mockMySQLRepo) CreateMomentLike(ctx context.Context, like *model.MomentLike) error {
	if m.likeErr != nil {
		return m.likeErr
	}
	key := fmt.Sprintf("%d:%d", like.MomentID, like.UserID)
	if _, exists := m.likes[key]; exists {
		return fmt.Errorf("duplicate like")
	}
	m.likes[key] = like
	return nil
}

func (m *mockMySQLRepo) DeleteMomentLike(ctx context.Context, momentID, userID int64) error {
	key := fmt.Sprintf("%d:%d", momentID, userID)
	delete(m.likes, key)
	return nil
}

func (m *mockMySQLRepo) CreateMomentComment(ctx context.Context, comment *model.MomentComment) error {
	if m.commentErr != nil {
		return m.commentErr
	}
	m.comments[comment.ID] = comment
	return nil
}

func (m *mockMySQLRepo) GetMomentCommentByID(ctx context.Context, id int64) (*model.MomentComment, error) {
	comment, ok := m.comments[id]
	if !ok {
		return nil, nil
	}
	return comment, nil
}

func (m *mockMySQLRepo) DeleteMomentComment(ctx context.Context, id int64) error {
	delete(m.comments, id)
	return nil
}

// Stub out all other MySQLRepo methods with panics

func (m *mockMySQLRepo) InsertPrivateMessage(_ context.Context, _ *model.PrivateMessage) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) InsertGroupMessage(_ context.Context, _ *model.GroupMessage) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) InsertMsgRevoked(_ context.Context, _ *model.MsgRevoked) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetUserByID(_ context.Context, _ int64) (*model.User, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetUserByUsername(_ context.Context, _ string) (*model.User, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateUser(_ context.Context, _ *model.User) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) UpdateUser(_ context.Context, _ *model.User) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateFriendRequest(_ context.Context, _ *model.FriendRequest) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) UpdateFriendRequest(_ context.Context, _ *model.FriendRequest) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetFriendRequestByID(_ context.Context, _ int64) (*model.FriendRequest, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetFriendRequestsByUser(_ context.Context, _ int64) ([]model.FriendRequest, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateFriendship(_ context.Context, _ *model.Friendship) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) DeleteFriendship(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetFriendList(_ context.Context, _ int64) ([]model.Friendship, error) {
	return nil, nil
}
func (m *mockMySQLRepo) IsFriend(_ context.Context, _ int64, _ int64) (bool, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateBlacklist(_ context.Context, _ *model.Blacklist) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) DeleteBlacklist(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) IsBlocked(_ context.Context, _ int64, _ int64) (bool, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateGroup(_ context.Context, _ *model.Group) (int64, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) UpdateGroup(_ context.Context, _ *model.Group) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetGroupByID(_ context.Context, _ int64) (*model.Group, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) AddGroupMember(_ context.Context, _ *model.GroupMember) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) RemoveGroupMember(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetGroupMembers(_ context.Context, _ int64) ([]model.GroupMember, error) {
	panic("not implemented")
}
func (m *mockMySQLRepo) UpdateGroupMemberRole(_ context.Context, _ int, _ int, _ int) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateAISummary(_ context.Context, _ *model.AISummary) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) CreateAIProfileItem(_ context.Context, _ *model.AIProfileItem) error {
	panic("not implemented")
}
func (m *mockMySQLRepo) GetAIProfileByUser(_ context.Context, _ int64) ([]model.AIProfileItem, error) {
	panic("not implemented")
}

// momentMockRedisRepo implements repository.RedisRepo for moment tests.
// Only moment feed methods are implemented; others panic.
type momentMockRedisRepo struct {
	timelines map[int64][]int64 // userID -> list of momentIDs
}

func newMomentMockRedisRepo() *momentMockRedisRepo {
	return &momentMockRedisRepo{
		timelines: make(map[int64][]int64),
	}
}

func (m *momentMockRedisRepo) PublishMomentFeed(ctx context.Context, userID int64, momentID int64, timestamp int64) error {
	m.timelines[userID] = append(m.timelines[userID], momentID)
	return nil
}

func (m *momentMockRedisRepo) GetMomentFeed(ctx context.Context, userID int64, lastSyncTime int64, limit int) ([]int64, error) {
	timeline, ok := m.timelines[userID]
	if !ok {
		return []int64{}, nil
	}
	if limit > len(timeline) {
		return timeline, nil
	}
	return timeline[:limit], nil
}

// Stub out all other RedisRepo methods with panics

func (m *momentMockRedisRepo) WriteInbox(_ context.Context, _ int64, _ *model.InboxMessage) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) WriteOutbox(_ context.Context, _ int64, _ *model.InboxMessage) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ReadInbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ReadOutbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) UpdateConvList(_ context.Context, _ int64, _ string, _ string, _ int64) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) GetConvList(_ context.Context, _ int64) ([]model.ConvSummary, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) IncrementUnread(_ context.Context, _ int64, _ string) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ClearUnread(_ context.Context, _ int64, _ string) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) GetUnreadMap(_ context.Context, _ int64) (map[string]int64, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) SetGroupReadPos(_ context.Context, _ int64, _ string, _ int64) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) GetGroupReadPos(_ context.Context, _ int64, _ string) (int64, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) GetGroupMemberships(_ context.Context, _ int64) ([]int64, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) GetGroupMembers(_ context.Context, _ int64) ([]int64, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) CheckDuplicate(_ context.Context, _ int64, _ string) (bool, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) TrimInbox(_ context.Context, _ int64, _ int) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) TrimOutbox(_ context.Context, _ int64, _ int) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) TrimInboxByTime(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) TrimOutboxByTime(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) TrimConvListByTime(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) TrimTimelineByTime(_ context.Context, _ int64, _ int64) error {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ExecPrivateMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.PrivateMsgCheckResult, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ExecGroupMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.GroupMsgCheckResult, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ExecInboxMarkRead(_ context.Context, _ int64, _ string) (int64, error) {
	panic("not implemented")
}
func (m *momentMockRedisRepo) ExecRevokeMsg(_ context.Context, _ int64, _ string, _ int64, _ string, _ int64) (bool, error) {
	panic("not implemented")
}

// momentMockMQRepo implements repository.MQRepo for moment tests.
type momentMockMQRepo struct {
	publishedMoments []*model.Moment
}

func newMomentMockMQRepo() *momentMockMQRepo {
	return &momentMockMQRepo{
		publishedMoments: make([]*model.Moment, 0),
	}
}

func (m *momentMockMQRepo) PublishPrivateMsg(_ context.Context, _ *model.PrivateMessage) error {
	panic("not implemented")
}
func (m *momentMockMQRepo) PublishGroupMsg(_ context.Context, _ *model.GroupMessage) error {
	panic("not implemented")
}
func (m *momentMockMQRepo) PublishMomentPush(_ context.Context, moment *model.Moment) error {
	m.publishedMoments = append(m.publishedMoments, moment)
	return nil
}

// ── Tests ──

func TestPublishMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	momentID, err := svc.PublishMoment(context.Background(), 100, "Hello world!", nil, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), momentID)

	// Verify moment was stored in MySQL
	moment, ok := mysqlRepo.moments[momentID]
	assert.True(t, ok)
	assert.Equal(t, int64(100), moment.AuthorID)
	assert.Equal(t, "Hello world!", moment.Content)
	assert.Equal(t, 1, moment.Visibility)

	// Verify moment was published to MQ
	assert.Len(t, mqRepo.publishedMoments, 1)
	assert.Equal(t, momentID, mqRepo.publishedMoments[0].ID)
}

func TestPublishMoment_EmptyContent(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	_, err := svc.PublishMoment(context.Background(), 100, "", nil, 1)
	assert.Error(t, err)
	assert.Equal(t, ErrMomentContentEmpty, err.Error())
}

func TestPublishMoment_InvalidVisibility(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	_, err := svc.PublishMoment(context.Background(), 100, "Hello", nil, 4)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidVisibility, err.Error())
}

func TestGetMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	moment, err := svc.GetMoment(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, moment)
	assert.Equal(t, int64(100), moment.AuthorID)
	assert.Equal(t, "Test moment", moment.Content)
}

func TestGetMoment_NotFound(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	moment, err := svc.GetMoment(context.Background(), 999)
	assert.Error(t, err)
	assert.Equal(t, ErrMomentNotFound, err.Error())
	assert.Nil(t, moment)
}

func TestLikeMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	err := svc.LikeMoment(context.Background(), 200, 1)
	assert.NoError(t, err)

	// Verify like was stored
	key := fmt.Sprintf("%d:%d", 1, 200)
	_, ok := mysqlRepo.likes[key]
	assert.True(t, ok)
}

func TestLikeMoment_MomentNotFound(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	err := svc.LikeMoment(context.Background(), 200, 999)
	assert.Error(t, err)
	assert.Equal(t, ErrMomentNotFound, err.Error())
}

func TestLikeMoment_AlreadyLiked(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	// First like should succeed
	err := svc.LikeMoment(context.Background(), 200, 1)
	assert.NoError(t, err)

	// Second like should fail with duplicate error
	err = svc.LikeMoment(context.Background(), 200, 1)
	assert.Error(t, err)
	// The error message should contain ErrAlreadyLiked prefix
	assert.Contains(t, err.Error(), ErrAlreadyLiked)
}

func TestUnlikeMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment and a like
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})
	mysqlRepo.CreateMomentLike(context.Background(), &model.MomentLike{
		MomentID:  1,
		UserID:    200,
		CreatedAt: time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	err := svc.UnlikeMoment(context.Background(), 200, 1)
	assert.NoError(t, err)

	// Verify like was removed
	key := fmt.Sprintf("%d:%d", 1, 200)
	_, ok := mysqlRepo.likes[key]
	assert.False(t, ok)
}

func TestCommentMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	commentID, err := svc.CommentMoment(context.Background(), 200, 1, "Great post!")
	assert.NoError(t, err)
	assert.NotZero(t, commentID)

	// Verify comment was stored
	comment, ok := mysqlRepo.comments[commentID]
	assert.True(t, ok)
	assert.Equal(t, int64(200), comment.UserID)
	assert.Equal(t, int64(1), comment.MomentID)
	assert.Equal(t, "Great post!", comment.Content)
}

func TestCommentMoment_MomentNotFound(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	_, err := svc.CommentMoment(context.Background(), 200, 999, "comment")
	assert.Error(t, err)
	assert.Equal(t, ErrMomentNotFound, err.Error())
}

func TestCommentMoment_EmptyContent(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	_, err := svc.CommentMoment(context.Background(), 200, 1, "")
	assert.Error(t, err)
	assert.Equal(t, ErrMomentContentEmpty, err.Error())
}

func TestDeleteComment_OwnershipValidation(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment and a comment by user 200
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})
	commentID := time.Now().UnixNano()
	mysqlRepo.CreateMomentComment(context.Background(), &model.MomentComment{
		ID:        commentID,
		MomentID:  1,
		UserID:    200, // comment belongs to user 200
		Content:   "Great post!",
		CreatedAt: time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	// User 200 (owner) should be able to delete their own comment
	err := svc.DeleteComment(context.Background(), 200, commentID)
	assert.NoError(t, err)

	// Verify comment was removed
	_, ok := mysqlRepo.comments[commentID]
	assert.False(t, ok)
}

func TestDeleteComment_NotOwner(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate a moment and a comment by user 200
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})
	commentID := time.Now().UnixNano()
	mysqlRepo.CreateMomentComment(context.Background(), &model.MomentComment{
		ID:        commentID,
		MomentID:  1,
		UserID:    200, // comment belongs to user 200
		Content:   "Great post!",
		CreatedAt: time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	// User 300 (not owner) should NOT be able to delete user 200's comment
	err := svc.DeleteComment(context.Background(), 300, commentID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotCommentOwner, err.Error())

	// Verify comment was NOT removed
	_, ok := mysqlRepo.comments[commentID]
	assert.True(t, ok)
}

func TestDeleteComment_NotFound(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	err := svc.DeleteComment(context.Background(), 200, 9999999)
	assert.Error(t, err)
	assert.Equal(t, ErrCommentNotFound, err.Error())
}

func TestGetFeed_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// Pre-populate moments
	m1 := &model.Moment{
		AuthorID:   100,
		Content:    "Moment 1",
		Visibility: 1,
		CreatedAt:  time.Now(),
	}
	m2 := &model.Moment{
		AuthorID:   101,
		Content:    "Moment 2",
		Visibility: 1,
		CreatedAt:  time.Now(),
	}
	mysqlRepo.CreateMoment(context.Background(), m1)
	mysqlRepo.CreateMoment(context.Background(), m2)

	// Pre-populate timeline for user 200
	redisRepo.PublishMomentFeed(context.Background(), 200, 1, time.Now().UnixMilli())
	redisRepo.PublishMomentFeed(context.Background(), 200, 2, time.Now().UnixMilli())

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	moments, err := svc.GetFeed(context.Background(), 200, 0, 10)
	assert.NoError(t, err)
	assert.Len(t, moments, 2)
}

func TestGetFeed_Empty(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	moments, err := svc.GetFeed(context.Background(), 200, 0, 10)
	assert.NoError(t, err)
	assert.Len(t, moments, 0)
}
