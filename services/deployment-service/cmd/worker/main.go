package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/kubernetes"
	"example.com/trest/services/deployment-service/internal/messaging"
	platformcrypto "example.com/trest/services/deployment-service/internal/platform/crypto"
	"example.com/trest/services/deployment-service/internal/platform/postgres"
	"example.com/trest/services/deployment-service/internal/worker"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("worker stopped: %v", err)
	}
}

func run(ctx context.Context) error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	pool, err := postgres.NewPool(ctx, databaseURL)

	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		return errors.New("RABBITMQ_URL is required")
	}

	keyProvider, err := platformcrypto.NewEnvironmentKeyProvider()
	if err != nil {
		return fmt.Errorf("load deployment encryption keys: %w", err)
	}

	kubernetesClient, err := kubernetes.NewClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		return err
	}

	namespace := os.Getenv("KUBERNETES_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	executor := kubernetes.NewExecutor(kubernetesClient, namespace, keyProvider)

	conn, err := messaging.Connect(rabbitMQURL)
	if err != nil {
		return err
	}

	defer conn.Close()

	log.Println("connected to RabbitMQ")

	consumer, err := messaging.NewConsumer(conn)
	if err != nil {
		return err
	}
	defer consumer.Close()

	retryPublisher, err := messaging.NewRetryPublisher(conn)
	if err != nil {
		return err
	}
	defer retryPublisher.Close()

	eventRepo := eventstore.NewRepository(pool)
	deploymentRepo := deployment.NewRepository(pool)

	service := worker.NewService(
		eventRepo,
		deploymentRepo,
		pool,
		executor,
	)

	runner := worker.NewRunner(
		consumer,
		retryPublisher,
		service,
	)

	group, workerCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return service.Run(workerCtx, 30*time.Second) })
	group.Go(func() error { return runner.Run(workerCtx) })
	return group.Wait()
}
