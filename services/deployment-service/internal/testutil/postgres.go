package testutil

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func StartPostgres(ctx context.Context) (*pgxpool.Pool, func(), error) {
	container, err := tcpostgres.Run(
		ctx,
		"postgres:17",
		tcpostgres.WithDatabase("projects_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start postgres container: %w", err)
	}

	cleanupContainer := func() {
		_ = testcontainers.TerminateContainer(container)
	}

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cleanupContainer()
		return nil, nil, fmt.Errorf("get database URL: %w", err)
	}

	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(
		filepath.Dir(filename), "..", "..", "migrations",
	)

	migrator, err := migrate.New(
		"file://"+filepath.ToSlash(migrationsPath),
		databaseURL,
	)
	if err != nil {
		cleanupContainer()
		return nil, nil, fmt.Errorf("initialize migrations: %w", err)
	}

	migrationErr := migrator.Up()
	_, _ = migrator.Close()

	if migrationErr != nil && migrationErr != migrate.ErrNoChange {
		cleanupContainer()
		return nil, nil, fmt.Errorf("apply migrations: %w", migrationErr)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		cleanupContainer()
		return nil, nil, fmt.Errorf("create database pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cleanupContainer()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}

	cleanup := func() {
		pool.Close()
		cleanupContainer()
	}

	return pool, cleanup, nil
}
