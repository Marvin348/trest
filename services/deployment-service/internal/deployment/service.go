package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/idempotency"
	"example.com/trest/services/deployment-service/internal/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db              *pgxpool.Pool
	repo            *Repository
	eventRepo       *eventstore.Repository
	idempotencyRepo *idempotency.Repository
	outboxRepo      *outbox.Repository
}

func NewService(
	db *pgxpool.Pool,
	repo *Repository,
	eventRepo *eventstore.Repository,
	idempotencyRepo *idempotency.Repository,
	outboxRepo *outbox.Repository,
) *Service {
	return &Service{
		db:              db,
		repo:            repo,
		eventRepo:       eventRepo,
		idempotencyRepo: idempotencyRepo,
		outboxRepo:      outboxRepo,
	}
}

func (s *Service) Create(ctx context.Context,
	params CreateDeploymentParams) (Deployment, error) {

	if err := validateCreateDeploymentParams(params); err != nil {
		return Deployment{}, err
	}

	deploymentID := uuid.New()
	payload, err := json.Marshal(newDeploymentCreatedPayload(params))
	if err != nil {
		return Deployment{}, fmt.Errorf("marshal deployment created payload: %w", err)
	}

	fingerprint, err := fingerprintCreateDeployment(params)
	if err != nil {
		return Deployment{}, fmt.Errorf("fingerprint deployment request: %w", err)
	}

	event := eventstore.Event{
		DeploymentID:  deploymentID,
		Type:          string(DeploymentCreated),
		Version:       1,
		SchemaVersion: 1,
		Payload:       payload,
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Deployment{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	err = s.idempotencyRepo.Claim(ctx, tx, idempotency.ProcessedRequest{
		Operation:    "deployment.create",
		RequestID:    params.RequestID,
		DeploymentID: deploymentID,
		Fingerprint:  fingerprint,
	})

	if errors.Is(err, idempotency.ErrAlreadyProcessed) {
		if err := tx.Rollback(ctx); err != nil {
			return Deployment{}, fmt.Errorf("rollback duplicate deployment: %w", err)
		}

		existing, err := s.idempotencyRepo.GetByRequestID(ctx,
			"deployment.create", params.RequestID)

		if err != nil {
			return Deployment{}, fmt.Errorf("get existing request: %w", err)
		}

		if !bytes.Equal(existing.Fingerprint, fingerprint) {
			return Deployment{}, ErrIdempotencyConflict
		}

		deployment, err := s.repo.GetByID(ctx, existing.DeploymentID)
		if err != nil {
			return Deployment{}, fmt.Errorf("get existing deployment: %w", err)
		}

		return deployment, nil
	}

	if err != nil {
		return Deployment{}, fmt.Errorf("claim idempotency key: %w", err)
	}

	if err := s.eventRepo.Append(ctx, tx, event); err != nil {
		return Deployment{}, err
	}

	created, err := s.repo.CreateWithTx(ctx, tx, params, deploymentID)

	if err != nil {
		return Deployment{}, fmt.Errorf("create deployment: %w", err)
	}

	messagePayload, err := json.Marshal(
		newDeploymentRequestedMessage(params, deploymentID),
	)
	if err != nil {
		return Deployment{}, fmt.Errorf("marshal deployment requested message: %w", err)
	}

	err = s.outboxRepo.Insert(ctx, tx, outbox.Message{
		Type:     "deployment.requested",
		Payload:  messagePayload,
		Status:   string(outbox.StatusPending),
		Attempts: 0,
	})
	if err != nil {
		return Deployment{}, fmt.Errorf("insert outbox message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, fmt.Errorf("commit transaction: %w", err)
	}

	return created, nil
}
