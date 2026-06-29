package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/goim/goim/internal/conn"
	"github.com/goim/goim/internal/model"
	"github.com/goim/goim/internal/protocol"
	"github.com/goim/goim/internal/repository"
)

// ── Constants ──

const (
	privateMsgQueue  = "private_msg_persist"
	inboxMaxCount    = 2000 // max entries retained per user inbox after trim
)

// PrivateMsgConsumer processes messages from the private_msg_persist queue.
// For each PrivateMessage it:
//  1. Writes to receiver's inbox (ZADD inbox:{receiverID}, readStatus=0)
//  2. Writes to sender's inbox (ZADD inbox:{senderID}, readStatus=1 — sender "read" their own message)
//  3. Updates conv_list for both parties
//  4. Increments unread counter for receiver
//  5. Pushes InboxMessage to online receiver via WebSocket
//  6. Inserts PrivateMessage into MySQL
//  7. Trims both inbox ZSets
type PrivateMsgConsumer struct {
	ch        *amqp.Channel
	mysqlRepo repository.MySQLRepo
	redisRepo repository.RedisRepo
	cm        *conn.ConnectionManager
	logger    *zap.Logger
}

// NewPrivateMsgConsumer creates a new PrivateMsgConsumer.
func NewPrivateMsgConsumer(ch *amqp.Channel, mysqlRepo repository.MySQLRepo, redisRepo repository.RedisRepo, cm *conn.ConnectionManager, logger *zap.Logger) *PrivateMsgConsumer {
	return &PrivateMsgConsumer{
		ch:        ch,
		mysqlRepo: mysqlRepo,
		redisRepo: redisRepo,
		cm:        cm,
		logger:    logger,
	}
}

// Start begins consuming messages from the private_msg_persist queue.
// Runs in a goroutine; blocks until the channel closes or context is cancelled.
// Uses manual acknowledgement: ack on success, nack+requeue on failure.
func (c *PrivateMsgConsumer) Start(ctx context.Context) error {
	deliveries, err := c.ch.Consume(
		privateMsgQueue,
		"goim-private-msg-consumer", // consumer tag
		false,                        // autoAck — we ack manually
		false,                        // exclusive
		false,                        // noLocal
		false,                        // noWait
		nil,                          // args
	)
	if err != nil {
		return fmt.Errorf("consume private_msg_persist: %w", err)
	}

	c.logger.Info("private msg consumer started")

	go func() {
		for d := range deliveries {
			c.handleDelivery(ctx, d)
		}
		c.logger.Info("private msg consumer deliveries channel closed")
	}()

	return nil
}

// handleDelivery processes a single AMQP delivery.
// On success: ack. On failure: nack with requeue so the message is retried.
func (c *PrivateMsgConsumer) handleDelivery(ctx context.Context, d amqp.Delivery) {
	msg, err := deserializePrivateMsg(d.Body)
	if err != nil {
		c.logger.Error("failed to deserialize private message", zap.Error(err))
		// Malformed message — nack without requeue (discard) to avoid infinite retry
		d.Nack(false, false)
		return
	}

	if err := c.process(ctx, msg); err != nil {
		c.logger.Error("failed to process private message",
			zap.Int64("msgID", msg.ID),
			zap.Int64("senderID", msg.SenderID),
			zap.Int64("receiverID", msg.ReceiverID),
			zap.Error(err),
		)
		// Transient failure — nack with requeue for retry
		d.Nack(false, true)
		return
	}

	d.Ack(false)
}

