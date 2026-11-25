package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/kubernetes"
	"example.com/trest/services/deployment-service/internal/testutil"
	"github.com/Marvin348/trest/packages/secretcrypto"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"uuid"
)

func TestRecoveryRun(t *testing.T) {
	t.Run("executes recovery immediately on start", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		state := seedOldRunningDeployment(t, db)
		deploymentRepo := deployment.NewRepository(db)

		executorCalls := 0

		executor := workerTestExecutor(func(ctx context.Context, message deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, _, _ := newWorkerTestService(db, executor)

		done := make(chan error, 1)

		go func() {
			done <- service.Run(ctx, 30*time.Millisecond)
		}()

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(ctx, state.ID)

			return err == nil &&
				saved.Status == deployment.StatusSucceeded
		}, 5*time.Second, 10*time.Millisecond)

		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Fatal("recovery did not stop after cancellation")
		}

		require.Equal(t, 1, executorCalls)
	})

	t.Run("runs recovery periodically after tick", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stateA := seedOldRunningDeployment(t, db)
		deploymentRepo := deployment.NewRepository(db)

		executorCalls := 0

		executor := workerTestExecutor(func(ctx context.Context, message deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, _, _ := newWorkerTestService(db, executor)

		done := make(chan error, 1)

		go func() {
			done <- service.Run(ctx, 30*time.Millisecond)
		}()

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(ctx, stateA.ID)

			return err == nil &&
				saved.Status == deployment.StatusSucceeded
		}, 5*time.Second, 10*time.Millisecond)

		stateB := seedOldRunningDeployment(t, db)

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(ctx, stateB.ID)

			return err == nil &&
				saved.Status == deployment.StatusSucceeded
		}, 5*time.Second, 10*time.Millisecond)

		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Fatal("recovery did not stop after cancellation")
		}

		require.Equal(t, 2, executorCalls)
	})

	t.Run("continues polling after recovery error", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stateA := seedOldRunningDeployment(t, db)
		deploymentRepo := deployment.NewRepository(db)

		executorCalls := 0

		executor := workerTestExecutor(func(ctx context.Context, message deployment.DeploymentRequestedMessage) error {
			executorCalls++

			if executorCalls == 1 {
				return context.DeadlineExceeded
			}

			return nil
		})

		service, _, _ := newWorkerTestService(db, executor)

		done := make(chan error, 1)

		go func() {
			done <- service.Run(ctx, 30*time.Millisecond)
		}()

		require.Eventually(t, func() bool {
			saved, err := deploymentRepo.GetByID(ctx, stateA.ID)

			return err == nil &&
				saved.Status == deployment.StatusSucceeded
		}, 5*time.Second, 10*time.Millisecond)

		cancel()

		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)

		case <-time.After(5 * time.Second):
			t.Fatal("recovery did not stop after cancellation")
		}

		require.Equal(t, 2, executorCalls)
	})
}

