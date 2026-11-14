package messaging

import (
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	DeploymentExchange   = "trest.deployments"
	DeploymentQueue      = "trest.deployments.requests"
	DeploymentRoutingKey = "deployment.requested"

	DeploymentDLX          = "trest.deployments.dlx"
	DeploymentDLQ          = "trest.deployments.dlq"
	DeploymentDLRoutingKey = "deployment.dead"
)

func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		DeploymentExchange, "direct",
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare deployment exchange: %w", err)
	}

	if err := ch.ExchangeDeclare(DeploymentDLX, "direct",
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare deployment dlx exchange: %w", err)
	}

	if _, err := ch.QueueDeclare(
		DeploymentDLQ,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare dead letter queue: %w", err)
	}

	if err := ch.QueueBind(
		DeploymentDLQ,
		DeploymentDLRoutingKey,
		DeploymentDLX,
		false, nil,
	); err != nil {
		return fmt.Errorf("bind dead letter queue: %w", err)
	}

	if _, err := ch.QueueDeclare(
		DeploymentQueue,
		true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    DeploymentDLX,
			"x-dead-letter-routing-key": DeploymentDLRoutingKey,
		},
	); err != nil {
		return fmt.Errorf("declare deployment queue: %w", err)
	}

	if err := ch.QueueBind(
		DeploymentQueue,
		DeploymentRoutingKey,
		DeploymentExchange,
		false, nil,
	); err != nil {
		return fmt.Errorf("bind deployment queue: %w", err)
	}

	return nil
}
