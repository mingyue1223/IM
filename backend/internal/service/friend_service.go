package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── 好友服务错误常量 ──

const (
	ErrSelfRequest      = "不能给自己发送好友请求"
	ErrAlreadyFriends   = "已经是该用户的好友"
	ErrFriendBlocked    = "你已拉黑该用户或已被该用户拉黑"
	ErrDuplicateRequest = "已存在待处理的好友请求"
	ErrRequestNotFound  = "好友请求未找到"
	ErrNotRequestTarget = "你不是该好友请求的接收者"
	ErrAlreadyBlocked   = "你已经拉黑了该用户"
)

// FriendService 处理好友相关的业务逻辑：发送/接受/拒绝好友请求、管理好友关系以及拉黑/取消拉黑用户。
type FriendService struct {
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	logger    *zap.Logger
}

// NewFriendService 创建一个包含所有必要依赖的 FriendService。
func NewFriendService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, logger *zap.Logger) *FriendService {
	return &FriendService{
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}
}

// SendFriendRequest 创建好友请求，并进行以下验证：
// - 不能给自己发请求
// - 不能已经是好友
// - 未被拉黑（任一方向）
// - 不存在重复的待处理请求
func (s *FriendService) SendFriendRequest(ctx context.Context, fromUserID, toUserID int64, message string) (*model.FriendRequest, error) {
	// 1. 不能给自己发请求
	if fromUserID == toUserID {
		return nil, fmt.Errorf(ErrSelfRequest)
	}

	// 2. 不能已经是好友
	isFriend, err := s.mysqlRepo.IsFriend(ctx, fromUserID, toUserID)
	if err != nil {
		return nil, fmt.Errorf("检查好友关系: %w", err)
	}
	if isFriend {
		return nil, fmt.Errorf(ErrAlreadyFriends)
	}

	// 3. 未被拉黑（检查双向）
	blockedBySender, err := s.mysqlRepo.IsBlocked(ctx, fromUserID, toUserID)
	if err != nil {
		return nil, fmt.Errorf("检查发送者是否被拉黑: %w", err)
	}
	if blockedBySender {
		return nil, fmt.Errorf(ErrFriendBlocked)
	}
	blockedByTarget, err := s.mysqlRepo.IsBlocked(ctx, toUserID, fromUserID)
	if err != nil {
		return nil, fmt.Errorf("检查接收者是否被拉黑: %w", err)
	}
	if blockedByTarget {
		return nil, fmt.Errorf(ErrFriendBlocked)
	}

	// 4. 不存在重复的待处理请求
	existingRequests, err := s.mysqlRepo.GetFriendRequestsByUser(ctx, fromUserID)
	if err != nil {
		return nil, fmt.Errorf("检查已有请求: %w", err)
	}
	for _, req := range existingRequests {
		if req.Status == 0 && // 待处理
			((req.FromUserID == fromUserID && req.ToUserID == toUserID) ||
				(req.FromUserID == toUserID && req.ToUserID == fromUserID)) {
			return nil, fmt.Errorf(ErrDuplicateRequest)
		}
	}

	// 5. 创建好友请求；同方向的历史已处理记录由仓储层原子重置为待处理。
	req := &model.FriendRequest{
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Message:    message,
		Status:     0, // 待处理
	}
	if err := s.mysqlRepo.CreateFriendRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("创建好友请求: %w", err)
	}

	s.logger.Debug("好友请求已发送",
		zap.Int64("fromUserID", fromUserID),
		zap.Int64("toUserID", toUserID),
		zap.Int64("requestID", req.ID),
	)

	return req, nil
}

// AcceptFriendRequest 接受好友请求。验证：
// - 请求存在
// - 调用者是请求的接收者（toUserID）
// 然后更新请求状态为已接受（1）并创建双向好友关系。
func (s *FriendService) AcceptFriendRequest(ctx context.Context, userID, requestID int64) (*model.Friendship, error) {
	// 1. 获取请求
	req, err := s.mysqlRepo.GetFriendRequestByID(ctx, requestID)
	if err != nil {
		return nil, fmt.Errorf("获取好友请求: %w", err)
	}
	if req == nil {
		return nil, fmt.Errorf(ErrRequestNotFound)
	}

	// 2. 验证调用者是请求的接收者
	if req.ToUserID != userID {
		return nil, fmt.Errorf(ErrNotRequestTarget)
	}

	// 3. 更新请求状态为已接受
	req.Status = 1
	if err := s.mysqlRepo.UpdateFriendRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("接受好友请求: %w", err)
	}

	// 4. 创建双向好友关系
	fs := &model.Friendship{
		UserID:   req.FromUserID,
		FriendID: req.ToUserID,
	}
	if err := s.mysqlRepo.CreateFriendship(ctx, fs); err != nil {
		return nil, fmt.Errorf("创建好友关系: %w", err)
	}

	// 5. 写入 Redis 好友缓存（Lua 消息校验依赖此 key）
	if err := s.redisRepo.SetFriendCache(ctx, req.FromUserID, req.ToUserID); err != nil {
		s.logger.Warn("设置好友缓存失败", zap.Error(err))
		// 非致命：MySQL 已写入，缓存可在后续 IsFriend 查询时按需回填
	}

	s.logger.Debug("好友请求已接受",
		zap.Int64("userID", userID),
		zap.Int64("requestID", requestID),
	)

	return fs, nil
}