// process executes the full fan-out logic for a private message.
func (c *PrivateMsgConsumer) process(ctx context.Context, msg *model.PrivateMessage) error {
	convID := model.BuildConvID(model.ConvTypePrivate, msg.SenderID, msg.ReceiverID)
	timestamp := msg.CreatedAt.UnixMilli()

	// ── 1. Build InboxMessage for receiver (readStatus=0) ──
	receiverInboxMsg := &model.InboxMessage{
		MsgID:      msg.ID,
		ConvID:     convID,
		ConvType:   model.ConvTypePrivate,
		FromID:     msg.SenderID,
		ToID:       msg.ReceiverID,
		MsgType:    msg.MsgType,
		Content:    msg.Content,
		ReadStatus: 0, // unread for receiver
		Timestamp:  timestamp,
	}

	// ── 2. Build InboxMessage for sender (readStatus=1) ──
	senderInboxMsg := &model.InboxMessage{
		MsgID:      msg.ID,
		ConvID:     convID,
		ConvType:   model.ConvTypePrivate,
		FromID:     msg.SenderID,
		ToID:       msg.ReceiverID,
		MsgType:    msg.MsgType,
		Content:    msg.Content,
		ReadStatus: 1, // sender has "read" their own message
		Timestamp:  timestamp,
	}

	// ── 3. Write to both inboxes ──
	if err := c.redisRepo.WriteInbox(ctx, msg.ReceiverID, receiverInboxMsg); err != nil {
		return fmt.Errorf("write receiver inbox: %w", err)
	}
	if err := c.redisRepo.WriteInbox(ctx, msg.SenderID, senderInboxMsg); err != nil {
		return fmt.Errorf("write sender inbox: %w", err)
	}

	// ── 4. Update conv_list for both parties ──
	convSummary := buildPrivateConvSummary(convID, msg)
	summaryJSON, err := json.Marshal(convSummary)
	if err != nil {
		c.logger.Warn("failed to marshal conv summary", zap.Error(err))
		summaryJSON = []byte(convID) // fallback
	}

	if err := c.redisRepo.UpdateConvList(ctx, msg.SenderID, convID, string(summaryJSON), timestamp); err != nil {
		c.logger.Warn("update sender conv_list failed", zap.Error(err))
		// non-critical — don't fail the whole delivery
	}
	if err := c.redisRepo.UpdateConvList(ctx, msg.ReceiverID, convID, string(summaryJSON), timestamp); err != nil {
		c.logger.Warn("update receiver conv_list failed", zap.Error(err))
	}

	// ── 5. Increment unread counter for receiver ──
	if err := c.redisRepo.IncrementUnread(ctx, msg.ReceiverID, convID); err != nil {
		c.logger.Warn("increment unread failed", zap.Error(err))
		// non-critical
	}

	// ── 6. Push to online receiver via WebSocket ──
	pushToConnection(c.cm, c.logger, msg.ReceiverID, protocol.TypeMsg, receiverInboxMsg)

	// ── 7. Insert into MySQL ──
	if c.mysqlRepo != nil {
		if err := c.mysqlRepo.InsertPrivateMessage(ctx, msg); err != nil {
			return fmt.Errorf("insert private message to MySQL: %w", err)
		}
	}

	// ── 8. Trim both inbox ZSets ──
	if err := c.redisRepo.TrimInbox(ctx, msg.ReceiverID, inboxMaxCount); err != nil {
		c.logger.Warn("trim receiver inbox failed", zap.Error(err))
	}
	if err := c.redisRepo.TrimInbox(ctx, msg.SenderID, inboxMaxCount); err != nil {
		c.logger.Warn("trim sender inbox failed", zap.Error(err))
	}

	c.logger.Debug("private message processed",
		zap.Int64("msgID", msg.ID),
		zap.Int64("senderID", msg.SenderID),
		zap.Int64("receiverID", msg.ReceiverID),
	)

	return nil
}

// ── Helpers ──

// deserializePrivateMsg parses AMQP body into a PrivateMessage.
func deserializePrivateMsg(body []byte) (*model.PrivateMessage, error) {
	var msg model.PrivateMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal private message: %w", err)
	}
	if msg.SenderID == 0 || msg.ReceiverID == 0 {
		return nil, fmt.Errorf("invalid private message: senderID=%d, receiverID=%d", msg.SenderID, msg.ReceiverID)
	}
	return &msg, nil
}

// buildPrivateConvSummary creates a ConvSummary for a private conversation.
// TargetName and TargetAvatar are left empty — they'll be filled during sync
// by the message service or by a separate user lookup.
func buildPrivateConvSummary(convID string, msg *model.PrivateMessage) *model.ConvSummary {
	return &model.ConvSummary{
		ConvID:      convID,
		ConvType:    model.ConvTypePrivate,
		TargetID:    msg.ReceiverID,
		LastMsg:     truncateContent(msg.Content, 50),
		LastMsgTime: msg.CreatedAt.UnixMilli(),
	}
}

// truncateContent shortens content to maxLen chars for conv summary display.
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	if strings.Contains(content, "\n") {
		// For multiline content, show first line + indicator
		firstLine := strings.Split(content, "\n")[0]
		if len(firstLine) > maxLen {
			return firstLine[:maxLen] + "..."
		}
		return firstLine + "\n..."
	}
	return content[:maxLen] + "..."
}

// pushToConnection sends a message to an online user via WebSocket.
// If the user is offline, the message is silently dropped (they'll get it via sync).
// This helper is shared by both consumers.
func pushToConnection(cm *conn.ConnectionManager, logger *zap.Logger, userID int64, msgType string, data interface{}) {
	encoded, err := protocol.EncodeMsg(msgType, data)
	if err != nil {
		logger.Error("EncodeMsg failed", zap.String("type", msgType), zap.Error(err))
		return
	}

	client, ok := cm.Get(userID)
	if !ok {
		// User offline — they'll get it via sync
		return
	}

	select {
	case client.SendCh <- encoded:
		// sent successfully
	default:
		// buffer full — drop message (user will sync later)
		logger.Warn("SendCh buffer full, dropping message",
			zap.Int64("userID", userID),
			zap.String("type", msgType),
		)
	}
}

// nowUnixMilli returns current time as Unix milliseconds.
func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
