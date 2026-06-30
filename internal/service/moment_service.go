package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── 验证/业务错误常量 ──

const (
	ErrMomentContentEmpty = "动态内容不能为空"
	ErrMomentNotFound     = "动态未找到"
	ErrNotCommentOwner    = "不是该评论的所有者"
	ErrInvalidVisibility  = "可见性必须为 1（全部）、2（好友）或 3（私密）"
	ErrAlreadyLiked       = "已经赞过该动态"
	ErrCommentNotFound    = "评论未找到"
)

// MomentService 处理动态发布、点赞/评论操作以及动态流获取。
type MomentService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	mqRepo    repository.MQRepo
	logger    *zap.Logger
}

// NewMomentService 使用所有必需的依赖项创建一个 MomentService。
func NewMomentService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, mqRepo repository.MQRepo, logger *zap.Logger) *MomentService {
	return &MomentService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		mqRepo:    mqRepo,
		logger:    logger,
	}
}

// PublishMoment 在 MySQL 中创建动态并发布到 MQ 以进行动态流扇出。
// 成功时返回所创建动态的 ID。
func (s *MomentService) PublishMoment(ctx context.Context, userID int64, content string, mediaUrls *string, visibility int) (int64, error) {
	if content == "" {
		return 0, fmt.Errorf(ErrMomentContentEmpty)
	}
	if visibility < 1 || visibility > 3 {
		return 0, fmt.Errorf(ErrInvalidVisibility)
	}

	now := time.Now()
	moment := &model.Moment{
		AuthorID:   userID,
		Content:    content,
		MediaUrls:  mediaUrls,
		Visibility: visibility,
		CreatedAt:  now,
	}

	if err := s.mysqlRepo.CreateMoment(ctx, moment); err != nil {
		return 0, fmt.Errorf("创建动态: %w", err)
	}

	// 发布到 MQ 以扇出到好友的时间线
	if err := s.mqRepo.PublishMomentPush(ctx, moment); err != nil {
		s.logger.Error("PublishMomentPush 失败",
			zap.Int64("momentID", moment.ID),
			zap.Int64("authorID", userID),
			zap.Error(err),
		)
		// 非关键：动态已持久化到 MySQL，动态流扇出将会重试
	}

	return moment.ID, nil
}

// GetMoment 根据 ID 返回单条动态。
func (s *MomentService) GetMoment(ctx context.Context, momentID int64) (*model.Moment, error) {
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return nil, fmt.Errorf("获取动态: %w", err)
	}
	if moment == nil {
		return nil, fmt.Errorf(ErrMomentNotFound)
	}
	return moment, nil
}

// GetUserMoments 返回用户自己的分页动态列表。
func (s *MomentService) GetUserMoments(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	moments, err := s.mysqlRepo.GetMomentsByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("按用户获取动态: %w", err)
	}
	return moments, nil
}

// LikeMoment 为动态创建一条点赞记录。
func (s *MomentService) LikeMoment(ctx context.Context, userID int64, momentID int64) error {
	// 验证动态是否存在
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return fmt.Errorf("检查动态: %w", err)
	}
	if moment == nil {
		return fmt.Errorf(ErrMomentNotFound)
	}

	like := &model.MomentLike{
		MomentID:  momentID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}
	if err := s.mysqlRepo.CreateMomentLike(ctx, like); err != nil {
		// 重复点赞（moment_id、user_id 上的唯一键约束）
		return fmt.Errorf("%s: %w", ErrAlreadyLiked, err)
	}
	return nil
}

// UnlikeMoment 移除动态的点赞记录。
func (s *MomentService) UnlikeMoment(ctx context.Context, userID int64, momentID int64) error {
	if err := s.mysqlRepo.DeleteMomentLike(ctx, momentID, userID); err != nil {
		return fmt.Errorf("取消点赞: %w", err)
	}
	return nil
}

// CommentMoment 在动态上创建一条评论。
func (s *MomentService) CommentMoment(ctx context.Context, userID int64, momentID int64, content string) (int64, error) {
	if content == "" {
		return 0, fmt.Errorf(ErrMomentContentEmpty)
	}

	// 验证动态是否存在
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return 0, fmt.Errorf("检查动态: %w", err)
	}
	if moment == nil {
		return 0, fmt.Errorf(ErrMomentNotFound)
	}

	// 生成评论 ID（moment_comments 使用 BIGINT 主键，无 AUTO_INCREMENT）
	commentID := time.Now().UnixNano()

	comment := &model.MomentComment{
		ID:        commentID,
		MomentID:  momentID,
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now(),
	}

	if err := s.mysqlRepo.CreateMomentComment(ctx, comment); err != nil {
		return 0, fmt.Errorf("创建评论: %w", err)
	}

	return commentID, nil
}

// DeleteComment 在删除评论前验证用户是否拥有该评论。
func (s *MomentService) DeleteComment(ctx context.Context, userID int64, commentID int64) error {
	// 获取评论以验证所有权
	comment, err := s.mysqlRepo.GetMomentCommentByID(ctx, commentID)
	if err != nil {
		return fmt.Errorf("获取评论: %w", err)
	}
	if comment == nil {
		return fmt.Errorf(ErrCommentNotFound)
	}
	if comment.UserID != userID {
		return fmt.Errorf(ErrNotCommentOwner)
	}

	if err := s.mysqlRepo.DeleteMomentComment(ctx, commentID); err != nil {
		return fmt.Errorf("删除评论: %w", err)
	}
	return nil
}

// GetFeed 从 Redis 时间线中检索用户的动态流，然后从 MySQL 中获取动态详情。
func (s *MomentService) GetFeed(ctx context.Context, userID int64, lastSyncTime int64, limit int) ([]model.Moment, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 从 Redis 时间线 ZSet 中获取动态 ID
	momentIDs, err := s.redisRepo.GetMomentFeed(ctx, userID, lastSyncTime, limit)
	if err != nil {
		return nil, fmt.Errorf("从 Redis 获取动态流: %w", err)
	}

	if len(momentIDs) == 0 {
		return []model.Moment{}, nil
	}

	// 从 MySQL 中获取动态详情
	moments := make([]model.Moment, 0, len(momentIDs))
	for _, id := range momentIDs {
		moment, err := s.mysqlRepo.GetMomentByID(ctx, id)
		if err != nil {
			s.logger.Warn("获取动态流中的动态失败",
				zap.Int64("momentID", id),
				zap.Error(err),
			)
			continue // 跳过加载失败的动态
		}
		if moment != nil {
			moments = append(moments, *moment)
		}
	}

	return moments, nil
}
