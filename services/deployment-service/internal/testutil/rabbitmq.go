package testutil

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func StartRabbitMQ(ctx context.Context) (string, func(), error) {
	container, err := rabbitmq.Run(
		ctx,
		"rabbitmq:4.3.6-management",
		rabbitmq.WithAdminUsername("test"),
		rabbitmq.WithAdminPassword("test"),
	)

	if err != nil {
		return "", nil, fmt.Errorf("start rabbitmq container: %w", err)
	}

	cleanup := func() {
		_ = testcontainers.TerminateContainer(container)
	}

	url, err := container.AmqpURL(ctx)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("get rabbitmq URL: %w", err)
	}

	return url, cleanup, nil
}
