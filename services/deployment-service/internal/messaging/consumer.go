package messaging

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	channel *amqp.Channel
}

func NewConsumer(conn *amqp.Connection) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open consumer channel: %w", err)
	}

	return &Consumer{
		channel: ch,
	}, nil
}

func (c *Consumer) Consume() (<-chan amqp.Delivery, error) {
	deliveries, err := c.channel.Consume(
		DeploymentQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("consume deployment queue: %w", err)
	}

	return deliveries, nil
}

func (c *Consumer) Close() error {
	return c.channel.Close()
}
