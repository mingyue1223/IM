package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── Friend service error constants ──

const (
	ErrSelfRequest      = "cannot send friend request to yourself"
	ErrAlreadyFriends   = "already friends with this user"
	ErrFriendBlocked    = "you have blocked this user or they have blocked you"
	ErrDuplicateRequest = "a pending friend request already exists"
	ErrRequestNotFound  = "friend request not found"
	ErrNotRequestTarget = "you are not the target of this friend request"
	ErrAlreadyBlocked   = "you have already blocked this user"
)

// FriendService handles friend-related business logic: sending/accepting/rejecting
// friend requests, managing friendships, and blocking/unblocking users.
type FriendService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	logger    *zap.Logger
}

// NewFriendService creates a FriendService with all required dependencies.
func NewFriendService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, logger *zap.Logger) *FriendService {
	return &FriendService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}
}

// SendFriendRequest creates a friend request after validating:
// - not self-request
// - not already friends
// - not blocked (either direction)
// - no duplicate pending request
func (s *FriendService) SendFriendRequest(ctx context.Context, fromUserID, toUserID int64, message string) (*model.FriendRequest, error) {
	// 1. Not self-request
	if fromUserID == toUserID {
		return nil, fmt.Errorf(ErrSelfRequest)
	}

	// 2. Not already friends
	isFriend, err := s.mysqlRepo.IsFriend(ctx, fromUserID, toUserID)
	if err != nil {
		return nil, fmt.Errorf("check friendship: %w", err)
	}
	if isFriend {
		return nil, fmt.Errorf(ErrAlreadyFriends)
	}

	// 3. Not blocked (check both directions)
	blockedBySender, err := s.mysqlRepo.IsBlocked(ctx, fromUserID, toUserID)
	if err != nil {
		return nil, fmt.Errorf("check blocked by sender: %w", err)
	}
	if blockedBySender {
		return nil, fmt.Errorf(ErrFriendBlocked)
	}
	blockedByTarget, err := s.mysqlRepo.IsBlocked(ctx, toUserID, fromUserID)
	if err != nil {
		return nil, fmt.Errorf("check blocked by target: %w", err)
	}
	if blockedByTarget {
		return nil, fmt.Errorf(ErrFriendBlocked)
	}

	// 4. No duplicate pending request
	existingRequests, err := s.mysqlRepo.GetFriendRequestsByUser(ctx, fromUserID)
	if err != nil {
		return nil, fmt.Errorf("check existing requests: %w", err)
	}
	for _, req := range existingRequests {
		if req.Status == 0 && // pending
			((req.FromUserID == fromUserID && req.ToUserID == toUserID) ||
				(req.FromUserID == toUserID && req.ToUserID == fromUserID)) {
			return nil, fmt.Errorf(ErrDuplicateRequest)
		}
	}

	// 5. Create the friend request
	req := &model.FriendRequest{
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Message:    message,
		Status:     0, // pending
	}
	if err := s.mysqlRepo.CreateFriendRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("create friend request: %w", err)
	}

	s.logger.Debug("friend request sent",
		zap.Int64("fromUserID", fromUserID),
		zap.Int64("toUserID", toUserID),
		zap.Int64("requestID", req.ID),
	)

	return req, nil
}

// AcceptFriendRequest accepts a friend request. It validates:
// - the request exists
// - the calling user is the target (toUserID) of the request
// Then it updates the request status to accepted (1) and creates a bidirectional Friendship.
func (s *FriendService) AcceptFriendRequest(ctx context.Context, userID, requestID int64) (*model.Friendship, error) {
	// 1. Get the request
	req, err := s.mysqlRepo.GetFriendRequestByID(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("get friend request: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf(ErrRequestNotFound)
	}

	// 2. Validate the calling user is the target
	if req.ToUserID != userID {
		return nil, fmt.Errorf(ErrNotRequestTarget)
	}

	// 3. Update request status to accepted
	req.Status = 1
	if err := s.mysqlRepo.UpdateFriendRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("accept friend request: %w", err)
	}

	// 4. Create bidirectional Friendship
	fs := &model.Friendship{
		UserID:   req.FromUserID,
		FriendID: req.ToUserID,
	}
	if err := s.mysqlRepo.CreateFriendship(ctx, fs); err != nil {
		return nil, fmt.Errorf("create friendship: %w", err)
	}

	s.logger.Debug("friend request accepted",
		zap.Int64("userID", userID),
		zap.Int64("requestID", requestID),
	)

	return fs, nil
}

