package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	ErrCommentNotFound    = "评论未找到"
)

// MomentService 处理动态发布、点赞/评论操作以及动态流获取。
type MomentService struct {
	mysqlRepo    repository.MySQLRepo
	redisRepo    repository.RedisRepo
	mqRepo       repository.MQRepo
	logger       *zap.Logger
	likeCacheTTL time.Duration // 点赞 Redis 缓存（集合/计数/标记）的 TTL
}

// NewMomentService 使用所有必需的依赖项创建一个 MomentService。
// likeCacheTTL 为点赞缓存的过期时间（来自 config.MomentConfig.LikeCacheTTLHours）。
func NewMomentService(mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, mqRepo repository.MQRepo, logger *zap.Logger, likeCacheTTL time.Duration) *MomentService {
	if likeCacheTTL <= 0 {
		likeCacheTTL = 7 * 24 * time.Hour
	}
	return &MomentService{
		mysqlRepo:    mysqlRepo,
		redisRepo:    redisRepo,
		mqRepo:       mqRepo,
		logger:       logger,
		likeCacheTTL: likeCacheTTL,
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

// GetMoment 根据 ID 返回单条动态（含点赞数与"我是否已赞"）。
func (s *MomentService) GetMoment(ctx context.Context, viewerID int64, momentID int64) (*model.Moment, error) {
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return nil, fmt.Errorf("获取动态: %w", err)
	}
	if moment == nil {
		return nil, fmt.Errorf(ErrMomentNotFound)
	}
	batch := []model.Moment{*moment}
	s.enrichLikes(ctx, viewerID, batch)
	return &batch[0], nil
}

// GetUserMoments 返回用户自己的分页动态列表（含点赞数与"我是否已赞"）。
func (s *MomentService) GetUserMoments(ctx context.Context, viewerID int64, userID int64, limit, offset int) ([]model.Moment, error) {
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
	s.enrichLikes(ctx, viewerID, moments)
	return moments, nil
}

// LikeMoment 为动态点赞。写路径走 Redis（SADD 判重 + INCR 计数，Lua 原子），
// 点赞状态变化时发 like_persist 事件由消费者异步落库。幂等：重复点赞返回当前点赞数、不报错。
// 返回该动态最新点赞数。
func (s *MomentService) LikeMoment(ctx context.Context, userID int64, momentID int64) (int64, error) {
	// 验证动态是否存在
	moment, err := s.mysqlRepo.GetMomentByID(ctx, momentID)
	if err != nil {
		return 0, fmt.Errorf("检查动态: %w", err)
	}
	if moment == nil {
		return 0, fmt.Errorf(ErrMomentNotFound)
	}

	// 确保点赞缓存已从 MySQL 预热（避免冷 Set 把老用户的赞误判为新赞）。
	if err := s.ensureLikesLoaded(ctx, momentID); err != nil {
		return 0, fmt.Errorf("预热点赞缓存: %w", err)
	}

	changed, count, err := s.redisRepo.LikeMomentAtomic(ctx, momentID, userID)
	if err != nil {
		return 0, fmt.Errorf("点赞: %w", err)
	}
	if changed {
		s.publishLikeEvent(ctx, momentID, userID, model.LikeActionLike)
	}
	return count, nil
}

// UnlikeMoment 取消点赞。SREM + DECR（Lua 原子，计数不低于 0），幂等返回当前点赞数。
func (s *MomentService) UnlikeMoment(ctx context.Context, userID int64, momentID int64) (int64, error) {
	if err := s.ensureLikesLoaded(ctx, momentID); err != nil {
		return 0, fmt.Errorf("预热点赞缓存: %w", err)
	}
	changed, count, err := s.redisRepo.UnlikeMomentAtomic(ctx, momentID, userID)
	if err != nil {
		return 0, fmt.Errorf("取消点赞: %w", err)
	}
	if changed {
		s.publishLikeEvent(ctx, momentID, userID, model.LikeActionUnlike)
	}
	return count, nil
}

// ensureLikesLoaded 触发点赞缓存的按需预热（loader 从 MySQL 拉取该动态全部点赞用户）。
func (s *MomentService) ensureLikesLoaded(ctx context.Context, momentID int64) error {
	return s.redisRepo.EnsureMomentLikesLoaded(ctx, momentID, func(c context.Context) ([]int64, error) {
		return s.mysqlRepo.GetMomentLikers(c, momentID)
	}, s.likeCacheTTL)
}

// publishLikeEvent 发布点赞持久化事件；失败仅告警（Redis 已生效，不阻塞用户）。
func (s *MomentService) publishLikeEvent(ctx context.Context, momentID, userID int64, action string) {
	evt := &model.LikeEvent{
		MomentID: momentID,
		UserID:   userID,
		Action:   action,
		Ts:       time.Now().UnixMilli(),
	}
	if err := s.mqRepo.PublishLikeEvent(ctx, evt); err != nil {
		s.logger.Error("发布点赞持久化事件失败",
			zap.Int64("momentID", momentID),
			zap.Int64("userID", userID),
			zap.String("action", action),
			zap.Error(err),
		)
	}
}

// enrichLikes 为一批动态填充 LikeCount / LikedByMe：先逐条确保缓存已预热，再一次性批量读取。
// 读取失败为非致命，保持零值。
func (s *MomentService) enrichLikes(ctx context.Context, viewerID int64, moments []model.Moment) {
	if len(moments) == 0 {
		return
	}
	ids := make([]int64, len(moments))
	for i := range moments {
		ids[i] = moments[i].ID
		if err := s.ensureLikesLoaded(ctx, moments[i].ID); err != nil {
			s.logger.Warn("预热点赞缓存失败", zap.Int64("momentID", moments[i].ID), zap.Error(err))
		}
	}
	counts, liked, err := s.redisRepo.GetMomentLikeStats(ctx, viewerID, ids)
	if err != nil {
		s.logger.Warn("批量读取点赞统计失败", zap.Error(err))
		return
	}
	for i := range moments {
		moments[i].LikeCount = counts[moments[i].ID]
		moments[i].LikedByMe = liked[moments[i].ID]
	}
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

// GetFeed 以推拉结合方式获取用户的朋友圈 Feed：
//   - 推：读取用户自己的收件箱 timeline:{userID}（普通好友写扩散来的动态）。
//   - 拉：读取用户自己的寄件箱 + 大V好友的寄件箱 moment_outbox:{bigFriend}。
//
// 各源按 score(时间戳) 降序取一页，Go 内归并、按 momentID 去重，
// 用复合游标 (ts,id) 分页规避重复/漏读与深分页衰减。
// 返回补全后的动态列表与 nextCursor（空串表示无更多）。
func (s *MomentService) GetFeed(ctx context.Context, userID int64, cursor string, limit int) ([]model.Moment, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	maxTs, maxID, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", fmt.Errorf("无效的游标: %w", err)
	}

	// ── 1. 确定所有 Feed 源 ──
	// pull=true 读寄件箱（自己 + 大V好友），pull=false 读收件箱（自己）。
	type feedSource struct {
		userID int64
		pull   bool
	}
	sources := []feedSource{
		{userID: userID, pull: true},  // 自己的寄件箱（自见）
		{userID: userID, pull: false}, // 自己的收件箱（普通好友推来的）
	}

	friends, err := s.mysqlRepo.GetFriendList(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("获取好友列表: %w", err)
	}
	if len(friends) > 0 {
		friendIDs := make([]int64, 0, len(friends))
		for _, fs := range friends {
			if fs.FriendID != userID {
				friendIDs = append(friendIDs, fs.FriendID)
			}
		}
		bigFriends, err := s.redisRepo.FilterBigUsers(ctx, friendIDs)
		if err != nil {
			return nil, "", fmt.Errorf("筛选大V好友: %w", err)
		}
		for _, bf := range bigFriends {
			sources = append(sources, feedSource{userID: bf, pull: true}) // 大V好友的寄件箱（拉取）
		}
	}

	// ── 2. 各源取一页并归并 ──
	// 每源多取 1 条用于判定是否还有下一页。
	seen := make(map[int64]struct{})
	merged := make([]model.FeedEntry, 0, limit*2)
	for _, src := range sources {
		var entries []model.FeedEntry
		var err error
		if src.pull {
			entries, err = s.redisRepo.GetOutboxPage(ctx, src.userID, maxTs, maxID, limit+1)
		} else {
			entries, err = s.redisRepo.GetTimelinePage(ctx, src.userID, maxTs, maxID, limit+1)
		}
		if err != nil {
			s.logger.Warn("读取 Feed 源失败",
				zap.Int64("srcUserID", src.userID), zap.Bool("pull", src.pull), zap.Error(err))
			continue // 单源失败不影响其它源
		}
		// 边界过滤（严格早于游标）已在 repo 层完成，这里只需按 momentID 去重跨源合并。
		for _, e := range entries {
			if _, dup := seen[e.MomentID]; dup {
				continue
			}
			seen[e.MomentID] = struct{}{}
			merged = append(merged, e)
		}
	}

	if len(merged) == 0 {
		return []model.Moment{}, "", nil
	}

	// ── 3. 按 (ts desc, id desc) 排序，取 limit 条 ──
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Ts != merged[j].Ts {
			return merged[i].Ts > merged[j].Ts
		}
		return merged[i].MomentID > merged[j].MomentID
	})

	nextCursor := ""
	if len(merged) > limit {
		last := merged[limit-1]
		nextCursor = encodeCursor(last.Ts, last.MomentID)
		merged = merged[:limit]
	}

	// ── 4. 批量补全动态详情（消除 N+1）──
	ids := make([]int64, len(merged))
	for i, e := range merged {
		ids[i] = e.MomentID
	}
	rows, err := s.mysqlRepo.GetMomentsByIDs(ctx, ids)
	if err != nil {
		return nil, "", fmt.Errorf("批量补全动态: %w", err)
	}
	byID := make(map[int64]model.Moment, len(rows))
	for _, m := range rows {
		byID[m.ID] = m
	}

	// 按归并顺序重排，并做读时可见性过滤（私密动态仅作者本人可见）。
	moments := make([]model.Moment, 0, len(merged))
	for _, e := range merged {
		m, ok := byID[e.MomentID]
		if !ok {
			continue // 动态已删除，读时容错跳过
		}
		if m.Visibility == 3 && m.AuthorID != userID {
			continue
		}
		moments = append(moments, m)
	}

	// 富化点赞数与"我是否已赞"
	s.enrichLikes(ctx, userID, moments)

	return moments, nextCursor, nil
}

// ── 游标编解码 ──
// 游标为 base64("{ts}_{id}")，复合键 (时间戳ms, momentID) 保证同毫秒多条也能稳定分界。

// encodeCursor 将 (ts,id) 编码为不透明游标字符串。
func encodeCursor(ts int64, id int64) string {
	raw := strconv.FormatInt(ts, 10) + "_" + strconv.FormatInt(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor 解析游标字符串，返回 (maxTs, maxID)。
// 空游标表示首页，返回 (-1, 0)：maxTs<0 由 Redis 层识别为"从最新开始"。
func decodeCursor(cursor string) (int64, int64, error) {
	if cursor == "" {
		return -1, 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, 0, fmt.Errorf("游标 base64 解码: %w", err)
	}
	parts := strings.SplitN(string(data), "_", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("游标格式错误")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("游标时间戳解析: %w", err)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("游标ID解析: %w", err)
	}
	return ts, id, nil
}
