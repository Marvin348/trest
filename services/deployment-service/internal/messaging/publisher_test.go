package messaging

import (
	"context"
	"testing"

	"example.com/trest/services/deployment-service/internal/outbox"
	"example.com/trest/services/deployment-service/internal/testutil"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uuid"
)

func TestPublish(t *testing.T) {
	ctx := context.Background()

	url, cleanup, err := testutil.StartRabbitMQ(ctx)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	conn, err := Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	publisher, err := NewPublisher(conn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, publisher.Close()) })

	t.Run("successful publish", func(t *testing.T) {
		message := newTestOutboxMessage()

		require.NoError(t, publisher.Publish(ctx, message))

		ch, err := conn.Channel()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, ch.Close()) })

		delivery, ok, err := ch.Get(DeploymentQueue, true)
		require.NoError(t, err)
		require.True(t, ok, "expected message in queue")

		assert.Equal(t, message.ID.String(), delivery.MessageId)
		assert.Equal(t, message.Type, delivery.Type)
		assert.Equal(t, "application/json", delivery.ContentType)
		assert.Equal(t, uint8(amqp.Persistent), delivery.DeliveryMode)
		assert.Equal(t, []byte(message.Payload), delivery.Body)
	})

	t.Run("unroutable message", func(t *testing.T) {
		ch, err := conn.Channel()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, ch.Close()) })

		require.NoError(t, ch.QueueUnbind(
			DeploymentQueue,
			DeploymentRoutingKey,
			DeploymentExchange,
			nil,
		))
		t.Cleanup(func() {
			require.NoError(t, ch.QueueBind(
				DeploymentQueue,
				DeploymentRoutingKey,
				DeploymentExchange,
				false,
				nil,
			))
		})

		message := newTestOutboxMessage()

		err = publisher.Publish(ctx, message)
		require.Error(t, err)
		assert.ErrorContains(t, err, "message "+message.ID.String()+" unroutable")
		assert.ErrorContains(t, err, "NO_ROUTE")

		_, ok, err := ch.Get(DeploymentQueue, true)
		require.NoError(t, err)
		assert.False(t, ok, "unroutable message must not reach the queue")
	})
}

func newTestOutboxMessage() outbox.Message {
	return outbox.Message{
		ID:       uuid.New(),
		Type:     DeploymentRoutingKey,
		Payload:  []byte(`{"image":"nginx:1.27","replicas":2}`),
		Status:   string(outbox.StatusPending),
		Attempts: 0,
	}
}
