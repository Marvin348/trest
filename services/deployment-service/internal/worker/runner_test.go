package worker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/kubernetes"
	"example.com/trest/services/deployment-service/internal/messaging"
	"example.com/trest/services/deployment-service/internal/testutil"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"uuid"
)

func TestRunner(t *testing.T) {
	url, cleanup, err := testutil.StartRabbitMQ(context.Background())
	require.NoError(t, err)
	t.Cleanup(cleanup)

	t.Run("acknowledges a successful deployment", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		state := seedWorkerDeployment(t, db, deployment.StatusQueued)
		message := workerProcessMessage(t, db, state.ID)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, deploymentRepo, _ := newWorkerTestService(db, executor)
		runner, channel := newRunnerTest(t, url, service)
		stop := startRunnerTest(t, runner)

		body, err := json.Marshal(message)
		require.NoError(t, err)

		publishRunnerMessage(t, channel, body, nil)

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(context.Background(), state.ID)
			return err == nil && saved.Status == deployment.StatusSucceeded
		}, 5*time.Second, 10*time.Millisecond)

		stop()

		require.Equal(t, 1, executorCalls)
		requireRunnerQueuesEmpty(t, channel)
	})

	t.Run("acknowledges a permanently failed deployment", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		state := seedWorkerDeployment(t, db, deployment.StatusQueued)
		message := workerProcessMessage(t, db, state.ID)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return kubernetes.ErrDeploymentMismatch
		})

		service, deploymentRepo, _ := newWorkerTestService(db, executor)
		runner, channel := newRunnerTest(t, url, service)
		stop := startRunnerTest(t, runner)

		body, err := json.Marshal(message)
		require.NoError(t, err)

		publishRunnerMessage(t, channel, body, nil)

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(context.Background(), state.ID)
			return err == nil && saved.Status == deployment.StatusFailed
		}, 5*time.Second, 10*time.Millisecond)

		stop()

		require.Equal(t, 1, executorCalls)
		requireRunnerQueuesEmpty(t, channel)
	})

	t.Run("retries a deadline error and processes the message again", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		state := seedWorkerDeployment(t, db, deployment.StatusQueued)
		message := workerProcessMessage(t, db, state.ID)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			if executorCalls == 1 {
				return context.DeadlineExceeded
			}

			return nil
		})

		service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)
		runner, channel := newRunnerTest(t, url, service)
		stop := startRunnerTest(t, runner)

		body, err := json.Marshal(message)
		require.NoError(t, err)

		headers := amqp.Table{"x-retry-count": int32(1)}
		messageID := publishRunnerMessage(t, channel, body, headers)

		retry := waitRunnerDelivery(t, channel, messaging.DeploymentRetryQueue)

		require.Equal(t, messageID, retry.MessageId)
		require.Equal(t, body, retry.Body)
		require.Equal(t, int32(2), retry.Headers["x-retry-count"])

		saved, err := deploymentRepo.GetByID(context.Background(), state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusRunning, saved.Status)

		// Put the retry back so RabbitMQ can return it after the real queue TTL.
		require.NoError(t, retry.Nack(false, true))

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(context.Background(), state.ID)
			return err == nil && saved.Status == deployment.StatusSucceeded
		}, 8*time.Second, 20*time.Millisecond)

		stop()

		require.Equal(t, 2, executorCalls)
		requireRunnerQueuesEmpty(t, channel)

		events, err := eventRepo.GetByDeploymentID(context.Background(), state.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)
		require.Equal(t, string(deployment.DeploymentSucceeded), events[2].Type)
	})

	t.Run("dead letters a message when retries are exhausted", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		state := seedWorkerDeployment(t, db, deployment.StatusRunning)
		message := workerProcessMessage(t, db, state.ID)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return context.DeadlineExceeded
		})

		service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)
		runner, channel := newRunnerTest(t, url, service)
		stop := startRunnerTest(t, runner)

		body, err := json.Marshal(message)
		require.NoError(t, err)

		messageID := publishRunnerMessage(t, channel, body, amqp.Table{"x-retry-count": int32(3)})

		deadLetter := waitRunnerDelivery(t, channel, messaging.DeploymentDLQ)

		require.Equal(t, messageID, deadLetter.MessageId)
		require.Equal(t, body, deadLetter.Body)
		require.Equal(t, int32(3), deadLetter.Headers["x-retry-count"])
		require.NoError(t, deadLetter.Ack(false))

		stop()

		require.Equal(t, 1, executorCalls)
		requireRunnerQueuesEmpty(t, channel)

		saved, err := deploymentRepo.GetByID(context.Background(), state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusRunning, saved.Status)

		events, err := eventRepo.GetByDeploymentID(context.Background(), state.ID)
		require.NoError(t, err)
		require.Len(t, events, 2)
	})

	for _, testCase := range []struct {
		name    string
		body    []byte
		headers amqp.Table
	}{
		{name: "invalid JSON", body: []byte(`{invalid`)},
		{name: "invalid retry header", body: []byte(`{}`), headers: amqp.Table{"x-retry-count": "invalid"}},
	} {
		t.Run("dead letters "+testCase.name, func(t *testing.T) {
			db := requireWorkerTestDB(t)
			testutil.ResetDB(t, db)

			executorCalls := 0

			executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
				executorCalls++
				return nil
			})

			service, _, _ := newWorkerTestService(db, executor)
			runner, channel := newRunnerTest(t, url, service)
			stop := startRunnerTest(t, runner)

			messageID := publishRunnerMessage(t, channel, testCase.body, testCase.headers)

			deadLetter := waitRunnerDelivery(t, channel, messaging.DeploymentDLQ)

			require.Equal(t, messageID, deadLetter.MessageId)
			require.Equal(t, testCase.body, deadLetter.Body)
			require.NoError(t, deadLetter.Ack(false))

			stop()

			require.Zero(t, executorCalls)
			requireRunnerQueuesEmpty(t, channel)
		})
	}

	t.Run("dead letters a deployment without events", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		message := deployment.DeploymentRequestedMessage{DeploymentID: uuid.New()}

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, _, _ := newWorkerTestService(db, executor)
		runner, channel := newRunnerTest(t, url, service)
		stop := startRunnerTest(t, runner)

		body, err := json.Marshal(message)
		require.NoError(t, err)

		messageID := publishRunnerMessage(t, channel, body, nil)

		deadLetter := waitRunnerDelivery(t, channel, messaging.DeploymentDLQ)

		require.Equal(t, messageID, deadLetter.MessageId)
		require.NoError(t, deadLetter.Ack(false))

		stop()

		require.Zero(t, executorCalls)
		requireRunnerQueuesEmpty(t, channel)
	})

	t.Run("dead letters an invalid state transition", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		state := seedWorkerDeployment(t, db, deployment.StatusRunning)
		message := workerProcessMessage(t, db, state.ID)

		_, err := db.Exec(context.Background(), `
			INSERT INTO deployment_events (deployment_id, version, type, payload)
			VALUES ($1, 3, $2, '{}')`, state.ID, string(deployment.DeploymentStarted))
		require.NoError(t, err)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, _, _ := newWorkerTestService(db, executor)
		runner, channel := newRunnerTest(t, url, service)
		stop := startRunnerTest(t, runner)

		body, err := json.Marshal(message)
		require.NoError(t, err)

		messageID := publishRunnerMessage(t, channel, body, nil)

		deadLetter := waitRunnerDelivery(t, channel, messaging.DeploymentDLQ)

		require.Equal(t, messageID, deadLetter.MessageId)
		require.NoError(t, deadLetter.Ack(false))

		stop()

		require.Zero(t, executorCalls)
		requireRunnerQueuesEmpty(t, channel)
	})

	t.Run("stops when its context is canceled", func(t *testing.T) {
		runner, _ := newRunnerTest(t, url, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := runner.Run(ctx)

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("returns an error when the consumer is closed", func(t *testing.T) {
		runner, _ := newRunnerTest(t, url, nil)

		require.NoError(t, runner.consumer.Close())

		err := runner.Run(context.Background())

		require.ErrorContains(t, err, "consume deployment queue")
	})
}

func TestGetRetryCount(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		headers amqp.Table
		count   int32
		wantErr bool
	}{
		{name: "missing header defaults to zero"},
		{name: "zero retries", headers: amqp.Table{"x-retry-count": int32(0)}},
		{name: "positive retries", headers: amqp.Table{"x-retry-count": int32(2)}, count: 2},
		{name: "negative retries", headers: amqp.Table{"x-retry-count": int32(-1)}, wantErr: true},
		{name: "string value", headers: amqp.Table{"x-retry-count": "2"}, wantErr: true},
		{name: "int64 value", headers: amqp.Table{"x-retry-count": int64(2)}, wantErr: true},
		{name: "nil value", headers: amqp.Table{"x-retry-count": nil}, wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			delivery := amqp.Delivery{Headers: testCase.headers}

			count, err := getRetryCount(delivery)

			if testCase.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, testCase.count, count)
		})
	}
}

