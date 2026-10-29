package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"errors"
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

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	slog.Info("Project Service starting", "port", 8090)

	return http.ListenAndServe(":8090", mux)
}
