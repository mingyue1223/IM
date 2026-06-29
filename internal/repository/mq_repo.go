package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/goim/goim/internal/model"
)

// MQRepo defines the MQ publish operations needed by the message service.
// The interface allows mocking in tests.
type MQRepo interface {
	PublishPrivateMsg(ctx context.Context, msg *model.PrivateMessage) error
	PublishGroupMsg(ctx context.Context, msg *model.GroupMessage) error
}

// ──────────────────────────────────────────────────────
// MQRepoImpl — concrete implementation using amqp091-go
// ──────────────────────────────────────────────────────

type MQRepoImpl struct {
	ch *amqp.Channel
}

func NewMQRepo(ch *amqp.Channel) *MQRepoImpl {
	return &MQRepoImpl{ch: ch}
}

const mqPublishTimeout = 5 * time.Second

func (m *MQRepoImpl) PublishPrivateMsg(ctx context.Context, msg *model.PrivateMessage) error {
	if m.ch == nil {
		return fmt.Errorf("amqp channel is nil")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal private message: %w", err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, mqPublishTimeout)
	defer cancel()
	return m.ch.PublishWithContext(
		publishCtx,
		"",                    // exchange (default)
		"private_msg_persist", // routing key = queue name
		false,                 // mandatory
		false,                 // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: 2, // persistent
		},
	)
}

func (m *MQRepoImpl) PublishGroupMsg(ctx context.Context, msg *model.GroupMessage) error {
	if m.ch == nil {
		return fmt.Errorf("amqp channel is nil")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal group message: %w", err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, mqPublishTimeout)
	defer cancel()
	return m.ch.PublishWithContext(
		publishCtx,
		"",                  // exchange (default)
		"group_msg_fanout",  // routing key = queue name
		false,               // mandatory
		false,               // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: 2, // persistent
		},
	)
}
