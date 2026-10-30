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


// Event-Driven Architecture: Service A wirft eine Nachricht in eine Warteschlange und arbeitet sofort weiter.
// - Apache Kafka: Der Industriestandard für Event-Streaming und extrem große Datenmengen. Services kommunizieren, indem sie "Events" (z. B. BestellungAbgeschlossen) in Kafka-Topics schreiben, die andere Services abonnieren.
// - RabbitMQ / NATS: Sehr beliebte, klassische Message Broker. NATS ist in der Go-Welt besonders beliebt, weil es extrem leichtgewichtig, in Go geschrieben und unglaublich schnell für Publish/Subscribe-Muster ist.

// Das wäre doch was für mich oder?