// RejectFriendRequest rejects a friend request. It validates:
// - the request exists
// - the calling user is the target (toUserID) of the request
// Then it updates the request status to rejected (2).
func (s *FriendService) RejectFriendRequest(ctx context.Context, userID, requestID int64) error {
	// 1. Get the request
	req, err := s.mysqlRepo.GetFriendRequestByID(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get friend request: %w", err)
	}
	if req == nil {
		return fmt.Errorf(ErrRequestNotFound)
	}

	// 2. Validate the calling user is the target
	if req.ToUserID != userID {
		return fmt.Errorf(ErrNotRequestTarget)
	}

	// 3. Update request status to rejected
	req.Status = 2
	if err := s.mysqlRepo.UpdateFriendRequest(ctx, req); err != nil {
		return fmt.Errorf("reject friend request: %w", err)
	}

	s.logger.Debug("friend request rejected",
		zap.Int64("userID", userID),
		zap.Int64("requestID", requestID),
	)

	return nil
}

// GetFriendRequests returns all friend requests involving the given user.
func (s *FriendService) GetFriendRequests(ctx context.Context, userID int64) ([]model.FriendRequest, error) {
	requests, err := s.mysqlRepo.GetFriendRequestsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get friend requests: %w", err)
	}
	return requests, nil
}

// FriendListItem enriches a Friendship with user profile data.
type FriendListItem struct {
	model.Friendship
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// GetFriendList returns the user's friends enriched with nickname and avatar.
func (s *FriendService) GetFriendList(ctx context.Context, userID int64) ([]FriendListItem, error) {
	friendships, err := s.mysqlRepo.GetFriendList(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get friend list: %w", err)
	}

	items := make([]FriendListItem, 0, len(friendships))
	for _, fs := range friendships {
		user, err := s.mysqlRepo.GetUserByID(ctx, fs.FriendID)
		if err != nil {
			s.logger.Warn("failed to get friend profile",
				zap.Int64("friendID", fs.FriendID),
				zap.Error(err),
			)
			// Still include the friendship, just without profile info
			items = append(items, FriendListItem{Friendship: fs})
			continue
		}
		if user == nil {
			items = append(items, FriendListItem{Friendship: fs})
			continue
		}
		items = append(items, FriendListItem{
			Friendship: fs,
			Nickname:   user.Nickname,
			AvatarURL:  user.AvatarURL,
		})
	}

	return items, nil
}

// DeleteFriend removes a bidirectional friendship between the two users.
func (s *FriendService) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	if err := s.mysqlRepo.DeleteFriendship(ctx, userID, friendID); err != nil {
		return fmt.Errorf("delete friendship: %w", err)
	}

	s.logger.Debug("friend deleted",
		zap.Int64("userID", userID),
		zap.Int64("friendID", friendID),
	)

	return nil
}

// BlockUser adds a user to the calling user's blacklist.
func (s *FriendService) BlockUser(ctx context.Context, userID, blockedID int64) error {
	// Check if already blocked
	isBlocked, err := s.mysqlRepo.IsBlocked(ctx, userID, blockedID)
	if err != nil {
		return fmt.Errorf("check already blocked: %w", err)
	}
	if isBlocked {
		return fmt.Errorf(ErrAlreadyBlocked)
	}

	bl := &model.Blacklist{
		UserID:    userID,
		BlockedID: blockedID,
	}
	if err := s.mysqlRepo.CreateBlacklist(ctx, bl); err != nil {
		return fmt.Errorf("create blacklist: %w", err)
	}

	s.logger.Debug("user blocked",
		zap.Int64("userID", userID),
		zap.Int64("blockedID", blockedID),
	)

	return nil
}

// UnblockUser removes a user from the calling user's blacklist.
func (s *FriendService) UnblockUser(ctx context.Context, userID, blockedID int64) error {
	if err := s.mysqlRepo.DeleteBlacklist(ctx, userID, blockedID); err != nil {
		return fmt.Errorf("delete blacklist: %w", err)
	}

	s.logger.Debug("user unblocked",
		zap.Int64("userID", userID),
		zap.Int64("blockedID", blockedID),
	)

	return nil
}

// IsBlocked checks whether the calling user has blocked the given user.
func (s *FriendService) IsBlocked(ctx context.Context, userID, blockedID int64) (bool, error) {
	isBlocked, err := s.mysqlRepo.IsBlocked(ctx, userID, blockedID)
	if err != nil {
		return false, fmt.Errorf("check is_blocked: %w", err)
	}
	return isBlocked, nil
}

// HandleFriendApply processes a friend application via WebSocket.
// This is kept as a placeholder for the WS MessageDispatcher — the HTTP
// handlers are the primary interface for friend operations.
func (s *FriendService) HandleFriendApply(userID int64, data []byte) {
	// TODO: implement WS-based friend apply if needed; HTTP handlers are preferred
}
