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
)

func TestServiceRun(t *testing.T) {
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

	t.Run("processes pending messages immediately on startup", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		message := newTestOutboxMessage(func(message *outbox.Message) {
			message.CreatedAt = time.Date(
				2026, time.November, 13, 15, 0, 0, 0, time.UTC,
			)
		})
		insertTestOutboxMessage(t, ctx, pool, message)

		startServiceRun(t, ctx, pool, service)
		requireEventuallyOutboxMessagePublished(t, ctx, pool, message, 2*time.Second)

		requireOutboxMessagePublished(t, ctx, pool, message)

		delivery := requireDeploymentDelivery(t, conn)
		assert.Equal(t, message.ID.String(), delivery.MessageId)
		assert.Equal(t, message.Type, delivery.Type)
		assert.JSONEq(t, string(message.Payload), string(delivery.Body))
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("processes pending messages after an outbox notification", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)
		startServiceRun(t, ctx, pool, service)
		requireEventuallyOutboxListenerConnected(t, ctx, pool)

		// Let the initial ProcessPending call finish so the notification is the
		// only mechanism that can process the message before the first tick.
		time.Sleep(100 * time.Millisecond)

		message := newTestOutboxMessage(func(message *outbox.Message) {
			message.CreatedAt = time.Date(
				2026, time.November, 13, 16, 0, 0, 0, time.UTC,
			)
		})
		insertTestOutboxMessage(t, ctx, pool, message)

		_, err := pool.Exec(ctx, "SELECT pg_notify('trest_outbox', '')")
		require.NoError(t, err)

		requireEventuallyOutboxMessagePublished(t, ctx, pool, message, 2*time.Second)
		delivery := requireDeploymentDelivery(t, conn)
		assert.Equal(t, message.ID.String(), delivery.MessageId)
		assert.JSONEq(t, string(message.Payload), string(delivery.Body))
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("polls repeatedly when no notification is sent", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		startedAt := time.Now()
		startServiceRun(t, ctx, pool, service)
		requireEventuallyOutboxListenerConnected(t, ctx, pool)

		// Insert only after the first five-second tick. A one-shot Timer would
		// never see this message; the next Ticker event must process it.
		if delay := time.Until(startedAt.Add(5200 * time.Millisecond)); delay > 0 {
			time.Sleep(delay)
		}

		message := newTestOutboxMessage(func(message *outbox.Message) {
			message.CreatedAt = time.Date(
				2026, time.November, 13, 17, 0, 0, 0, time.UTC,
			)
		})
		insertTestOutboxMessage(t, ctx, pool, message)

		requireEventuallyOutboxMessagePublished(t, ctx, pool, message, 6*time.Second)
		delivery := requireDeploymentDelivery(t, conn)
		assert.Equal(t, message.ID.String(), delivery.MessageId)
		assert.JSONEq(t, string(message.Payload), string(delivery.Body))
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("reconnects the listener after its database connection is lost", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)
		startServiceRun(t, ctx, pool, service)

		originalPID := requireEventuallyOutboxListenerPID(t, ctx, pool)

		var terminated bool
		err := pool.QueryRow(
			ctx,
			"SELECT pg_terminate_backend($1)",
			originalPID,
		).Scan(&terminated)
		require.NoError(t, err)
		require.True(t, terminated, "expected listener backend to be terminated")

		var reconnectedPID int32
		require.Eventually(t, func() bool {
			pid, connected := outboxListenerPID(ctx, pool)
			if connected && pid != originalPID {
				reconnectedPID = pid
				return true
			}
			return false
		}, 3*time.Second, 20*time.Millisecond)
		assert.NotZero(t, reconnectedPID)

		message := newTestOutboxMessage(func(message *outbox.Message) {
			message.CreatedAt = time.Date(
				2026, time.November, 13, 17, 30, 0, 0, time.UTC,
			)
		})
		insertTestOutboxMessage(t, ctx, pool, message)
		_, err = pool.Exec(ctx, "SELECT pg_notify('trest_outbox', '')")
		require.NoError(t, err)

		requireEventuallyOutboxMessagePublished(t, ctx, pool, message, 2*time.Second)
		delivery := requireDeploymentDelivery(t, conn)
		assert.Equal(t, message.ID.String(), delivery.MessageId)
		assert.JSONEq(t, string(message.Payload), string(delivery.Body))
		requireDeploymentQueueEmpty(t, conn)
	})

	t.Run("continues after a temporary processing error", func(t *testing.T) {
		resetPublishingState(t, ctx, pool, conn)

		message := newTestOutboxMessage(func(message *outbox.Message) {
			message.CreatedAt = time.Date(
				2026, time.November, 13, 18, 0, 0, 0, time.UTC,
			)
		})
		insertTestOutboxMessage(t, ctx, pool, message)
		installSuppressOutboxUpdateTrigger(t, ctx, pool)

		startServiceRun(t, ctx, pool, service)
		firstDelivery := requireEventuallyDeploymentDelivery(t, conn, 2*time.Second)
		assert.Equal(t, message.ID.String(), firstDelivery.MessageId)
		requireOutboxMessagePending(t, ctx, pool, message)

		removeSuppressOutboxUpdateTrigger(t, ctx, pool)
		requireEventuallyOutboxListenerConnected(t, ctx, pool)
		_, err := pool.Exec(ctx, "SELECT pg_notify('trest_outbox', '')")
		require.NoError(t, err)

		requireEventuallyOutboxMessagePublished(t, ctx, pool, message, 2*time.Second)
		secondDelivery := requireDeploymentDelivery(t, conn)
		assert.Equal(t, message.ID.String(), secondDelivery.MessageId)
		assert.JSONEq(t, string(message.Payload), string(secondDelivery.Body))
		requireDeploymentQueueEmpty(t, conn)
	})
}

