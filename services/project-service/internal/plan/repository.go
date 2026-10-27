package plan

import (
	"context"
	"errors"
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

func (r *Repository) GetByID(ctx context.Context, id string) (Plan, error) {
	var plan Plan

	err := r.db.QueryRow(ctx, `
	SELECT id, name, cpu_request_m, cpu_limit_m,
           memory_request_mi, memory_limit_mi,
           replicas, created_at, updated_at
    FROM plans
    WHERE id = $1`, id).Scan(
		&plan.ID,
		&plan.Name,
		&plan.CPURequestM,
		&plan.CPULimitM,
		&plan.MemoryRequestMi,
		&plan.MemoryLimitMi,
		&plan.Replicas,
		&plan.CreatedAt,
		&plan.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Plan{}, ErrNotFound
		}

		return Plan{}, fmt.Errorf("get plan by ID: %w", err)
	}

	return plan, nil
}