// RejectFriendRequest 拒绝好友请求。验证：
// - 请求存在
// - 调用者是请求的接收者（toUserID）
// 然后更新请求状态为已拒绝（2）。
func (s *FriendService) RejectFriendRequest(ctx context.Context, userID, requestID int64) error {
	// 1. 获取请求
	req, err := s.mysqlRepo.GetFriendRequestByID(ctx, requestID)
	if err != nil {
		return fmt.Errorf("获取好友请求: %w", err)
	}
	if req == nil {
		return fmt.Errorf(ErrRequestNotFound)
	}

	// 2. 验证调用者是请求的接收者
	if req.ToUserID != userID {
		return fmt.Errorf(ErrNotRequestTarget)
	}

	// 3. 更新请求状态为已拒绝
	req.Status = 2
	if err := s.mysqlRepo.UpdateFriendRequest(ctx, req); err != nil {
		return fmt.Errorf("拒绝好友请求: %w", err)
	}

	s.logger.Debug("好友请求已拒绝",
		zap.Int64("userID", userID),
		zap.Int64("requestID", requestID),
	)

	return nil
}

// GetFriendRequests 返回涉及指定用户的待处理好友请求。
func (s *FriendService) GetFriendRequests(ctx context.Context, userID int64) ([]model.FriendRequest, error) {
	requests, err := s.mysqlRepo.GetFriendRequestsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取好友请求: %w", err)
	}
	return requests, nil
}

// FriendListItem 使用用户个人资料数据丰富 Friendship 信息。
type FriendListItem struct {
	model.Friendship
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// GetFriendList 返回用户的好友列表，包含昵称和头像。
func (s *FriendService) GetFriendList(ctx context.Context, userID int64) ([]FriendListItem, error) {
	friendships, err := s.mysqlRepo.GetFriendList(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取好友列表: %w", err)
	}

	items := make([]FriendListItem, 0, len(friendships))
	for _, fs := range friendships {
		user, err := s.mysqlRepo.GetUserByID(ctx, fs.FriendID)
		if err != nil {
			s.logger.Warn("获取好友个人资料失败",
				zap.Int64("friendID", fs.FriendID),
				zap.Error(err),
			)
			// 仍然包含好友关系，只是没有个人资料信息
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

// DeleteFriend 删除两个用户之间的双向好友关系。
func (s *FriendService) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	if err := s.mysqlRepo.DeleteFriendship(ctx, userID, friendID); err != nil {
		return fmt.Errorf("删除好友关系: %w", err)
	}

	s.logger.Debug("好友已删除",
		zap.Int64("userID", userID),
		zap.Int64("friendID", friendID),
	)

	return nil
}

// BlockUser 将用户添加到调用者的黑名单中。
func (s *FriendService) BlockUser(ctx context.Context, userID, blockedID int64) error {
	// 检查是否已经拉黑
	isBlocked, err := s.mysqlRepo.IsBlocked(ctx, userID, blockedID)
	if err != nil {
		return fmt.Errorf("检查是否已拉黑: %w", err)
	}
	if isBlocked {
		return fmt.Errorf(ErrAlreadyBlocked)
	}

	bl := &model.Blacklist{
		UserID:    userID,
		BlockedID: blockedID,
	}
	if err := s.mysqlRepo.CreateBlacklist(ctx, bl); err != nil {
		return fmt.Errorf("创建黑名单记录: %w", err)
	}

	s.logger.Debug("用户已拉黑",
		zap.Int64("userID", userID),
		zap.Int64("blockedID", blockedID),
	)

	return nil
}

// UnblockUser 将用户从调用者的黑名单中移除。
func (s *FriendService) UnblockUser(ctx context.Context, userID, blockedID int64) error {
	if err := s.mysqlRepo.DeleteBlacklist(ctx, userID, blockedID); err != nil {
		return fmt.Errorf("删除黑名单记录: %w", err)
	}

	s.logger.Debug("用户已取消拉黑",
		zap.Int64("userID", userID),
		zap.Int64("blockedID", blockedID),
	)

	return nil
}

// IsBlocked 检查调用者是否已拉黑指定用户。
func (s *FriendService) IsBlocked(ctx context.Context, userID, blockedID int64) (bool, error) {
	isBlocked, err := s.mysqlRepo.IsBlocked(ctx, userID, blockedID)
	if err != nil {
		return false, fmt.Errorf("检查是否已拉黑: %w", err)
	}
	return isBlocked, nil
}

// HandleFriendApply 通过 WebSocket 处理好友申请。
// 此方法作为 WS MessageDispatcher 的占位符保留——HTTP 处理器是好友操作的主要接口。
func (s *FriendService) HandleFriendApply(userID int64, data []byte) {
	// TODO: 如有需要，实现基于 WebSocket 的好友申请；HTTP 处理器是首选方式
}
