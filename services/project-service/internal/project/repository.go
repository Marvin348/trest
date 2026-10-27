package project

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, name string) (Project, error) {
	var project Project

	err := r.db.QueryRow(ctx,
		`INSERT INTO projects (name)
	     VALUES ($1)
	     RETURNING id, name, created_at, updated_at`,
		name,
	).Scan(
		&project.ID,
		&project.Name,
		&project.CreatedAt,
		&project.UpdatedAt,
	)

	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}

	return project, nil
}

func (r *Repository) ExistsById(ctx context.Context, id string) (bool, error) {
	var exists bool

	err := r.db.QueryRow(ctx, 
	`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, id).Scan(&exists)

	if err != nil {
		return false, err
	}

	return exists, nil
}
