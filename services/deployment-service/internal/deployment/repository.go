package deployment

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

func (r *Repository) Create(ctx context.Context, data CreateDeploymentParams) (Deployment, error) {

	const query = `
		INSERT INTO deployments (request_id, project_id, application_id, image, 
			replicas, container_port, cpu_request_m, cpu_limit_m, memory_request_mi, 
			memory_limit_mi, environment
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (request_id) DO NOTHING
		RETURNING id, request_id, project_id, application_id, image, replicas, container_port,
				  cpu_request_m, cpu_limit_m, memory_request_mi, memory_limit_mi,
				  environment, status, created_at, updated_at
		`

	var deployment Deployment

	err := r.db.QueryRow(ctx, query,
		data.RequestID,
		data.ProjectID,
		data.ApplicationID,
		data.Image,
		data.Replicas,
		data.ContainerPort,
		data.CPURequestM,
		data.CPULimitM,
		data.MemoryRequestMi,
		data.MemoryLimitMi,
		data.Environment,
	).Scan(
		&deployment.ID,
		&deployment.RequestID,
		&deployment.ProjectID,
		&deployment.ApplicationID,
		&deployment.Image,
		&deployment.Replicas,
		&deployment.ContainerPort,
		&deployment.CPURequestM,
		&deployment.CPULimitM,
		&deployment.MemoryRequestMi,
		&deployment.MemoryLimitMi,
		&deployment.Environment,
		&deployment.Status,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrAlreadyExists
	}

	if err != nil {
		return Deployment{}, fmt.Errorf("insert deployment: %w", err)
	}

	return deployment, nil
}

func (r *Repository) GetByRequestID(ctx context.Context, requestID uuid.UUID) (Deployment, error) {

	const query = `
		SELECT id, request_id, project_id, application_id, image, replicas, container_port,
				  cpu_request_m, cpu_limit_m, memory_request_mi, memory_limit_mi,
				  environment, status, created_at, updated_at
		FROM deployments
		WHERE request_id = $1
	`

	var deployment Deployment

	err := r.db.QueryRow(ctx, query, requestID).Scan(
		&deployment.ID,
		&deployment.RequestID,
		&deployment.ProjectID,
		&deployment.ApplicationID,
		&deployment.Image,
		&deployment.Replicas,
		&deployment.ContainerPort,
		&deployment.CPURequestM,
		&deployment.CPULimitM,
		&deployment.MemoryRequestMi,
		&deployment.MemoryLimitMi,
		&deployment.Environment,
		&deployment.Status,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Deployment{}, ErrNotFound
		}

		return Deployment{}, fmt.Errorf("failed to get deployment by request id: %w", err)
	}

	return deployment, nil
}
