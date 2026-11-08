package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/idempotency"

	"example.com/trest/services/deployment-service/internal/platform/postgres"
	"github.com/joho/godotenv"
)

func Run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is missing")
	}

	pool, err := postgres.NewPool(context.Background(), databaseURL)

	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	idempotencyRepository := idempotency.NewRepository(pool)
	repository := deployment.NewRepository(pool)
	eventRepository := eventstore.NewRepository(pool)

	service := deployment.NewService(
		pool,
		repository,
		eventRepository,
		idempotencyRepository,
	)

	return runServers(service)
}
