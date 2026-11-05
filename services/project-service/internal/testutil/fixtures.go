package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func CreateTestPlan(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var id string

	err := pool.QueryRow(
		context.Background(),
		`INSERT INTO plans (
            name,
            cpu_request_m,
            cpu_limit_m,
            memory_request_mi,
            memory_limit_mi,
            replicas
        )
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id`,
		"Test Plan",
		100,
		200,
		128,
		256,
		1,
	).Scan(&id)

	if err != nil {
		t.Fatalf("create test plan: %v", err)
	}

	return id
}

func CreateTestProject(
	t *testing.T,
	pool *pgxpool.Pool,
) string {
	t.Helper()

	var id string

	err := pool.QueryRow(
		context.Background(),
		`INSERT INTO projects (name)
         VALUES ($1)
         RETURNING id`,
		"Test Project",
	).Scan(&id)

	require.NoError(t, err)

	return id
}

func CreateTestApplication(
	t *testing.T,
	pool *pgxpool.Pool,
	projectID string,
	planID string,
) string {
	t.Helper()

	var id string

	err := pool.QueryRow(
		context.Background(),
		`INSERT INTO applications (
			project_id,
			plan_id,
			name,
			image,
			container_port,
			cpu_request_m,
			cpu_limit_m,
			memory_request_mi,
			memory_limit_mi,
			replicas
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		projectID,
		planID,
		"Test Application",
		"nginx:1.27",
		8080,
		100,
		250,
		256,
		512,
		3,
	).Scan(&id)

	require.NoError(t, err)

	return id
}
