package outbox

import (
	"context"
	"fmt"

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
