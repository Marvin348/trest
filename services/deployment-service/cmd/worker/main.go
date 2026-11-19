package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/messaging"
	"example.com/trest/services/deployment-service/internal/platform/postgres"
	"example.com/trest/services/deployment-service/internal/worker"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := postgres.NewPool(context.Background(), databaseURL)

	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatal("RABBITMQ_URL is required")
	}

	conn, err := messaging.Connect(rabbitMQURL)
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	log.Println("connected to RabbitMQ")

	consumer, err := messaging.NewConsumer(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.Close()

	retryPublisher, err := messaging.NewRetryPublisher(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer retryPublisher.Close()

	eventRepo := eventstore.NewRepository(pool)
	deploymentRepo := deployment.NewRepository(pool)

	service := worker.NewService(
		eventRepo,
		deploymentRepo,
		pool,
	)

	runner := worker.NewRunner(
		consumer,
		retryPublisher,
		service,
	)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := runner.Run(ctx); err != nil &&
		!errors.Is(err, context.Canceled) {
		log.Printf("worker stopped: %v", err)
	}
}
