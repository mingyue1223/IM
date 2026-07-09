package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/repository"
)

// ── 常量 ──

const likePersistQueue = "like_persist"

// LikePersistConsumer 消费 like_persist 队列，把点赞/取消赞事件**批量削峰**镜像写入 MySQL。
//
// 削峰策略：不逐条落库，而是攒批——攒够 batchSize 条 或 每 flushInterval 触发一次 flush，
// 把一批事件按 (momentID,userID) 折叠为最终动作（last-action-wins），再分成
// 批量 INSERT IGNORE（点赞）与批量 DELETE（取消赞）各一条 SQL 落库，显著降低写库 QPS。
//
// 一致性：INSERT IGNORE / DELETE 均幂等，整批落库成功后统一 ack；失败则整批 nack+requeue 重试。
// 单 batcher goroutine 顺序 flush，保证跨批的时间先后；批内 last-action-wins 保证同一
// (moment,user) 落库为最终态。
type LikePersistConsumer struct {
	ch        *amqp.Channel
	mysqlRepo repository.MySQLRepo
	logger    *zap.Logger

	batchSize     int
	flushInterval time.Duration
}

// NewLikePersistConsumer 创建 LikePersistConsumer。
// batchSize/flushMs 来自配置（config.MomentConfig）。
func NewLikePersistConsumer(ch *amqp.Channel, mysqlRepo repository.MySQLRepo, logger *zap.Logger, batchSize, flushMs int) *LikePersistConsumer {
	if batchSize <= 0 {
		batchSize = 200
	}
	if flushMs <= 0 {
		flushMs = 500
	}
	return &LikePersistConsumer{
		ch:            ch,
		mysqlRepo:     mysqlRepo,
		logger:        logger,
		batchSize:     batchSize,
		flushInterval: time.Duration(flushMs) * time.Millisecond,
	}
}

// Start 开始从 like_persist 队列消费。在 goroutine 中运行；投递通道关闭或 ctx 取消时退出。
func (c *LikePersistConsumer) Start(ctx context.Context) error {
	deliveries, err := c.ch.Consume(
		likePersistQueue,
		"goim-like-persist-consumer", // 消费者标签
		false,                        // autoAck — 手动确认
		false,                        // exclusive
		false,                        // noLocal
		false,                        // noWait
		nil,                          // args
	)
	if err != nil {
		return fmt.Errorf("消费 like_persist 失败: %w", err)
	}

	c.logger.Info("点赞持久化消费者已启动",
		zap.Int("batchSize", c.batchSize),
		zap.Duration("flushInterval", c.flushInterval),
	)

	go c.runBatcher(ctx, deliveries)
	return nil
}

// runBatcher 攒批循环：攒够 batchSize 或到 flushInterval 就 flush；通道关闭/ctx 取消时 flush 残余后退出。
func (c *LikePersistConsumer) runBatcher(ctx context.Context, deliveries <-chan amqp.Delivery) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	batch := make([]amqp.Delivery, 0, c.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		c.flush(ctx, batch)
		batch = batch[:0]
	}

	for {
		select {
		case d, ok := <-deliveries:
			if !ok {
				flush()
				c.logger.Info("点赞持久化消费者投递通道已关闭")
				return
			}
			batch = append(batch, d)
			if len(batch) >= c.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			c.logger.Info("点赞持久化消费者已停止（ctx 取消）")
			return
		}
	}
}

// flush 落库一批事件：解析（丢弃格式错误）→ 折叠 last-action-wins → 批量 upsert/delete → 统一 ack。
func (c *LikePersistConsumer) flush(ctx context.Context, batch []amqp.Delivery) {
	type folded struct {
		action string
		ts     int64
	}
	// map 覆盖天然实现"按到达顺序 last-action-wins"（valid 切片保持到达顺序）。
	final := make(map[model.MomentLikeKey]folded, len(batch))
	valid := make([]amqp.Delivery, 0, len(batch))

	for _, d := range batch {
		evt, err := deserializeLikeEvent(d.Body)
		if err != nil {
			c.logger.Error("反序列化点赞事件失败", zap.Error(err))
			d.Nack(false, false) // 格式错误 — 丢弃，不重投
			continue
		}
		valid = append(valid, d)
		final[model.MomentLikeKey{MomentID: evt.MomentID, UserID: evt.UserID}] = folded{action: evt.Action, ts: evt.Ts}
	}

	if len(valid) == 0 {
		return
	}

	upserts := make([]model.MomentLike, 0, len(final))
	deletes := make([]model.MomentLikeKey, 0, len(final))
	for key, f := range final {
		if f.action == model.LikeActionUnlike {
			deletes = append(deletes, key)
		} else {
			upserts = append(upserts, model.MomentLike{
				MomentID:  key.MomentID,
				UserID:    key.UserID,
				CreatedAt: time.UnixMilli(f.ts),
			})
		}
	}

	if err := c.mysqlRepo.BatchUpsertMomentLikes(ctx, upserts); err != nil {
		c.logger.Error("批量落库点赞失败，整批重投", zap.Int("count", len(upserts)), zap.Error(err))
		c.nackAll(valid)
		return
	}
	if err := c.mysqlRepo.BatchDeleteMomentLikes(ctx, deletes); err != nil {
		c.logger.Error("批量落库取消赞失败，整批重投", zap.Int("count", len(deletes)), zap.Error(err))
		c.nackAll(valid)
		return
	}

	for _, d := range valid {
		d.Ack(false)
	}
	c.logger.Debug("点赞事件批量落库完成",
		zap.Int("events", len(valid)),
		zap.Int("upserts", len(upserts)),
		zap.Int("deletes", len(deletes)),
	)
}

// nackAll 整批 nack 并 requeue（幂等落库，重试安全）。
func (c *LikePersistConsumer) nackAll(deliveries []amqp.Delivery) {
	for _, d := range deliveries {
		d.Nack(false, true)
	}
}

// deserializeLikeEvent 将 AMQP 消息体解析为 LikeEvent 并校验。
func deserializeLikeEvent(body []byte) (*model.LikeEvent, error) {
	var evt model.LikeEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return nil, fmt.Errorf("反序列化点赞事件: %w", err)
	}
	if evt.MomentID == 0 || evt.UserID == 0 {
		return nil, fmt.Errorf("无效点赞事件: momentID=%d, userID=%d", evt.MomentID, evt.UserID)
	}
	if evt.Action != model.LikeActionLike && evt.Action != model.LikeActionUnlike {
		return nil, fmt.Errorf("未知点赞动作: %q", evt.Action)
	}
	return &evt, nil
}