func newRunnerTest(t *testing.T, url string, service *Service) (*Runner, *amqp.Channel) {
	t.Helper()

	conn, err := messaging.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	retryPublisher, err := messaging.NewRetryPublisher(conn)
	require.NoError(t, err)

	consumer, err := messaging.NewConsumer(conn)
	require.NoError(t, err)

	channel, err := conn.Channel()
	require.NoError(t, err)

	for _, queue := range []string{
		messaging.DeploymentQueue,
		messaging.DeploymentRetryQueue,
		messaging.DeploymentDLQ,
	} {
		_, err := channel.QueuePurge(queue, false)
		require.NoError(t, err)
	}

	return NewRunner(consumer, retryPublisher, service), channel
}

func startRunnerTest(t *testing.T, runner *Runner) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- runner.Run(ctx) }()

	stop := sync.OnceFunc(func() {
		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Error("runner did not stop after cancellation")
		}

		// Closing the consumer requeues unacknowledged messages, making missing ACKs visible.
		require.NoError(t, runner.consumer.Close())
	})

	t.Cleanup(stop)

	return stop
}

func publishRunnerMessage(t *testing.T, channel *amqp.Channel, body []byte, headers amqp.Table) string {
	t.Helper()

	id := uuid.New().String()

	err := channel.PublishWithContext(
		context.Background(),
		messaging.DeploymentExchange,
		messaging.DeploymentRoutingKey,
		false,
		false,
		amqp.Publishing{
			MessageId:    id,
			Type:         messaging.DeploymentRoutingKey,
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			Body:         body,
		},
	)
	require.NoError(t, err)

	return id
}

func waitRunnerDelivery(t *testing.T, channel *amqp.Channel, queue string) amqp.Delivery {
	t.Helper()

	var delivery amqp.Delivery
	var getErr error

	require.Eventually(t, func() bool {
		var found bool
		delivery, found, getErr = channel.Get(queue, false)
		return getErr != nil || found
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, getErr)

	return delivery
}

func requireRunnerQueuesEmpty(t *testing.T, channel *amqp.Channel) {
	t.Helper()

	for _, name := range []string{
		messaging.DeploymentQueue,
		messaging.DeploymentRetryQueue,
		messaging.DeploymentDLQ,
	} {
		queue, err := channel.QueueInspect(name)
		require.NoError(t, err)
		require.Zero(t, queue.Messages, "unexpected message in %s", name)
	}
}