func TestRecoverySucceed(t *testing.T) {
	t.Run("resumes an old running deployment and succeeds", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()
		state := seedWorkerDeployment(t, db, deployment.StatusRunning)

		deploymentRepo := deployment.NewRepository(db)
		savedDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)

		// Only deployments that have been running for more than five minutes are recovered.
		_, err = db.Exec(ctx,
			`UPDATE deployments SET updated_at = NOW() - INTERVAL '6 minutes' WHERE id = $1`, state.ID)
		require.NoError(t, err)

		expectedMessage := deployment.DeploymentRequestedMessage{
			DeploymentID:    state.ID,
			ProjectID:       savedDeployment.ProjectID,
			ApplicationID:   savedDeployment.ApplicationID,
			Image:           savedDeployment.Image,
			Replicas:        savedDeployment.Replicas,
			ContainerPort:   savedDeployment.ContainerPort,
			CPURequestM:     savedDeployment.CPURequestM,
			CPULimitM:       savedDeployment.CPULimitM,
			MemoryRequestMi: savedDeployment.MemoryRequestMi,
			MemoryLimitMi:   savedDeployment.MemoryLimitMi,
			Environment:     savedDeployment.Environment,
		}
		executorCalls := 0

		executor := workerTestExecutor(func(ctx context.Context, message deployment.DeploymentRequestedMessage) error {
			executorCalls++
			require.Equal(t, expectedMessage, message)

			runningDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
			require.NoError(t, err)
			require.Equal(t, deployment.StatusRunning, runningDeployment.Status)

			return nil
		})

		service, _, eventRepo := newWorkerTestService(db, executor)

		beforeEvents, err := eventRepo.GetByDeploymentID(ctx, state.ID)

		require.NoError(t, err)
		require.Len(t, beforeEvents, 2)

		require.Equal(t, string(deployment.DeploymentCreated), beforeEvents[0].Type)
		require.Equal(t, string(deployment.DeploymentStarted), beforeEvents[1].Type)

		beforeState, err := deployment.Rehydrate(beforeEvents)
		require.NoError(t, err)
		require.Equal(t, state, beforeState)

		err = service.RecoverRunningDeployments(ctx)

		require.NoError(t, err)
		require.Equal(t, 1, executorCalls)

		finishedDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)

		require.Equal(t, deployment.StatusSucceeded, finishedDeployment.Status)

		events, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)
		require.Equal(t, beforeEvents, events[:2])
		require.Equal(t, state.ID, events[2].DeploymentID)
		require.Equal(t, string(deployment.DeploymentSucceeded), events[2].Type)
		require.Equal(t, int64(3), events[2].Version)
		require.Equal(t, 1, events[2].SchemaVersion)
		require.JSONEq(t, `{}`, string(events[2].Payload))

		rehydrated, err := deployment.Rehydrate(events)

		require.NoError(t, err)

		require.Equal(t, deployment.State{
			ID:      state.ID,
			Status:  deployment.StatusSucceeded,
			Version: 3,
		}, rehydrated)
	})
}

