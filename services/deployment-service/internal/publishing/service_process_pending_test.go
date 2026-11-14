package publishing

import (
	"context"
	"testing"
	"time"

	"example.com/trest/services/deployment-service/internal/messaging"
	"example.com/trest/services/deployment-service/internal/outbox"
	"example.com/trest/services/deployment-service/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uuid"
)

func TestProcessPending(t *testing.T) {
	ctx := context.Background()

	pool, cleanupPostgres, err := testutil.StartPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(cleanupPostgres)

	rabbitURL, cleanupRabbit, err := testutil.StartRabbitMQ(ctx)
	require.NoError(t, err)
	t.Cleanup(cleanupRabbit)

	conn, err := messaging.Connect(rabbitURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	publisher, err := messaging.NewPublisher(conn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, publisher.Close()) })

	outboxRepo := outbox.NewRepository(pool)
	service := NewService(outboxRepo, publisher)

	t.Run("gets pending outbox messages and publishes them", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		createdAt := time.Date(2026, time.November, 13, 10, 0, 0, 0, time.UTC)
		messages := []outbox.Message{
			newTestOutboxMessage(),
			newTestOutboxMessage(func(message *outbox.Message) {
				message.Status = string(outbox.StatusPublished)
			}),
			newTestOutboxMessage(func(message *outbox.Message) {
				message.Attempts = 4
			}),
			newTestOutboxMessage(func(message *outbox.Message) {
				message.Status = string(outbox.StatusPublished)
			}),
			newTestOutboxMessage(),
		}

		for i := range messages {
			messages[i].CreatedAt = createdAt.Add(time.Duration(i) * time.Second)
			insertTestOutboxMessage(t, ctx, pool, messages[i])
		}

		require.NoError(t, service.ProcessPending(ctx))

		ch, err := conn.Channel()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, ch.Close()) })

		expectedPublished := []outbox.Message{
			messages[0],
			messages[2],
			messages[4],
		}
		for _, expected := range expectedPublished {
			delivery, ok, err := ch.Get(messaging.DeploymentQueue, true)
			require.NoError(t, err)
			require.True(t, ok, "expected outbox message %s in queue", expected.ID)

			assert.Equal(t, expected.ID.String(), delivery.MessageId)
			assert.Equal(t, expected.Type, delivery.Type)
			assert.JSONEq(t, string(expected.Payload), string(delivery.Body))

			requireOutboxMessagePublished(t, ctx, pool, expected)
		}

		_, ok, err := ch.Get(messaging.DeploymentQueue, true)
		require.NoError(t, err)
		assert.False(t, ok, "only pending outbox messages should be published")
		assert.Zero(t, outboxMessagePendingCount(t, ctx, pool))
	})

	t.Run("does nothing when there are no pending messages", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		require.NoError(t, service.ProcessPending(ctx))
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("ignores pending messages of other types", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		message := newTestOutboxMessage(func(message *outbox.Message) {
			message.Type = "deployment.succeeded"
			message.CreatedAt = time.Date(
				2026, time.November, 13, 10, 30, 0, 0, time.UTC,
			)
		})
		insertTestOutboxMessage(t, ctx, pool, message)

		require.NoError(t, service.ProcessPending(ctx))
		requireDeploymentQueueEmpty(t, conn)
		assert.Equal(t, 1, outboxMessagePendingCount(t, ctx, pool))
		requireOutboxMessagePending(t, ctx, pool, message)
	})

	t.Run("returns an error when loading pending messages fails", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := service.ProcessPending(canceledCtx)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("returns an error and stops when publishing fails", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		createdAt := time.Date(2026, time.November, 13, 11, 0, 0, 0, time.UTC)
		messages := []outbox.Message{
			newTestOutboxMessage(func(message *outbox.Message) {
				message.CreatedAt = createdAt
			}),
			newTestOutboxMessage(func(message *outbox.Message) {
				message.CreatedAt = createdAt.Add(time.Second)
			}),
		}
		for _, message := range messages {
			insertTestOutboxMessage(t, ctx, pool, message)
		}

		ch, err := conn.Channel()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, ch.Close()) })
		require.NoError(t, ch.QueueUnbind(
			messaging.DeploymentQueue,
			messaging.DeploymentRoutingKey,
			messaging.DeploymentExchange,
			nil,
		))
		t.Cleanup(func() {
			require.NoError(t, ch.QueueBind(
				messaging.DeploymentQueue,
				messaging.DeploymentRoutingKey,
				messaging.DeploymentExchange,
				false,
				nil,
			))
		})

		err = service.ProcessPending(ctx)
		require.Error(t, err)
		assert.ErrorContains(t, err, "message "+messages[0].ID.String()+" unroutable")
		assert.ErrorContains(t, err, "NO_ROUTE")
		assert.Equal(t, 2, outboxMessagePendingCount(t, ctx, pool))
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("returns an error when marking a message published fails", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		createdAt := time.Date(2026, time.November, 13, 12, 0, 0, 0, time.UTC)
		messages := []outbox.Message{
			newTestOutboxMessage(func(message *outbox.Message) {
				message.CreatedAt = createdAt
			}),
			newTestOutboxMessage(func(message *outbox.Message) {
				message.CreatedAt = createdAt.Add(time.Second)
			}),
		}
		for _, message := range messages {
			insertTestOutboxMessage(t, ctx, pool, message)
		}

		installSuppressOutboxUpdateTrigger(t, ctx, pool)

		err := service.ProcessPending(ctx)
		require.Error(t, err)
		assert.ErrorContains(
			t,
			err,
			"outbox message "+messages[0].ID.String()+" was not marked published",
		)
		assert.Equal(t, 2, outboxMessagePendingCount(t, ctx, pool))

		delivery := requireDeploymentDelivery(t, conn)
		assert.Equal(t, messages[0].ID.String(), delivery.MessageId)
		assert.JSONEq(t, string(messages[0].Payload), string(delivery.Body))
		requireDeploymentQueueEmpty(t, conn)
	})
}

