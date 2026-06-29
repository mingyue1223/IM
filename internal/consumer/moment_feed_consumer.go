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

// ── Constants ──

const momentPushQueue = "moment_push"

// MomentFeedConsumer processes messages from the moment_push queue.
// For each moment publish event, it fan-outs to all friends' timeline ZSets
// and adds the moment to the author's own timeline.
type MomentFeedConsumer struct {
	ch        *amqp.Channel
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	logger    *zap.Logger
}

// NewMomentFeedConsumer creates a new MomentFeedConsumer.
func NewMomentFeedConsumer(ch *amqp.Channel, mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, logger *zap.Logger) *MomentFeedConsumer {
	return &MomentFeedConsumer{
		ch:        ch,
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}
}

// Start begins consuming messages from the moment_push queue.
// Runs in a goroutine; blocks until the channel closes or context is cancelled.
func (c *MomentFeedConsumer) Start(ctx context.Context) error {
	deliveries, err := c.ch.Consume(
		momentPushQueue,
		"goim-moment-feed-consumer", // consumer tag
		false,                        // autoAck — we ack manually
		false,                        // exclusive
		false,                        // noLocal
		false,                        // noWait
		nil,                          // args
	)
	if err != nil {
		return fmt.Errorf("consume moment_push: %w", err)
	}

	c.logger.Info("moment feed consumer started")

	go func() {
		for d := range deliveries {
			c.handleDelivery(ctx, d)
		}
		c.logger.Info("moment feed consumer deliveries channel closed")
	}()

	return nil
}

// handleDelivery processes a single AMQP delivery.
// On success: ack. On failure: nack with requeue so the message is retried.
func (c *MomentFeedConsumer) handleDelivery(ctx context.Context, d amqp.Delivery) {
	moment, err := deserializeMoment(d.Body)
	if err != nil {
		c.logger.Error("failed to deserialize moment", zap.Error(err))
		// Malformed message — nack without requeue (discard)
		d.Nack(false, false)
		return
	}

	if err := c.process(ctx, moment); err != nil {
		c.logger.Error("failed to process moment fan-out",
			zap.Int64("momentID", moment.ID),
			zap.Int64("authorID", moment.AuthorID),
			zap.Error(err),
		)
		// Transient failure — nack with requeue for retry
		d.Nack(false, true)
		return
	}

	d.Ack(false)
}

// process executes the fan-out logic for a moment.
// It adds the moment to the author's own timeline and to all friends' timelines.
func (c *MomentFeedConsumer) process(ctx context.Context, moment *model.Moment) error {
	timestamp := moment.CreatedAt.UnixMilli()

	// ── 1. Add to author's own timeline ──
	if err := c.redisRepo.PublishMomentFeed(ctx, moment.AuthorID, moment.ID, timestamp); err != nil {
		c.logger.Warn("failed to add moment to author's own timeline",
			zap.Int64("authorID", moment.AuthorID),
			zap.Int64("momentID", moment.ID),
			zap.Error(err),
		)
		// non-critical — don't fail the whole delivery
	}

	// ── 2. Fan-out to friends ──
	// Only fan-out if visibility is "all" (1) or "friends only" (2)
	// Private moments (3) are only visible to the author
	if moment.Visibility == 3 {
		c.logger.Debug("private moment — skipping fan-out",
			zap.Int64("momentID", moment.ID),
			zap.Int64("authorID", moment.AuthorID),
		)
		return nil
	}

	friends, err := c.mysqlRepo.GetFriendList(ctx, moment.AuthorID)
	if err != nil {
		return fmt.Errorf("get friend list for author %d: %w", moment.AuthorID, err)
	}

	for _, fs := range friends {
		friendID := fs.FriendID
		if friendID == moment.AuthorID {
			// Skip self (already added to own timeline above)
			continue
		}

		if err := c.redisRepo.PublishMomentFeed(ctx, friendID, moment.ID, timestamp); err != nil {
			c.logger.Warn("failed to add moment to friend's timeline",
				zap.Int64("friendID", friendID),
				zap.Int64("momentID", moment.ID),
				zap.Error(err),
			)
			// non-critical for one friend — continue with remaining friends
		}
	}

	c.logger.Debug("moment fan-out completed",
		zap.Int64("momentID", moment.ID),
		zap.Int64("authorID", moment.AuthorID),
		zap.Int("friendCount", len(friends)),
	)

	return nil
}

// ── Helpers ──

// deserializeMoment parses AMQP body into a Moment.
func deserializeMoment(body []byte) (*model.Moment, error) {
	var moment model.Moment
	if err := json.Unmarshal(body, &moment); err != nil {
		return nil, fmt.Errorf("unmarshal moment: %w", err)
	}
	if moment.AuthorID == 0 || moment.ID == 0 {
		return nil, fmt.Errorf("invalid moment: authorID=%d, id=%d", moment.AuthorID, moment.ID)
	}
	return &moment, nil
}
