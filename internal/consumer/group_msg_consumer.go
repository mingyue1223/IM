package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/protocol"
	"github.com/goim/goim/internal/repository"
)

// ── Constants ──

const (
	groupMsgQueue  = "group_msg_fanout"
	outboxMaxCount = 2000 // max entries retained per group outbox after trim
)

// GroupMsgConsumer processes messages from the group_msg_fanout queue.
// For each GroupMessage it:
//  1. Writes to group's outbox (ZADD outbox:{groupID}, InboxMessage with convType=2)
//  2. Gets group members list (from Redis SET group_members:{groupID})
//  3. For each member:
//     a. Updates conv_list (ZADD conv_list:{userID})
//     b. Increments unread counter (skip sender)
//  4. Pushes InboxMessage to online members via WebSocket (skip sender)
//  5. Inserts GroupMessage into MySQL
//  6. Trims outbox ZSet
type GroupMsgConsumer struct {
	ch        *amqp.Channel
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	cm        *conn.ConnectionManager
	logger    *zap.Logger
}

// NewGroupMsgConsumer creates a new GroupMsgConsumer.
func NewGroupMsgConsumer(ch *amqp.Channel, mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, cm *conn.ConnectionManager, logger *zap.Logger) *GroupMsgConsumer {
	return &GroupMsgConsumer{
		ch:        ch,
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		cm:        cm,
		logger:    logger,
	}
}

// Start begins consuming messages from the group_msg_fanout queue.
// Runs in a goroutine; blocks until the channel closes or context is cancelled.
// Uses manual acknowledgement: ack on success, nack+requeue on failure.
func (c *GroupMsgConsumer) Start(ctx context.Context) error {
	deliveries, err := c.ch.Consume(
		groupMsgQueue,
		"goim-group-msg-consumer", // consumer tag
		false,                      // autoAck — we ack manually
		false,                      // exclusive
		false,                      // noLocal
		false,                      // noWait
		nil,                        // args
	)
	if err != nil {
		return fmt.Errorf("consume group_msg_fanout: %w", err)
	}

	c.logger.Info("group msg consumer started")

	go func() {
		for d := range deliveries {
			c.handleDelivery(ctx, d)
		}
		c.logger.Info("group msg consumer deliveries channel closed")
	}()

	return nil
}

// handleDelivery processes a single AMQP delivery.
// On success: ack. On failure: nack with requeue for retry.
func (c *GroupMsgConsumer) handleDelivery(ctx context.Context, d amqp.Delivery) {
	msg, err := deserializeGroupMsg(d.Body)
	if err != nil {
		c.logger.Error("failed to deserialize group message", zap.Error(err))
		// Malformed message — discard to avoid infinite retry of bad data
		d.Nack(false, false)
		return
	}

	if err := c.process(ctx, msg); err != nil {
		c.logger.Error("failed to process group message",
			zap.Int64("msgID", msg.ID),
			zap.Int64("groupID", msg.GroupID),
			zap.Int64("senderID", msg.SenderID),
			zap.Error(err),
		)
		// Transient failure — requeue for retry
		d.Nack(false, true)
		return
	}

	d.Ack(false)
}

// process executes the full fan-out logic for a group message.
func (c *GroupMsgConsumer) process(ctx context.Context, msg *model.GroupMessage) error {
	convID := model.BuildConvID(model.ConvTypeGroup, msg.GroupID, 0)
	timestamp := msg.CreatedAt.UnixMilli()

	// ── 1. Build InboxMessage for group outbox ──
	outboxMsg := &model.InboxMessage{
		MsgID:    msg.ID,
		ConvID:   convID,
		ConvType: model.ConvTypeGroup,
		FromID:   msg.SenderID,
		ToID:     msg.GroupID,
		MsgType:  msg.MsgType,
		Content:  msg.Content,
		GroupSeq: msg.GroupSeq,
		Timestamp: timestamp,
	}

	// ── 2. Write to group outbox ──
	if err := c.redisRepo.WriteOutbox(ctx, msg.GroupID, outboxMsg); err != nil {
		return fmt.Errorf("write group outbox: %w", err)
	}

	// ── 3. Get group members ──
	memberIDs, err := c.redisRepo.GetGroupMembers(ctx, msg.GroupID)
	if err != nil {
		return fmt.Errorf("get group members: %w", err)
	}

	if len(memberIDs) == 0 {
		c.logger.Warn("group has no members",
			zap.Int64("groupID", msg.GroupID),
		)
		// Continue to persist in MySQL even if no members — outbox is still written
	}

	// ── 4. For each member: update conv_list, increment unread, push via WS ──
	convSummary := buildGroupConvSummary(convID, msg)
	summaryJSON, err := json.Marshal(convSummary)
	if err != nil {
		c.logger.Warn("failed to marshal conv summary", zap.Error(err))
		summaryJSON = []byte(convID) // fallback
	}

	for _, memberID := range memberIDs {
		// Update conv_list for every member (including sender)
		if err := c.redisRepo.UpdateConvList(ctx, memberID, convID, string(summaryJSON), timestamp); err != nil {
			c.logger.Warn("update conv_list failed for member",
				zap.Int64("memberID", memberID),
				zap.Error(err),
			)
			// non-critical
		}

		// Increment unread for all members except sender
		if memberID != msg.SenderID {
			if err := c.redisRepo.IncrementUnread(ctx, memberID, convID); err != nil {
				c.logger.Warn("increment unread failed for member",
					zap.Int64("memberID", memberID),
					zap.Error(err),
				)
				// non-critical
			}

			// Push InboxMessage to online members (skip sender)
			pushToConnection(c.cm, c.logger, memberID, protocol.TypeMsg, outboxMsg)
		}
	}

	// ── 5. Insert into MySQL ──
	if c.mysqlRepo != nil {
		if err := c.mysqlRepo.InsertGroupMessage(ctx, msg); err != nil {
		return fmt.Errorf("insert group message to MySQL: %w", err)
		}
	}

	// ── 6. Trim outbox ZSet ──
	if err := c.redisRepo.TrimOutbox(ctx, msg.GroupID, outboxMaxCount); err != nil {
		c.logger.Warn("trim group outbox failed", zap.Error(err))
	}

	c.logger.Debug("group message processed",
		zap.Int64("msgID", msg.ID),
		zap.Int64("groupID", msg.GroupID),
		zap.Int64("groupSeq", msg.GroupSeq),
		zap.Int64("senderID", msg.SenderID),
		zap.Int("memberCount", len(memberIDs)),
	)

	return nil
}

// ── Helpers ──

// deserializeGroupMsg parses AMQP body into a GroupMessage.
func deserializeGroupMsg(body []byte) (*model.GroupMessage, error) {
	var msg model.GroupMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal group message: %w", err)
	}
	if msg.GroupID == 0 {
		return nil, fmt.Errorf("invalid group message: groupID=%d", msg.GroupID)
	}
	return &msg, nil
}

// buildGroupConvSummary creates a ConvSummary for a group conversation.
// TargetName is left empty — filled during sync by a group info lookup.
func buildGroupConvSummary(convID string, msg *model.GroupMessage) *model.ConvSummary {
	return &model.ConvSummary{
		ConvID:      convID,
		ConvType:    model.ConvTypeGroup,
		TargetID:    msg.GroupID,
		LastMsg:     truncateContent(msg.Content, 50),
		LastMsgTime: msg.CreatedAt.UnixMilli(),
	}
}
