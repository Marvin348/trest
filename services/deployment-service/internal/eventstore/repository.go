package eventstore

import (
	"context"
	"errors"
	"fmt"
	"uuid"

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

func (r *Repository) GetByDeploymentID(ctx context.Context,
	deploymentID uuid.UUID) ([]Event, error) {
	const query = `
			SELECT *
			FROM deployment_events
			WHERE deployment_id = $1
			ORDER BY version ASC
		`

	rows, err := r.db.Query(ctx, query, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events by deployment id: %w", err)
	}
	defer rows.Close()

	var events []Event

	for rows.Next() {
		var event Event

		err := rows.Scan(
			&event.ID,
			&event.DeploymentID,
			&event.Version,
			&event.Type,
			&event.SchemaVersion,
			&event.Payload,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over event rows: %w", err)
	}

	return events, nil
}

func (r *Repository) GetByDeploymentIDs(ctx context.Context,
	deploymentIDs []uuid.UUID) (map[uuid.UUID][]Event, error) {
	if len(deploymentIDs) == 0 {
		return map[uuid.UUID][]Event{}, nil
	}

	const query = `
		SELECT *
		FROM deployment_events
		WHERE deployment_id = ANY($1)
		ORDER BY deployment_id, version ASC
		`

	rows, err := r.db.Query(ctx, query, deploymentIDs)
	if err != nil {
		return nil, fmt.Errorf("query events by deployment ids: %w", err)
	}
	defer rows.Close()

	eventsByDeployment := make(map[uuid.UUID][]Event, len(deploymentIDs))
	for rows.Next() {
		var ev Event

		if err := rows.Scan(
			&ev.ID,
			&ev.DeploymentID,
			&ev.Version,
			&ev.Type,
			&ev.SchemaVersion,
			&ev.Payload,
			&ev.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		eventsByDeployment[ev.DeploymentID] = append(eventsByDeployment[ev.DeploymentID], ev)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event rows: %w", err)
	}

	return eventsByDeployment, nil
}
