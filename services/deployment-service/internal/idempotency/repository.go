package idempotency

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Claim(ctx context.Context,
	tx pgx.Tx, request ProcessedRequest) error {

	const query = `
			INSERT INTO processed_requests(
				operation,
				request_id,
				deployment_id,
				fingerprint
			)
			VALUES($1, $2, $3, $4)
			ON CONFLICT (operation, request_id) DO NOTHING
		`

	tag, err := tx.Exec(ctx, query,
		request.Operation,
		request.RequestID,
		request.DeploymentID,
		request.Fingerprint,
	)

	if err != nil {
		return fmt.Errorf("reserve idempotency key: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrAlreadyProcessed
	}

	return nil
}

func (r *Repository) GetByRequestID(
	ctx context.Context,
	operation string,
	requestID uuid.UUID) (ProcessedRequest, error) {

	query := `
			SELECT operation, request_id, deployment_id, fingerprint
			FROM processed_requests
			WHERE operation = $1 AND request_id = $2
		`

	var processedRequest ProcessedRequest

	err := r.db.QueryRow(ctx, query, operation, requestID).Scan(
		&processedRequest.Operation,
		&processedRequest.RequestID,
		&processedRequest.DeploymentID,
		&processedRequest.Fingerprint,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProcessedRequest{}, ErrNotFound
		}

		return ProcessedRequest{},
			fmt.Errorf("failed to get processed request: %w", err)
	}

	return processedRequest, nil
}
