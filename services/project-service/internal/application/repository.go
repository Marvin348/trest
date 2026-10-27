package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, data CreateApplicationData) (Application, error) {

	const query = `
		INSERT INTO applications (
			project_id, plan_id, name, image, container_port, cpu_request_m, cpu_limit_m, 
			memory_request_mi, memory_limit_mi, replicas
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, project_id, plan_id, name, image, container_port, cpu_request_m, cpu_limit_m, 
		          memory_request_mi, memory_limit_mi, replicas, created_at, updated_at
		`

	var application Application

	err := r.db.QueryRow(ctx, query, data.ProjectID,
		data.PlanID,
		data.Name,
		data.Image,
		data.ContainerPort,
		data.CPURequestM,
		data.CPULimitM,
		data.MemoryRequestMi,
		data.MemoryLimitMi,
		data.Replicas).Scan(
		&application.ID,
		&application.ProjectID,
		&application.PlanID,
		&application.Name,
		&application.Image,
		&application.ContainerPort,
		&application.CPURequestM,
		&application.CPULimitM,
		&application.MemoryRequestMi,
		&application.MemoryLimitMi,
		&application.Replicas,
		&application.CreatedAt,
		&application.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) {
			switch {
			case pgErr.Code == "23505" &&
				pgErr.ConstraintName == "uq_application_project_name":
				return Application{}, ErrDuplicateApplication

			case pgErr.Code == "23503" &&
				pgErr.ConstraintName == "fk_application_project":
				return Application{}, ErrProjectNotFound

			case pgErr.Code == "23503" &&
				pgErr.ConstraintName == "fk_application_plan":
				return Application{}, ErrPlanNotFound
			}
		}

		return Application{}, fmt.Errorf("insert application: %w", err)
	}

	return application, nil
}
