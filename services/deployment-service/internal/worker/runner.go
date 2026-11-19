package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Runner struct {
	consumer       *messaging.Consumer
	retryPublisher *messaging.RetryPublisher
	service        *Service
}

func NewRunner(
	consumer *messaging.Consumer,
	retryPublisher *messaging.RetryPublisher,
	service *Service,
) *Runner {
	return &Runner{
		consumer:       consumer,
		retryPublisher: retryPublisher,
		service:        service,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	deliveries, err := r.consumer.Consume()
	if err != nil {
		return err
	}

	const MaxRetries int32 = 3

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("rabbitmq deliveries channel closed")
			}

			retryCount, err := getRetryCount(delivery)
			if err != nil {
				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return nackErr
				}
				continue
			}

			var message deployment.DeploymentRequestedMessage

			if err := json.Unmarshal(delivery.Body, &message); err != nil {
				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return nackErr
				}
				continue
			}

			err = r.service.Process(ctx, message)

			if err == nil {
				if ackErr := delivery.Ack(false); ackErr != nil {
					return ackErr
				}
				continue
			}

			if errors.Is(err, deployment.ErrNoEvents) ||
				errors.Is(err, deployment.ErrInvalidTransition) {

				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return nackErr
				}
				continue
			}

			if retryCount >= MaxRetries {
				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return nackErr
				}
				continue
			}

			if err := r.retryPublisher.PublishRetry(
				ctx,
				delivery,
				retryCount+1,
			); err != nil {
				return fmt.Errorf("publish deployment retry: %w", err)
			}

			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("ack original deployment message: %w", err)
			}
		}
	}
}

func getRetryCount(delivery amqp.Delivery) (int32, error) {
	value, exists := delivery.Headers["x-retry-count"]
	if !exists {
		return 0, nil
	}

	count, ok := value.(int32)
	if !ok || count < 0 {
		return 0, fmt.Errorf("invalid retry count header: %v", value)
	}

	return count, nil
}
