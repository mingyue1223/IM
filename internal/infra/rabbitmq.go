package infra

import (
	"github.com/goim/goim/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueNames lists all durable queues the IM system needs.
var QueueNames = []string{
	"private_msg_persist",
	"group_msg_fanout",
	"moment_push",
	"like_persist",
	"comment_persist",
	"ai_summary_persist",
	"ai_profile_persist",
}

// NewRabbitMQConn establishes a RabbitMQ connection and opens a channel.
func NewRabbitMQConn(cfg *config.RabbitMQConfig) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	return conn, ch, nil
}

// DeclareQueues declares all 7 durable queues on the given channel.
func DeclareQueues(ch *amqp.Channel) error {
	for _, name := range QueueNames {
		_, err := ch.QueueDeclare(name, true, false, false, false, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
