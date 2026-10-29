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
