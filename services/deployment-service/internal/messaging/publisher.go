package messaging

import (
	"context"
	"fmt"

	"example.com/trest/services/deployment-service/internal/outbox"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	channel *amqp.Channel
	returns <-chan amqp.Return
}

func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	if err := DeclareTopology(ch); err != nil {
		ch.Close()
		return nil, err
	}

	if err := ch.Confirm(false); err != nil {
		ch.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}

	returns := ch.NotifyReturn(make(chan amqp.Return, 1))

	return &Publisher{
		channel: ch,
		returns: returns,
	}, nil
}

func (p *Publisher) Close() error {
	return p.channel.Close()
}

func (p *Publisher) Publish(ctx context.Context,
	message outbox.Message) error {
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		DeploymentExchange,
		DeploymentRoutingKey,
		true,
		false,
		amqp.Publishing{
			MessageId:    message.ID.String(),
			Type:         message.Type,
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         message.Payload,
		},
	)
	if err != nil {
		return fmt.Errorf("publish outbox message: %w", err)
	}

	if confirmation == nil {
		return fmt.Errorf("publisher confirms not enabled")
	}

	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for publisher confirm: %w", err)
	}

	select {
	case returned := <-p.returns:
		return fmt.Errorf("message %s unroutable: %s",
			message.ID,
			returned.ReplyText)
	default:
	}

	if !acked {
		return fmt.Errorf("rabbitmq rejected outbox message %s", message.ID)
	}

	return nil
}
