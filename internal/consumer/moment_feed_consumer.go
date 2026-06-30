package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── 常量 ──

const momentPushQueue = "moment_push"

// MomentFeedConsumer 处理来自 moment_push 队列的消息。
// 对于每条动态发布事件，它将动态分发到所有好友的时间线 ZSet 中，
// 并将动态添加到作者自己的时间线中。
type MomentFeedConsumer struct {
	ch        *amqp.Channel
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	logger    *zap.Logger
}

// NewMomentFeedConsumer 创建一个新的 MomentFeedConsumer。
func NewMomentFeedConsumer(ch *amqp.Channel, mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, logger *zap.Logger) *MomentFeedConsumer {
	return &MomentFeedConsumer{
		ch:        ch,
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}
}

// Start 开始从 moment_push 队列消费消息。
// 在 goroutine 中运行；阻塞直到通道关闭或上下文被取消。
func (c *MomentFeedConsumer) Start(ctx context.Context) error {
	deliveries, err := c.ch.Consume(
		momentPushQueue,
		"goim-moment-feed-consumer", // 消费者标签
		false,                        // autoAck — 手动确认
		false,                        // exclusive 排他
		false,                        // noLocal
		false,                        // noWait
		nil,                          // args 参数
	)
	if err != nil {
		return fmt.Errorf("消费 moment_push: %w", err)
	}

	c.logger.Info("动态消息消费者已启动")

	go func() {
		for d := range deliveries {
			c.handleDelivery(ctx, d)
		}
		c.logger.Info("动态消息消费者投递通道已关闭")
	}()

	return nil
}

// handleDelivery 处理单条 AMQP 投递消息。
// 成功：ack 确认。失败：nack 并重新入队以便重试。
func (c *MomentFeedConsumer) handleDelivery(ctx context.Context, d amqp.Delivery) {
	moment, err := deserializeMoment(d.Body)
	if err != nil {
		c.logger.Error("反序列化动态失败", zap.Error(err))
		// 消息格式错误 — nack 但不重新入队（丢弃）
		d.Nack(false, false)
		return
	}

	if err := c.process(ctx, moment); err != nil {
		c.logger.Error("动态分发处理失败",
			zap.Int64("momentID", moment.ID),
			zap.Int64("authorID", moment.AuthorID),
			zap.Error(err),
		)
		// 临时性错误 — nack 并重新入队等待重试
		d.Nack(false, true)
		return
	}

	d.Ack(false)
}

// process 执行动态的分发逻辑。
// 它将动态添加到作者自己的时间线以及所有好友的时间线中。
func (c *MomentFeedConsumer) process(ctx context.Context, moment *model.Moment) error {
	timestamp := moment.CreatedAt.UnixMilli()

	// ── 1. 添加到作者自己的时间线 ──
	if err := c.redisRepo.PublishMomentFeed(ctx, moment.AuthorID, moment.ID, timestamp); err != nil {
		c.logger.Warn("添加动态到作者自己的时间线失败",
			zap.Int64("authorID", moment.AuthorID),
			zap.Int64("momentID", moment.ID),
			zap.Error(err),
		)
		// 非关键错误 — 不要导致整个投递失败
	}

	// ── 2. 分发给好友 ──
	// 仅当可见性为"全部"(1) 或 "仅好友"(2) 时才进行分发
	// 私密动态(3) 仅对作者本人可见
	if moment.Visibility == 3 {
		c.logger.Debug("私密动态 — 跳过分发",
			zap.Int64("momentID", moment.ID),
			zap.Int64("authorID", moment.AuthorID),
		)
		return nil
	}

	friends, err := c.mysqlRepo.GetFriendList(ctx, moment.AuthorID)
	if err != nil {
		return fmt.Errorf("获取作者 %d 的好友列表失败: %w", moment.AuthorID, err)
	}

	for _, fs := range friends {
		friendID := fs.FriendID
		if friendID == moment.AuthorID {
			// 跳过自己（已在上面添加到自己的时间线）
			continue
		}

		if err := c.redisRepo.PublishMomentFeed(ctx, friendID, moment.ID, timestamp); err != nil {
			c.logger.Warn("添加动态到好友时间线失败",
				zap.Int64("friendID", friendID),
				zap.Int64("momentID", moment.ID),
				zap.Error(err),
			)
			// 单个好友失败是非关键错误 — 继续处理剩余好友
		}
	}

	c.logger.Debug("动态分发完成",
		zap.Int64("momentID", moment.ID),
		zap.Int64("authorID", moment.AuthorID),
		zap.Int("friendCount", len(friends)),
	)

	return nil
}

// ── 辅助函数 ──

// deserializeMoment 将 AMQP 消息体解析为 Moment 结构体。
func deserializeMoment(body []byte) (*model.Moment, error) {
	var moment model.Moment
	if err := json.Unmarshal(body, &moment); err != nil {
		return nil, fmt.Errorf("反序列化动态: %w", err)
	}
	if moment.AuthorID == 0 || moment.ID == 0 {
		return nil, fmt.Errorf("无效的动态: authorID=%d, id=%d", moment.AuthorID, moment.ID)
	}
	return &moment, nil
}
