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

// ── 模拟实现 ──

// mockMySQLRepo 为动态测试实现 repository.MySQLRepo。
// 仅实现动态相关方法；其他方法会触发 panic。
type mockMySQLRepo struct {
	moments    map[int64]*model.Moment
	likes      map[string]*model.MomentLike // 键："momentID:userID"
	comments   map[int64]*model.MomentComment
	nextID     int64
	likeErr    error // CreateMomentLike 的可注入错误
	commentErr error // CreateMomentComment 的可注入错误
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
		return fmt.Errorf("重复点赞")
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

// 用 panic 桩实现所有其他 MySQLRepo 方法

func (m *mockMySQLRepo) InsertPrivateMessage(_ context.Context, _ *model.PrivateMessage) error {
	panic("未实现")
}
func (m *mockMySQLRepo) InsertGroupMessage(_ context.Context, _ *model.GroupMessage) error {
	panic("未实现")
}
func (m *mockMySQLRepo) InsertMsgRevoked(_ context.Context, _ *model.MsgRevoked) error {
	panic("未实现")
}
func (m *mockMySQLRepo) GetUserByID(_ context.Context, _ int64) (*model.User, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) GetUserByUsername(_ context.Context, _ string) (*model.User, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateUser(_ context.Context, _ *model.User) error {
	panic("未实现")
}
func (m *mockMySQLRepo) UpdateUser(_ context.Context, _ *model.User) error {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateFriendRequest(_ context.Context, _ *model.FriendRequest) error {
	panic("未实现")
}
func (m *mockMySQLRepo) UpdateFriendRequest(_ context.Context, _ *model.FriendRequest) error {
	panic("未实现")
}
func (m *mockMySQLRepo) GetFriendRequestByID(_ context.Context, _ int64) (*model.FriendRequest, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) GetFriendRequestsByUser(_ context.Context, _ int64) ([]model.FriendRequest, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateFriendship(_ context.Context, _ *model.Friendship) error {
	panic("未实现")
}
func (m *mockMySQLRepo) DeleteFriendship(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *mockMySQLRepo) GetFriendList(_ context.Context, _ int64) ([]model.Friendship, error) {
	return nil, nil
}
func (m *mockMySQLRepo) IsFriend(_ context.Context, _ int64, _ int64) (bool, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateBlacklist(_ context.Context, _ *model.Blacklist) error {
	panic("未实现")
}
func (m *mockMySQLRepo) DeleteBlacklist(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *mockMySQLRepo) IsBlocked(_ context.Context, _ int64, _ int64) (bool, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateGroup(_ context.Context, _ *model.Group) (int64, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) UpdateGroup(_ context.Context, _ *model.Group) error {
	panic("未实现")
}
func (m *mockMySQLRepo) GetGroupByID(_ context.Context, _ int64) (*model.Group, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) AddGroupMember(_ context.Context, _ *model.GroupMember) error {
	panic("未实现")
}
func (m *mockMySQLRepo) RemoveGroupMember(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *mockMySQLRepo) GetGroupMembers(_ context.Context, _ int64) ([]model.GroupMember, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) UpdateGroupMemberRole(_ context.Context, _ int, _ int, _ int) error {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateAISummary(_ context.Context, _ *model.AISummary) error {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateAIProfileItem(_ context.Context, _ *model.AIProfileItem) error {
	panic("未实现")
}
func (m *mockMySQLRepo) GetAIProfileByUser(_ context.Context, _ int64) ([]model.AIProfileItem, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) GetUserSettings(_ context.Context, _ int64) (*model.UserSettings, error) {
	panic("未实现")
}
func (m *mockMySQLRepo) CreateOrUpdateUserSettings(_ context.Context, _ *model.UserSettings) error {
	panic("未实现")
}
func (m *mockMySQLRepo) SearchPrivateMessages(_ context.Context, _ int64, _ string, _ int, _ int) ([]model.PrivateMessage, error) {
	panic("未实现")
}

// momentMockRedisRepo 为动态测试实现 repository.RedisRepo。
// 仅实现动态信息流方法；其他方法会触发 panic。
type momentMockRedisRepo struct {
	timelines map[int64][]int64 // 用户ID -> 动态ID列表
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

// 用 panic 桩实现所有其他 RedisRepo 方法

func (m *momentMockRedisRepo) WriteInbox(_ context.Context, _ int64, _ *model.InboxMessage) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) WriteOutbox(_ context.Context, _ int64, _ *model.InboxMessage) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) ReadInbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) ReadOutbox(_ context.Context, _ int64, _ int64, _ int) ([]model.InboxMessage, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) UpdateConvList(_ context.Context, _ int64, _ string, _ string, _ int64) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) GetConvList(_ context.Context, _ int64) ([]model.ConvSummary, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) IncrementUnread(_ context.Context, _ int64, _ string) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) ClearUnread(_ context.Context, _ int64, _ string) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) GetUnreadMap(_ context.Context, _ int64) (map[string]int64, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) SetGroupReadPos(_ context.Context, _ int64, _ string, _ int64) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) GetGroupReadPos(_ context.Context, _ int64, _ string) (int64, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) GetGroupMemberships(_ context.Context, _ int64) ([]int64, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) GetGroupMembers(_ context.Context, _ int64) ([]int64, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) CheckDuplicate(_ context.Context, _ int64, _ string) (bool, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) TrimInbox(_ context.Context, _ int64, _ int) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) TrimOutbox(_ context.Context, _ int64, _ int) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) TrimInboxByTime(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) TrimOutboxByTime(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) TrimConvListByTime(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) TrimTimelineByTime(_ context.Context, _ int64, _ int64) error {
	panic("未实现")
}
func (m *momentMockRedisRepo) ExecPrivateMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.PrivateMsgCheckResult, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) ExecGroupMsgCheck(_ context.Context, _ int64, _ int64, _ string) (*redis.GroupMsgCheckResult, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) ExecInboxMarkRead(_ context.Context, _ int64, _ string) (int64, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) ExecRevokeMsg(_ context.Context, _ int64, _ string, _ int64, _ string, _ int64) (bool, error) {
	panic("未实现")
}
func (m *momentMockRedisRepo) AddGroupMemberRedis(_ context.Context, _ int64, _ int64) error   { return nil }
func (m *momentMockRedisRepo) RemoveGroupMemberRedis(_ context.Context, _ int64, _ int64) error { return nil }
func (m *momentMockRedisRepo) SetWorkingMemory(_ context.Context, _ int64, _ string, _ string, _ int64) error { return nil }
func (m *momentMockRedisRepo) GetWorkingMemory(_ context.Context, _ int64, _ string) (string, error)            { return "", nil }
func (m *momentMockRedisRepo) GetAllWorkingMemory(_ context.Context, _ int64) (map[string]string, error)        { return nil, nil }

