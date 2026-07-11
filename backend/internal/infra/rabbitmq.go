package infra

import (
	"github.com/goim/goim/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueNames 列出 IM 系统所需的所有持久队列。
var QueueNames = []string{
	"private_msg_persist",
	"group_msg_fanout",
	"moment_push",
	"like_persist",
	"comment_persist",
}

// NewRabbitMQConn 建立 RabbitMQ 连接并打开一个通道。
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

// DeclareQueues 在给定通道上声明所有 5 个持久队列。
func DeclareQueues(ch *amqp.Channel) error {
	for _, name := range QueueNames {
		_, err := ch.QueueDeclare(name, true, false, false, false, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