func newTestOutboxMessage(mods ...func(*outbox.Message)) outbox.Message {
	message := outbox.Message{
		ID:       uuid.New(),
		Type:     messaging.DeploymentRoutingKey,
		Payload:  []byte(`{"image":"nginx:1.27","replicas":2}`),
		Status:   string(outbox.StatusPending),
		Attempts: 0,
	}
	for _, mod := range mods {
		mod(&message)
	}
	return message
}

func insertTestOutboxMessage(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	message outbox.Message,
) {
	t.Helper()

	_, err := db.Exec(ctx, `
		INSERT INTO outbox_messages (
			id, type, payload, status, attempts, created_at, published_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
		message.ID,
		message.Type,
		message.Payload,
		message.Status,
		message.Attempts,
		message.CreatedAt,
		message.PublishedAt,
	)
	require.NoError(t, err)
}

func requireOutboxMessagePublished(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	expected outbox.Message,
) {
	t.Helper()

	var status string
	var attempts int
	var publishedAt *time.Time
	err := db.QueryRow(ctx, `
		SELECT status, attempts, published_at
		FROM outbox_messages
		WHERE id = $1
		`, expected.ID).Scan(&status, &attempts, &publishedAt)
	require.NoError(t, err)
	assert.Equal(t, string(outbox.StatusPublished), status)
	assert.Equal(t, expected.Attempts, attempts)
	assert.NotNil(t, publishedAt)
}

func requireOutboxMessagePending(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	expected outbox.Message,
) {
	t.Helper()

	var status string
	var attempts int
	var publishedAt *time.Time
	err := db.QueryRow(ctx, `
		SELECT status, attempts, published_at
		FROM outbox_messages
		WHERE id = $1
		`, expected.ID).Scan(&status, &attempts, &publishedAt)
	require.NoError(t, err)
	assert.Equal(t, string(outbox.StatusPending), status)
	assert.Equal(t, expected.Attempts, attempts)
	assert.Nil(t, publishedAt)
}

func outboxMessagePendingCount(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
) int {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_messages
		WHERE status = 'pending'
		`).Scan(&count))

	return count
}

func resetPublishingState(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	conn *amqp.Connection,
) {
	t.Helper()

	testutil.ResetDB(t, db)

	ch, err := conn.Channel()
	require.NoError(t, err)
	_, err = ch.QueuePurge(messaging.DeploymentQueue, false)
	require.NoError(t, err)
	require.NoError(t, ch.Close())
}

func requireDeploymentDelivery(
	t *testing.T,
	conn *amqp.Connection,
) amqp.Delivery {
	t.Helper()

	ch, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ch.Close()) })

	delivery, ok, err := ch.Get(messaging.DeploymentQueue, true)
	require.NoError(t, err)
	require.True(t, ok, "expected deployment message in queue")

	return delivery
}

func requireDeploymentQueueEmpty(t *testing.T, conn *amqp.Connection) {
	t.Helper()

	ch, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ch.Close()) })

	_, ok, err := ch.Get(messaging.DeploymentQueue, true)
	require.NoError(t, err)
	require.False(t, ok, "expected deployment queue to be empty")
}

func installSuppressOutboxUpdateTrigger(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
) {
	t.Helper()

	_, err := db.Exec(ctx, `
		CREATE FUNCTION suppress_test_outbox_update()
		RETURNS trigger AS $$
		BEGIN
			RETURN NULL;
		END;
		$$ LANGUAGE plpgsql
		`)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		CREATE TRIGGER suppress_test_outbox_update
		BEFORE UPDATE ON outbox_messages
		FOR EACH ROW
		EXECUTE FUNCTION suppress_test_outbox_update()
		`)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS suppress_test_outbox_update
			ON outbox_messages
			`)
		require.NoError(t, err)

		_, err = db.Exec(context.Background(), `
			DROP FUNCTION IF EXISTS suppress_test_outbox_update()
			`)
		require.NoError(t, err)
	})
}
