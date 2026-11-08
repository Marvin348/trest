package eventstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Append(
	ctx context.Context,
	tx pgx.Tx,
	event Event,
) error {
	const query = `
		INSERT INTO deployment_events (
			deployment_id,
			version,
			type,
			schema_version,
			payload
		)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := tx.Exec(
		ctx,
		query,
		event.DeploymentID,
		event.Version,
		event.Type,
		event.SchemaVersion,
		event.Payload,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			pgErr.ConstraintName == "deployment_events_deployment_id_version_key" {
			return fmt.Errorf("%w: %w", ErrConcurrencyConflict, err)
		}

		return fmt.Errorf("append event: %w", err)
	}

	return nil
}