func startServiceRun(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	service *Service,
) {
	t.Helper()

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- service.Run(runCtx, db.Config().ConnString())
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Error("service did not stop after context cancellation")
		}

		require.Eventually(t, func() bool {
			return !outboxListenerConnected(ctx, db)
		}, 2*time.Second, 20*time.Millisecond)
	})
}

func requireEventuallyOutboxMessagePublished(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	message outbox.Message,
	timeout time.Duration,
) {
	t.Helper()

	require.Eventually(t, func() bool {
		var status string
		var publishedAt *time.Time
		err := db.QueryRow(ctx, `
			SELECT status, published_at
			FROM outbox_messages
			WHERE id = $1
			`, message.ID).Scan(&status, &publishedAt)

		return err == nil &&
			status == string(outbox.StatusPublished) &&
			publishedAt != nil
	}, timeout, 20*time.Millisecond)
}

func requireEventuallyOutboxListenerConnected(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
) {
	t.Helper()

	require.Eventually(t, func() bool {
		return outboxListenerConnected(ctx, db)
	}, 2*time.Second, 20*time.Millisecond)
}

func outboxListenerConnected(ctx context.Context, db *pgxpool.Pool) bool {
	_, connected := outboxListenerPID(ctx, db)
	return connected
}

func requireEventuallyOutboxListenerPID(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
) int32 {
	t.Helper()

	var pid int32
	require.Eventually(t, func() bool {
		var connected bool
		pid, connected = outboxListenerPID(ctx, db)
		return connected
	}, 2*time.Second, 20*time.Millisecond)

	return pid
}

func outboxListenerPID(ctx context.Context, db *pgxpool.Pool) (int32, bool) {
	var pid int32
	err := db.QueryRow(ctx, `
		SELECT pid
		FROM pg_stat_activity
		WHERE datname = current_database()
		AND query LIKE 'LISTEN trest_outbox%'
		AND state = 'idle'
		ORDER BY backend_start DESC
		LIMIT 1
		`).Scan(&pid)

	return pid, err == nil
}

func requireEventuallyDeploymentDelivery(
	t *testing.T,
	conn *amqp.Connection,
	timeout time.Duration,
) amqp.Delivery {
	t.Helper()

	ch, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ch.Close()) })

	var delivery amqp.Delivery
	require.Eventually(t, func() bool {
		var ok bool
		delivery, ok, err = ch.Get(messaging.DeploymentQueue, true)
		return err == nil && ok
	}, timeout, 20*time.Millisecond)
	require.NoError(t, err)

	return delivery
}

func removeSuppressOutboxUpdateTrigger(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
) {
	t.Helper()

	_, err := db.Exec(ctx, `
		DROP TRIGGER IF EXISTS suppress_test_outbox_update
		ON outbox_messages
		`)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		DROP FUNCTION IF EXISTS suppress_test_outbox_update()
		`)
	require.NoError(t, err)
}