// momentMockMQRepo 为动态测试实现 repository.MQRepo。
type momentMockMQRepo struct {
	publishedMoments []*model.Moment
}

func newMomentMockMQRepo() *momentMockMQRepo {
	return &momentMockMQRepo{
		publishedMoments: make([]*model.Moment, 0),
	}
}

func (m *momentMockMQRepo) PublishPrivateMsg(_ context.Context, _ *model.PrivateMessage) error {
	panic("未实现")
}
func (m *momentMockMQRepo) PublishGroupMsg(_ context.Context, _ *model.GroupMessage) error {
	panic("未实现")
}
func (m *momentMockMQRepo) PublishMomentPush(_ context.Context, moment *model.Moment) error {
	m.publishedMoments = append(m.publishedMoments, moment)
	return nil
}

// ── 测试 ──

func TestPublishMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	momentID, err := svc.PublishMoment(context.Background(), 100, "Hello world!", nil, 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), momentID)

	// 验证动态已存储到 MySQL
	moment, ok := mysqlRepo.moments[momentID]
	assert.True(t, ok)
	assert.Equal(t, int64(100), moment.AuthorID)
	assert.Equal(t, "Hello world!", moment.Content)
	assert.Equal(t, 1, moment.Visibility)

	// 验证动态已发布到 MQ
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

	// 预填充一条动态
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

	// 预填充一条动态
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	err := svc.LikeMoment(context.Background(), 200, 1)
	assert.NoError(t, err)

	// 验证点赞已存储
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

	// 预填充一条动态
	mysqlRepo.CreateMoment(context.Background(), &model.Moment{
		AuthorID:   100,
		Content:    "Test moment",
		Visibility: 1,
		CreatedAt:  time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	// 第一次点赞应该成功
	err := svc.LikeMoment(context.Background(), 200, 1)
	assert.NoError(t, err)

	// 第二次点赞应该因重复而失败
	err = svc.LikeMoment(context.Background(), 200, 1)
	assert.Error(t, err)
	// 错误消息应包含 ErrAlreadyLiked 前缀
	assert.Contains(t, err.Error(), ErrAlreadyLiked)
}

func TestUnlikeMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// 预填充一条动态和一个点赞
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

	// 验证点赞已移除
	key := fmt.Sprintf("%d:%d", 1, 200)
	_, ok := mysqlRepo.likes[key]
	assert.False(t, ok)
}

func TestCommentMoment_Success(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// 预填充一条动态
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

	// 验证评论已存储
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

	// 预填充一条动态
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

	// 预填充一条动态和用户 200 的评论
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
		UserID:    200, // 评论属于用户 200
		Content:   "Great post!",
		CreatedAt: time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	// 用户 200（所有者）应该能够删除自己的评论
	err := svc.DeleteComment(context.Background(), 200, commentID)
	assert.NoError(t, err)

	// 验证评论已移除
	_, ok := mysqlRepo.comments[commentID]
	assert.False(t, ok)
}

func TestDeleteComment_NotOwner(t *testing.T) {
	mysqlRepo := newMockMySQLRepo()
	redisRepo := newMomentMockRedisRepo()
	mqRepo := newMomentMockMQRepo()
	logger, _ := zap.NewDevelopment()

	// 预填充一条动态和用户 200 的评论
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
		UserID:    200, // 评论属于用户 200
		Content:   "Great post!",
		CreatedAt: time.Now(),
	})

	svc := NewMomentService(mysqlRepo, redisRepo, mqRepo, logger)

	// 用户 300（非所有者）不应能够删除用户 200 的评论
	err := svc.DeleteComment(context.Background(), 300, commentID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotCommentOwner, err.Error())

	// 验证评论未被移除
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

	// 预填充动态
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

	// 为用户 200 预填充时间线
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
