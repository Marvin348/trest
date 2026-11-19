package messaging

import (
	"context"
	"testing"
	"time"

	"example.com/trest/services/deployment-service/internal/testutil"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryPublisher(t *testing.T) {
	ctx := context.Background()

	url, cleanup, err := testutil.StartRabbitMQ(ctx)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	conn, err := Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	retryPublisher, err := NewRetryPublisher(conn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, retryPublisher.Close()) })

	ch, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ch.Close()) })

	t.Run("publishes message to retry queue", func(t *testing.T) {
		purgeRetryTestQueues(t, ch)
		delivery := newTestRetryDelivery("retry-message")

		require.NoError(t, retryPublisher.PublishRetry(ctx, delivery, 2))

		retried, ok, err := ch.Get(DeploymentRetryQueue, true)
		require.NoError(t, err)
		require.True(t, ok, "expected message in retry queue")
		assertRetryDelivery(t, delivery, retried, 2)

		assert.Equal(t, int32(1), delivery.Headers["x-retry-count"],
			"publishing must not mutate the original delivery headers")
	})

	t.Run("routes expired retry back to deployment queue", func(t *testing.T) {
		purgeRetryTestQueues(t, ch)
		delivery := newTestRetryDelivery("expired-retry-message")

		require.NoError(t, retryPublisher.PublishRetry(ctx, delivery, 3))

		_, ok, err := ch.Get(DeploymentQueue, true)
		require.NoError(t, err)
		require.False(t, ok, "retry must not reach deployment queue before TTL")

		var routed amqp.Delivery
		var getErr error

		require.Eventually(t, func() bool {
			var found bool
			routed, found, getErr = ch.Get(DeploymentQueue, true)
			return getErr == nil && found
		}, 8*time.Second, 100*time.Millisecond,
			"expected retry in deployment queue after TTL")
		require.NoError(t, getErr)
		assertRetryDelivery(t, delivery, routed, 3)
	})

	t.Run("returns error for unroutable retry", func(t *testing.T) {
		purgeRetryTestQueues(t, ch)

		require.NoError(t, ch.QueueUnbind(
			DeploymentRetryQueue,
			DeploymentRetryRoutingKey,
			DeploymentExchange,
			nil,
		))
		t.Cleanup(func() {
			require.NoError(t, ch.QueueBind(
				DeploymentRetryQueue,
				DeploymentRetryRoutingKey,
				DeploymentExchange,
				false,
				nil,
			))
		})

		delivery := newTestRetryDelivery("unroutable-retry-message")

		err := retryPublisher.PublishRetry(ctx, delivery, 2)

		require.Error(t, err)
		assert.ErrorContains(t, err,
			"retry message "+delivery.MessageId+" unroutable")
		assert.ErrorContains(t, err, "NO_ROUTE")

		_, ok, getErr := ch.Get(DeploymentRetryQueue, true)
		require.NoError(t, getErr)
		assert.False(t, ok, "unroutable retry must not reach retry queue")
	})
}

func newTestRetryDelivery(messageID string) amqp.Delivery {
	return amqp.Delivery{
		Headers: amqp.Table{
			"trace-id":      "trace-123",
			"x-retry-count": int32(1),
		},
		MessageId:    messageID,
		Type:         DeploymentRoutingKey,
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body: []byte(`{
			"image": "nginx:2.27",
			"replicas": 2,
			"project_id": "b763fd1a-c910-4b29-89ea-240104f33baf"
		}`),
	}
}

func assertRetryDelivery(
	t *testing.T,
	original amqp.Delivery,
	actual amqp.Delivery,
	retryCount int32,
) {
	t.Helper()

	assert.Equal(t, original.MessageId, actual.MessageId)
	assert.Equal(t, original.Type, actual.Type)
	assert.Equal(t, original.ContentType, actual.ContentType)
	assert.Equal(t, uint8(amqp.Persistent), actual.DeliveryMode)
	assert.Equal(t, original.Body, actual.Body)
	assert.Equal(t, "trace-123", actual.Headers["trace-id"])
	assert.Equal(t, retryCount, actual.Headers["x-retry-count"])
}

func purgeRetryTestQueues(t *testing.T, ch *amqp.Channel) {
	t.Helper()

	for _, queue := range []string{
		DeploymentQueue,
		DeploymentRetryQueue,
		DeploymentDLQ,
	} {
		_, err := ch.QueuePurge(queue, false)
		require.NoError(t, err)
	}
}
