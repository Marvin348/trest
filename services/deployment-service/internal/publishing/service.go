package publishing

import (
	"context"
	"log"
	"time"

	"example.com/trest/services/deployment-service/internal/messaging"
	"example.com/trest/services/deployment-service/internal/outbox"
)

type Service struct {
	outboxRepo *outbox.Repository
	publisher  *messaging.Publisher
}

func NewService(
	outboxRepo *outbox.Repository,
	publisher *messaging.Publisher,
) *Service {
	return &Service{
		outboxRepo: outboxRepo,
		publisher:  publisher,
	}
}

func (s *Service) ProcessPending(ctx context.Context) error {
	messages, err := s.outboxRepo.GetPending(ctx)
	if err != nil {
		return err
	}

	for _, message := range messages {
		if err := s.publisher.Publish(ctx, message); err != nil {
			return err
		}

		if err := s.outboxRepo.MarkPublished(ctx, message.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) Run(ctx context.Context, databaseURL string) error {
	wake := make(chan struct{}, 1)

	go ListenOutbox(ctx, databaseURL, wake)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	if err := s.ProcessPending(ctx); err != nil {
		log.Printf("process pending: %v", err)
	}

	for {
		select {
		case <-wake:

		case <-ticker.C:

		case <-ctx.Done():
			return ctx.Err()

		}

		if err := s.ProcessPending(ctx); err != nil {
			log.Printf("process pending: %v", err)
		}
	}
}
