package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── Validation / business-error constants ──

const (
	ErrMomentContentEmpty = "moment content cannot be empty"
	ErrMomentNotFound     = "moment not found"
	ErrNotCommentOwner    = "not the owner of this comment"
	ErrInvalidVisibility  = "visibility must be 1 (all), 2 (friends), or 3 (private)"
	ErrAlreadyLiked       = "already liked this moment"
	ErrCommentNotFound    = "comment not found"
)

// MomentService handles moment publishing, like/comment operations, and feed retrieval.
type MomentService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	mqRepo    repository.MQRepo
	logger    *zap.Logger
}

// NewMomentService creates a MomentService with all required dependencies.
func NewMomentService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, mqRepo repository.MQRepo, logger *zap.Logger) *MomentService {
	return &MomentService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		mqRepo:    mqRepo,
		logger:    logger,
	}
}

// PublishMoment creates a moment in MySQL and publishes to MQ for feed fan-out.
// Returns the created moment's ID on success.
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
		return 0, fmt.Errorf("create moment: %w", err)
	}

	// Publish to MQ for fan-out to friends' timelines
	if err := s.mqRepo.PublishMomentPush(ctx, moment); err != nil {
		s.logger.Error("PublishMomentPush failed",
			zap.Int64("momentID", moment.ID),
			zap.Int64("authorID", userID),
			zap.Error(err),
		)
		// Non-critical: moment is persisted in MySQL, feed fan-out will be retried
	}

	return moment.ID, nil
}

// GetMoment returns a single moment by ID.
func (s *MomentService) GetMoment(ctx context.Context, momentID int64) (*model.Moment, error) {
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return nil, fmt.Errorf("get moment: %w", err)
	}
	if moment == nil {
		return nil, fmt.Errorf(ErrMomentNotFound)
	}
	return moment, nil
}

// GetUserMoments returns a paginated list of a user's own moments.
func (s *MomentService) GetUserMoments(ctx context.Context, userID int64, limit, offset int) ([]model.Moment, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	moments, err := s.mysqlRepo.GetMomentsByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get moments by user: %w", err)
	}
	return moments, nil
}

// LikeMoment creates a like record for a moment.
func (s *MomentService) LikeMoment(ctx context.Context, userID int64, momentID int64) error {
	// Verify moment exists
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return fmt.Errorf("check moment: %w", err)
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
		// Duplicate like (unique key constraint on moment_id, user_id)
		return fmt.Errorf("%s: %w", ErrAlreadyLiked, err)
	}
	return nil
}

// UnlikeMoment removes a like record for a moment.
func (s *MomentService) UnlikeMoment(ctx context.Context, userID int64, momentID int64) error {
	if err := s.mysqlRepo.DeleteMomentLike(ctx, momentID, userID); err != nil {
		return fmt.Errorf("unlike moment: %w", err)
	}
	return nil
}

// CommentMoment creates a comment on a moment.
func (s *MomentService) CommentMoment(ctx context.Context, userID int64, momentID int64, content string) (int64, error) {
	if content == "" {
		return 0, fmt.Errorf(ErrMomentContentEmpty)
	}

	// Verify moment exists
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return 0, fmt.Errorf("check moment: %w", err)
	}
	if moment == nil {
		return 0, fmt.Errorf(ErrMomentNotFound)
	}

	// Generate comment ID (moment_comments uses BIGINT PK without AUTO_INCREMENT)
	commentID := time.Now().UnixNano()

	comment := &model.MomentComment{
		ID:        commentID,
		MomentID:  momentID,
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now(),
	}

	if err := s.mysqlRepo.CreateMomentComment(ctx, comment); err != nil {
		return 0, fmt.Errorf("create comment: %w", err)
	}

	return commentID, nil
}

// DeleteComment validates that the user owns the comment before deleting it.
func (s *MomentService) DeleteComment(ctx context.Context, userID int64, commentID int64) error {
	// Fetch comment to validate ownership
	comment, err := s.mysqlRepo.GetMomentCommentByID(ctx, commentID)
	if err != nil {
		return fmt.Errorf("get comment: %w", err)
	}
	if comment == nil {
		return fmt.Errorf(ErrCommentNotFound)
	}
	if comment.UserID != userID {
		return fmt.Errorf(ErrNotCommentOwner)
	}

	if err := s.mysqlRepo.DeleteMomentComment(ctx, commentID); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}

// GetFeed retrieves the user's moment feed from Redis timeline, then fetches
// moment details from MySQL.
func (s *MomentService) GetFeed(ctx context.Context, userID int64, lastSyncTime int64, limit int) ([]model.Moment, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Get moment IDs from Redis timeline ZSet
	momentIDs, err := s.redisRepo.GetMomentFeed(ctx, userID, lastSyncTime, limit)
	if err != nil {
		return nil, fmt.Errorf("get moment feed from redis: %w", err)
	}

	if len(momentIDs) == 0 {
		return []model.Moment{}, nil
	}

	// Fetch moment details from MySQL
	moments := make([]model.Moment, 0, len(momentIDs))
	for _, id := range momentIDs {
		moment, err := s.mysqlRepo.GetMomentByID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to fetch moment for feed",
				zap.Int64("momentID", id),
				zap.Error(err),
			)
			continue // skip moments that fail to load
		}
		if moment != nil {
			moments = append(moments, *moment)
		}
	}

	return moments, nil
}
