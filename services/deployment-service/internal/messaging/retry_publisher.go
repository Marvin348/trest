package messaging

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RetryPublisher struct {
	channel *amqp.Channel
	returns <-chan amqp.Return
	mu      sync.Mutex
}

func NewRetryPublisher(conn *amqp.Connection) (*RetryPublisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open retry publisher channel: %w", err)
	}

	if err := DeclareTopology(ch); err != nil {
		ch.Close()
		return nil, err
	}

	if err := ch.Confirm(false); err != nil {
		ch.Close()
		return nil, fmt.Errorf("enable retry publisher confirms: %w", err)
	}

	returns := ch.NotifyReturn(make(chan amqp.Return, 1))

	return &RetryPublisher{
		channel: ch,
		returns: returns,
	}, nil
}

func (p *RetryPublisher) Close() error {
	return p.channel.Close()
}

func (p *RetryPublisher) PublishRetry(
	ctx context.Context,
	delivery amqp.Delivery,
	retryCount int32,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	headers := make(amqp.Table, len(delivery.Headers)+1)
	for key, value := range delivery.Headers {
		headers[key] = value
	}
	headers["x-retry-count"] = retryCount

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		DeploymentExchange,
		DeploymentRetryRoutingKey,
		true,
		false,
		amqp.Publishing{
			Headers:      headers,
			MessageId:    delivery.MessageId,
			Type:         delivery.Type,
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         delivery.Body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish retry message: %w", err)
	}

	if confirmation == nil {
		return fmt.Errorf("retry publisher confirms not enabled")
	}

	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for retry publisher confirm: %w", err)
	}

	select {
	case returned := <-p.returns:
		return fmt.Errorf(
			"retry message %s unroutable: %s",
			delivery.MessageId,
			returned.ReplyText,
		)
	default:
	}

	if !acked {
		return fmt.Errorf(
			"rabbitmq rejected retry message %s",
			delivery.MessageId,
		)
	}

	return nil
}