func TestRecovery(t *testing.T) {
	t.Run("does nothing when there are no deployments", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		executorCalls := 0
		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})
		service, _, _ := newWorkerTestService(db, executor)

		err := service.RecoverRunningDeployments(context.Background())

		require.NoError(t, err)
		require.Zero(t, executorCalls)
	})

	t.Run("ignores recently started deployments", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()
		state := seedWorkerDeployment(t, db, deployment.StatusRunning)

		executorCalls := 0

		executor := workerTestExecutor(func(ctx context.Context, message deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)
		beforeEvents, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, beforeEvents, 2)

		err = service.RecoverRunningDeployments(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, executorCalls)

		savedDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusRunning, savedDeployment.Status)

		afterEvents, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, afterEvents, 2)
		require.Equal(t, beforeEvents, afterEvents)
	})

	t.Run("ignores queued and terminal deployments", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()

		queued := seedWorkerDeployment(t, db, deployment.StatusQueued)
		succeeded := seedWorkerDeployment(t, db, deployment.StatusRunning)
		failed := seedWorkerDeployment(t, db, deployment.StatusRunning)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})
		service, deploymentRepo, _ := newWorkerTestService(db, executor)

		require.NoError(t, service.finishDeployment(ctx, succeeded, nil))
		require.NoError(t, service.finishDeployment(ctx, failed, kubernetes.ErrDeploymentMismatch))

		_, err := db.Exec(ctx, `UPDATE deployments SET updated_at = NOW() - INTERVAL '6 minutes'`)
		require.NoError(t, err)

		err = service.RecoverRunningDeployments(ctx)

		require.NoError(t, err)
		require.Zero(t, executorCalls)

		for id, status := range map[uuid.UUID]deployment.Status{
			queued.ID: deployment.StatusQueued, succeeded.ID: deployment.StatusSucceeded, failed.ID: deployment.StatusFailed,
		} {
			saved, err := deploymentRepo.GetByID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, status, saved.Status)
		}
	})

	t.Run("deadline leaves deployment running and retry succeeds", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()

		state := seedOldRunningDeployment(t, db)

		executorCalls := 0

		executor := workerTestExecutor(func(ctx context.Context, _ deployment.DeploymentRequestedMessage) error {
			executorCalls++
			if executorCalls == 1 {
				readyCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
				<-readyCtx.Done()
				return fmt.Errorf("wait for readiness: %w", readyCtx.Err())
			}
			return nil
		})

		service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)

		before, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)

		err = service.RecoverRunningDeployments(ctx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NoError(t, ctx.Err())
		require.Equal(t, 1, executorCalls)

		saved, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusRunning, saved.Status)

		after, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, before, after)

		require.NoError(t, service.RecoverRunningDeployments(ctx))
		require.Equal(t, 2, executorCalls)

		saved, err = deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusSucceeded, saved.Status)

		after, err = eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, after, 3)
		require.Equal(t, before, after[:2])
		require.Equal(t, string(deployment.DeploymentSucceeded), after[2].Type)
	})

	t.Run("temporary error leaves events unchanged", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()

		state := seedOldRunningDeployment(t, db)

		executionErr := errors.New("kubernetes unavailable")

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return executionErr
		})
		service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)

		before, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)

		err = service.RecoverRunningDeployments(ctx)

		require.ErrorIs(t, err, executionErr)
		require.Equal(t, 1, executorCalls)

		saved, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusRunning, saved.Status)

		after, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("permanent error marks deployment failed", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()

		state := seedOldRunningDeployment(t, db)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return kubernetes.ErrDeploymentMismatch
		})
		service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)

		err := service.RecoverRunningDeployments(ctx)

		require.NoError(t, err)
		require.Equal(t, 1, executorCalls)

		saved, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusFailed, saved.Status)

		events, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)
		require.Equal(t, string(deployment.DeploymentFailed), events[2].Type)
		require.Equal(t, int64(3), events[2].Version)

		var payload deployment.DeploymentFailedPayload
		require.NoError(t, json.Unmarshal(events[2].Payload, &payload))
		require.Equal(t, "DeploymentMismatch", payload.Reason)
		require.Equal(t, kubernetes.ErrDeploymentMismatch.Error(), payload.Message)
	})

	t.Run("continues with the next deployment after an executor error", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		ctx := context.Background()

		failed := seedOldRunningDeployment(t, db)
		next := seedOldRunningDeployment(t, db)

		_, err := db.Exec(ctx,
			`UPDATE deployments SET updated_at = NOW() - INTERVAL '7 minutes' WHERE id = $1`, failed.ID)
		require.NoError(t, err)

		executionErr := errors.New("kubernetes unavailable")

		var calls []uuid.UUID
		executor := workerTestExecutor(func(_ context.Context, message deployment.DeploymentRequestedMessage) error {
			calls = append(calls, message.DeploymentID)
			if message.DeploymentID == failed.ID {
				return executionErr
			}
			return nil
		})
		service, deploymentRepo, _ := newWorkerTestService(db, executor)

		err = service.RecoverRunningDeployments(ctx)

		require.ErrorIs(t, err, executionErr)
		require.ErrorContains(t, err, failed.ID.String())
		require.Equal(t, []uuid.UUID{failed.ID, next.ID}, calls)

		saved, err := deploymentRepo.GetByID(ctx, failed.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusRunning, saved.Status)

		saved, err = deploymentRepo.GetByID(ctx, next.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusSucceeded, saved.Status)
	})

	for _, testCase := range []struct {
		name   string
		events []eventstore.Event
		want   string
	}{
		{name: "missing event history", want: deployment.ErrNoEvents.Error()},
		{name: "invalid event history", want: "invalid event version", events: []eventstore.Event{
			{Type: string(deployment.DeploymentCreated), Version: 1, Payload: json.RawMessage(`{}`)},
			{Type: string(deployment.DeploymentStarted), Version: 3, Payload: json.RawMessage(`{}`)},
		}},
		{name: "invalid created payload", want: "decode deployment created payload", events: []eventstore.Event{
			{Type: string(deployment.DeploymentCreated), Version: 1, Payload: json.RawMessage(`{"replicas":"invalid"}`)},
			{Type: string(deployment.DeploymentStarted), Version: 2, Payload: json.RawMessage(`{}`)},
		}},
	} {
		t.Run(testCase.name+" does not stop other deployments", func(t *testing.T) {
			db := requireWorkerTestDB(t)
			testutil.ResetDB(t, db)

			ctx := context.Background()

			brokenID := seedRecoveryEventStream(t, db, testCase.events)

			next := seedOldRunningDeployment(t, db)
			_, err := db.Exec(ctx,
				`UPDATE deployments SET updated_at = NOW() - INTERVAL '7 minutes' WHERE id = $1`, brokenID)
			require.NoError(t, err)

			var calls []uuid.UUID

			executor := workerTestExecutor(func(_ context.Context, message deployment.DeploymentRequestedMessage) error {
				calls = append(calls, message.DeploymentID)
				return nil
			})

			service, deploymentRepo, eventRepo := newWorkerTestService(db, executor)
			before, err := eventRepo.GetByDeploymentID(ctx, brokenID)
			require.NoError(t, err)

			err = service.RecoverRunningDeployments(ctx)

			require.ErrorContains(t, err, testCase.want)
			require.Equal(t, []uuid.UUID{next.ID}, calls)

			saved, err := deploymentRepo.GetByID(ctx, brokenID)
			require.NoError(t, err)
			require.Equal(t, deployment.StatusRunning, saved.Status)

			after, err := eventRepo.GetByDeploymentID(ctx, brokenID)
			require.NoError(t, err)
			require.Equal(t, before, after)

			saved, err = deploymentRepo.GetByID(ctx, next.ID)
			require.NoError(t, err)
			require.Equal(t, deployment.StatusSucceeded, saved.Status)
		})
	}

	for _, eventType := range []deployment.EventType{deployment.DeploymentSucceeded, deployment.DeploymentFailed} {
		t.Run("skips stale running projection with "+string(eventType), func(t *testing.T) {
			db := requireWorkerTestDB(t)
			testutil.ResetDB(t, db)
			ctx := context.Background()
			id := seedRecoveryEventStream(t, db, []eventstore.Event{
				{Type: string(deployment.DeploymentCreated), Version: 1, Payload: json.RawMessage(`{}`)},
				{Type: string(deployment.DeploymentStarted), Version: 2, Payload: json.RawMessage(`{}`)},
				{Type: string(eventType), Version: 3, Payload: json.RawMessage(`{}`)},
			})
			executorCalls := 0
			executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
				executorCalls++
				return nil
			})
			service, _, eventRepo := newWorkerTestService(db, executor)
			before, err := eventRepo.GetByDeploymentID(ctx, id)
			require.NoError(t, err)

			err = service.RecoverRunningDeployments(ctx)

			require.NoError(t, err)
			require.Zero(t, executorCalls)

			after, err := eventRepo.GetByDeploymentID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}

	for _, testCase := range []struct {
		name         string
		otherErr     error
		executionErr error
		status       deployment.Status
		wantConflict bool
	}{
		{name: "both succeed", status: deployment.StatusSucceeded},
		{name: "both fail", otherErr: kubernetes.ErrSecretMismatch, executionErr: kubernetes.ErrDeploymentMismatch,
			status: deployment.StatusFailed},
		{name: "other fails but recovery succeeds", otherErr: kubernetes.ErrSecretMismatch,
			status: deployment.StatusFailed, wantConflict: true},
		{name: "other succeeds but recovery fails", executionErr: kubernetes.ErrDeploymentMismatch,
			status: deployment.StatusSucceeded, wantConflict: true},
	} {
		t.Run("concurrent completion when "+testCase.name, func(t *testing.T) {
			db := requireWorkerTestDB(t)
			testutil.ResetDB(t, db)

			ctx := context.Background()

			state := seedOldRunningDeployment(t, db)

			otherService, _, eventRepo := newWorkerTestService(db, nil)
			var otherEvents []eventstore.Event

			executorCalls := 0

			executor := workerTestExecutor(func(ctx context.Context, _ deployment.DeploymentRequestedMessage) error {
				executorCalls++
				require.NoError(t, otherService.finishDeployment(ctx, state, testCase.otherErr))
				var err error
				otherEvents, err = eventRepo.GetByDeploymentID(ctx, state.ID)
				require.NoError(t, err)
				return testCase.executionErr
			})
			service, deploymentRepo, _ := newWorkerTestService(db, executor)

			err := service.RecoverRunningDeployments(ctx)

			if testCase.wantConflict {
				require.ErrorContains(t, err, "unexpected status "+string(testCase.status))
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, executorCalls)

			saved, err := deploymentRepo.GetByID(ctx, state.ID)
			require.NoError(t, err)
			require.Equal(t, testCase.status, saved.Status)

			events, err := eventRepo.GetByDeploymentID(ctx, state.ID)
			require.NoError(t, err)
			require.Len(t, events, 3)
			require.Equal(t, otherEvents, events)
		})
	}

	t.Run("passes encrypted environment values unchanged", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		id := seedRecoveryEventStream(t, db, []eventstore.Event{
			{Type: string(deployment.DeploymentCreated), Version: 1, Payload: json.RawMessage(
				`{"environment":{"TOKEN":{"version":1,"key_id":"test-key","nonce":"test-nonce","ciphertext":"test-ciphertext"}}}`)},
			{Type: string(deployment.DeploymentStarted), Version: 2, Payload: json.RawMessage(`{}`)},
		})

		executorCalls := 0
		executor := workerTestExecutor(func(_ context.Context, message deployment.DeploymentRequestedMessage) error {
			executorCalls++
			require.Equal(t, id, message.DeploymentID)
			require.Equal(t, map[string]secretcrypto.EncryptedValue{
				"TOKEN": {Version: 1, KeyID: "test-key", Nonce: "test-nonce", Ciphertext: "test-ciphertext"},
			}, message.Environment)
			return nil
		})
		service, deploymentRepo, _ := newWorkerTestService(db, executor)

		err := service.RecoverRunningDeployments(context.Background())

		require.NoError(t, err)
		require.Equal(t, 1, executorCalls)

		saved, err := deploymentRepo.GetByID(context.Background(), id)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusSucceeded, saved.Status)
	})
}

