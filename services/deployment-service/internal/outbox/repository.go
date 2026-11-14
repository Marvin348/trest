package outbox

import (
	"context"
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

func (r *Repository) Insert(ctx context.Context,
	tx pgx.Tx, message Message) error {

	const query = `
			INSERT INTO outbox_messages (type, payload, status, attempts)
			VALUES ($1, $2, $3, $4)
		`

	_, err := tx.Exec(ctx, query,
		message.Type,
		message.Payload,
		message.Status,
		message.Attempts)

	if err != nil {
		return fmt.Errorf("failed to insert outbox message: %w", err)
	}

	return nil
}

func (r *Repository) Notify(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx,
		"SELECT pg_notify('trest_outbox', '')",
	)
	if err != nil {
		return fmt.Errorf("notify outbox: %w", err)
	}

	return nil
}

func (r *Repository) GetPending(ctx context.Context) ([]Message, error) {

	const query = `
		SELECT id, type, payload, status, attempts, 
			created_at, published_at
		FROM outbox_messages
		WHERE status = 'pending'
		AND type = 'deployment.requested'
		ORDER BY created_at
		LIMIT 100;
		`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get pending messages: %w", err)
	}
	defer rows.Close()

	var messages []Message

	for rows.Next() {
		var message Message

		err := rows.Scan(
			&message.ID,
			&message.Type,
			&message.Payload,
			&message.Status,
			&message.Attempts,
			&message.CreatedAt,
			&message.PublishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan outbox message: %w", err)
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox messages: %w", err)
	}

	return messages, nil
}

func (r *Repository) MarkPublished(ctx context.Context,
	messageID uuid.UUID) error {

	const query = `
		UPDATE outbox_messages
		SET status = 'published',
			published_at = NOW()
		WHERE id = $1
		AND status = 'pending'
	`

	tag, err := r.db.Exec(ctx, query, messageID)
	if err != nil {
		return fmt.Errorf("mark outbox message published: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox message %s was not marked published", messageID)
	}

	return nil
}
