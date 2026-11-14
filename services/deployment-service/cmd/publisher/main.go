package main

import (
	"context"
	"errors"
	"log"
	"os"

	"example.com/trest/services/deployment-service/internal/messaging"
	"example.com/trest/services/deployment-service/internal/outbox"
	"example.com/trest/services/deployment-service/internal/platform/postgres"
	"example.com/trest/services/deployment-service/internal/publishing"
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

	publisher, err := messaging.NewPublisher(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer publisher.Close()

	log.Println("RabbitMQ channel opened")

	ctx := context.Background()

	outboxRepo := outbox.NewRepository(pool)

	publishingService := publishing.NewService(
		outboxRepo,
		publisher,
	)

	if err := publishingService.Run(ctx, databaseURL); err != nil {
		log.Printf("publisher stopped: %v", err)
	}

}