func seedOldRunningDeployment(t *testing.T, db *pgxpool.Pool) deployment.State {
	t.Helper()

	state := seedWorkerDeployment(t, db, deployment.StatusRunning)

	_, err := db.Exec(context.Background(),
		`UPDATE deployments SET updated_at = NOW() - INTERVAL '6 minutes' WHERE id = $1`, state.ID)
	require.NoError(t, err)
	return state
}

// Explicit event streams allow inconsistent fixtures without modifying stored events.
func seedRecoveryEventStream(t *testing.T, db *pgxpool.Pool, events []eventstore.Event) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	_, err := db.Exec(ctx, `
		INSERT INTO deployments (id, request_id, project_id, application_id, image, replicas,
			container_port, cpu_request_m, cpu_limit_m, memory_request_mi, memory_limit_mi, status, updated_at)
		VALUES ($1, gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'nginx:1.27', 2,
			8080, 100, 250, 256, 512, 'RUNNING', NOW() - INTERVAL '6 minutes')`, id)
	require.NoError(t, err)
	for _, event := range events {
		_, err := db.Exec(ctx, `
			INSERT INTO deployment_events (deployment_id, version, type, schema_version, payload)
			VALUES ($1, $2, $3, 1, $4)`, id, event.Version, event.Type, event.Payload)
		require.NoError(t, err)
	}
	return id
}
