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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"uuid"
)

func TestProcess(t *testing.T) {
	t.Run("queued deployment succeeds", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)
		ctx := context.Background()

		state := seedWorkerDeployment(t, db, deployment.StatusQueued)
		deploymentRepo := deployment.NewRepository(db)

		queuedDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusQueued, queuedDeployment.Status)

		message := deployment.DeploymentRequestedMessage{
			DeploymentID:    queuedDeployment.ID,
			ProjectID:       queuedDeployment.ProjectID,
			ApplicationID:   queuedDeployment.ApplicationID,
			Image:           queuedDeployment.Image,
			Replicas:        queuedDeployment.Replicas,
			ContainerPort:   queuedDeployment.ContainerPort,
			CPURequestM:     queuedDeployment.CPURequestM,
			CPULimitM:       queuedDeployment.CPULimitM,
			MemoryRequestMi: queuedDeployment.MemoryRequestMi,
			MemoryLimitMi:   queuedDeployment.MemoryLimitMi,
			Environment:     queuedDeployment.Environment,
		}

		executorCalls := 0
		executor := workerTestExecutor(func(ctx context.Context, received deployment.DeploymentRequestedMessage) error {
			executorCalls++
			require.Equal(t, message, received)

			runningDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
			require.NoError(t, err)
			require.Equal(t, deployment.StatusRunning, runningDeployment.Status)
			return nil
		})
		service, _, eventRepo := newWorkerTestService(db, executor)

		err = service.Process(ctx, message)

		require.NoError(t, err)
		require.Equal(t, 1, executorCalls)

		savedDeployment, err := deploymentRepo.GetByID(ctx, state.ID)
		require.NoError(t, err)
		require.Equal(t, deployment.StatusSucceeded, savedDeployment.Status)

		events, err := eventRepo.GetByDeploymentID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)

		expectedTypes := []deployment.EventType{
			deployment.DeploymentCreated,
			deployment.DeploymentStarted,
			deployment.DeploymentSucceeded,
		}
		for index, expectedType := range expectedTypes {
			require.Equal(t, state.ID, events[index].DeploymentID)
			require.Equal(t, string(expectedType), events[index].Type)
			require.Equal(t, int64(index+1), events[index].Version)
		}

		rehydrated, err := deployment.Rehydrate(events)
		require.NoError(t, err)
		require.Equal(t, deployment.State{
			ID:      state.ID,
			Status:  deployment.StatusSucceeded,
			Version: 3,
		}, rehydrated)
	})

	t.Run("running deployment succeeds without another started event", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		state := seedWorkerDeployment(t, db, deployment.StatusRunning)
		message := workerProcessMessage(t, db, state.ID)

		executorCalls := 0
		executor := workerTestExecutor(func(_ context.Context,
			received deployment.DeploymentRequestedMessage,
		) error {
			executorCalls++
			require.Equal(t, message, received)
			return nil
		})

		service, _, _ := newWorkerTestService(db, executor)

		err := service.Process(context.Background(), message)

		require.NoError(t, err)
		require.Equal(t, 1, executorCalls)
		requireProcessState(
			t,
			service,
			state.ID,
			deployment.StatusSucceeded,
			deployment.DeploymentCreated,
			deployment.DeploymentStarted,
			deployment.DeploymentSucceeded,
		)
	})

	t.Run("temporary executor errors leave deployment running", func(t *testing.T) {
		for _, initialStatus := range []deployment.Status{deployment.StatusQueued, deployment.StatusRunning} {
			t.Run(string(initialStatus), func(t *testing.T) {
				for _, testCase := range []struct {
					name string
					err  error
				}{
					{name: "connection error", err: errors.New("kubernetes connection reset")},
					{name: "kubernetes timeout", err: apierrors.NewTimeoutError("API request timed out", 1)},
					{name: "kubernetes unavailable", err: apierrors.NewServiceUnavailable("API unavailable")},
				} {
					t.Run(testCase.name, func(t *testing.T) {
						db := requireWorkerTestDB(t)
						testutil.ResetDB(t, db)

						state := seedWorkerDeployment(t, db, initialStatus)
						message := workerProcessMessage(t, db, state.ID)

						executorCalls := 0
						executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
							executorCalls++
							return fmt.Errorf("deploy: %w", testCase.err)
						})

						service, _, _ := newWorkerTestService(db, executor)

						err := service.Process(context.Background(), message)

						require.ErrorIs(t, err, testCase.err)
						require.Equal(t, 1, executorCalls)
						requireProcessState(
							t,
							service,
							state.ID,
							deployment.StatusRunning,
							deployment.DeploymentCreated,
							deployment.DeploymentStarted)
					})
				}
			})
		}
	})

	t.Run("executor deadline leaves deployment running and retry succeeds", func(t *testing.T) {
		for _, initialStatus := range []deployment.Status{deployment.StatusQueued, deployment.StatusRunning} {
			t.Run(string(initialStatus), func(t *testing.T) {
				db := requireWorkerTestDB(t)
				testutil.ResetDB(t, db)

				state := seedWorkerDeployment(t, db, initialStatus)
				message := workerProcessMessage(t, db, state.ID)
				
				ctx := context.Background()
				executorCalls := 0

				executor := workerTestExecutor(func(ctx context.Context, received deployment.DeploymentRequestedMessage) error {
					executorCalls++
					require.Equal(t, message, received)
					if executorCalls == 1 {
						// Model the executor's readiness timeout while the worker context stays active.
						readyCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
						defer cancel()

						<-readyCtx.Done()
						return fmt.Errorf("wait for deployment readiness: %w", readyCtx.Err())
					}
					return nil
				})

				service, _, _ := newWorkerTestService(db, executor)

				err := service.Process(ctx, message)

				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.NoError(t, ctx.Err())
				require.Equal(t, 1, executorCalls)
				runningEvents := requireProcessState(
					t,
					service,
					state.ID,
					deployment.StatusRunning,
					deployment.DeploymentCreated,
					deployment.DeploymentStarted)

				err = service.Process(ctx, message)

				require.NoError(t, err)
				require.Equal(t, 2, executorCalls)

				finishedEvents := requireProcessState(
					t,
					service,
					state.ID,
					deployment.StatusSucceeded,
					deployment.DeploymentCreated,
					deployment.DeploymentStarted,
					deployment.DeploymentSucceeded,
				)

				require.Equal(t, runningEvents, finishedEvents[:2])
			})
		}
	})

	t.Run("permanent executor errors fail deployment", func(t *testing.T) {
		invalidDeployment := apierrors.NewInvalid(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "test",
			field.ErrorList{field.Invalid(field.NewPath("spec", "replicas"), -1, "must not be negative")})

		for _, initialStatus := range []deployment.Status{deployment.StatusQueued, deployment.StatusRunning} {
			t.Run(string(initialStatus), func(t *testing.T) {

				for _, testCase := range []struct {
					name   string
					err    error
					reason string
				}{
					{name: "deployment mismatch", err: kubernetes.ErrDeploymentMismatch, reason: "DeploymentMismatch"},
					{name: "secret mismatch", err: kubernetes.ErrSecretMismatch, reason: "SecretMismatch"},
					{name: "invalid kubernetes deployment", err: invalidDeployment, reason: "Invalid"},
				} {
					t.Run(testCase.name, func(t *testing.T) {
						db := requireWorkerTestDB(t)
						testutil.ResetDB(t, db)

						state := seedWorkerDeployment(t, db, initialStatus)
						message := workerProcessMessage(t, db, state.ID)

						executionErr := fmt.Errorf("deploy: %w", testCase.err)

						executorCalls := 0

						executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
							executorCalls++
							return executionErr
						})

						service, _, _ := newWorkerTestService(db, executor)

						err := service.Process(context.Background(), message)

						require.NoError(t, err)
						require.Equal(t, 1, executorCalls)

						events := requireProcessState(t, service, state.ID, deployment.StatusFailed,
							deployment.DeploymentCreated, deployment.DeploymentStarted, deployment.DeploymentFailed)

						var payload deployment.DeploymentFailedPayload

						require.NoError(t, json.Unmarshal(events[2].Payload, &payload))
						require.Equal(t, testCase.reason, payload.Reason)
						require.Equal(t, executionErr.Error(), payload.Message)
					})
				}
			})
		}
	})

	t.Run("terminal deployments ignore redelivery", func(t *testing.T) {
		for _, testCase := range []struct {
			status    deployment.Status
			eventType deployment.EventType
			err       error
		}{
			{status: deployment.StatusSucceeded, eventType: deployment.DeploymentSucceeded},
			{status: deployment.StatusFailed, eventType: deployment.DeploymentFailed, err: kubernetes.ErrDeploymentMismatch},
		} {
			t.Run(string(testCase.status), func(t *testing.T) {
				db := requireWorkerTestDB(t)
				testutil.ResetDB(t, db)

				state := seedWorkerDeployment(t, db, deployment.StatusQueued)
				message := workerProcessMessage(t, db, state.ID)

				executorCalls := 0

				executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
					executorCalls++
					return testCase.err
				})

				service, _, _ := newWorkerTestService(db, executor)

				require.NoError(t, service.Process(context.Background(), message))
				require.Equal(t, 1, executorCalls)

				before := requireProcessState(
					t,
					service,
					state.ID,
					testCase.status,
					deployment.DeploymentCreated,
					deployment.DeploymentStarted,
					testCase.eventType)

				err := service.Process(context.Background(), message)

				require.NoError(t, err)
				require.Equal(t, 1, executorCalls)

				after := requireProcessState(t,
					service,
					state.ID,
					testCase.status,
					deployment.DeploymentCreated,
					deployment.DeploymentStarted,
					testCase.eventType)

				require.Equal(t, before, after)
			})
		}
	})

	t.Run("expired worker context does not execute or change deployment", func(t *testing.T) {
		for _, initialStatus := range []deployment.Status{deployment.StatusQueued, deployment.StatusRunning} {
			t.Run(string(initialStatus), func(t *testing.T) {
				db := requireWorkerTestDB(t)
				testutil.ResetDB(t, db)

				state := seedWorkerDeployment(t, db, initialStatus)
				message := workerProcessMessage(t, db, state.ID)

				executorCalls := 0
				executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
					executorCalls++
					return nil
				})

				service, _, _ := newWorkerTestService(db, executor)

				expectedTypes := []deployment.EventType{deployment.DeploymentCreated}

				if initialStatus == deployment.StatusRunning {
					expectedTypes = append(expectedTypes, deployment.DeploymentStarted)
				}

				before := requireProcessState(t, service, state.ID, initialStatus, expectedTypes...)

				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()

				err := service.Process(ctx, message)

				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Zero(t, executorCalls)
				after := requireProcessState(t, service, state.ID, initialStatus, expectedTypes...)
				require.Equal(t, before, after)
			})
		}
	})

	t.Run("worker cancellation during execution leaves deployment running", func(t *testing.T) {
		for _, initialStatus := range []deployment.Status{deployment.StatusQueued, deployment.StatusRunning} {
			t.Run(string(initialStatus), func(t *testing.T) {
				db := requireWorkerTestDB(t)
				testutil.ResetDB(t, db)

				state := seedWorkerDeployment(t, db, initialStatus)
				message := workerProcessMessage(t, db, state.ID)

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				executorCalls := 0

				executor := workerTestExecutor(func(ctx context.Context, _ deployment.DeploymentRequestedMessage) error {
					executorCalls++
					cancel()
					<-ctx.Done()
					return fmt.Errorf("deploy: %w", ctx.Err())
				})

				service, _, _ := newWorkerTestService(db, executor)

				err := service.Process(ctx, message)

				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, 1, executorCalls)

				requireProcessState(
					t,
					service,
					state.ID,
					deployment.StatusRunning,
					deployment.DeploymentCreated,
					deployment.DeploymentStarted)
			})
		}
	})

	t.Run("concurrent completion checks the persisted terminal state", func(t *testing.T) {
		for _, testCase := range []struct {
			name         string
			otherErr     error
			executionErr error
			status       deployment.Status
			eventType    deployment.EventType
			wantConflict bool
		}{
			{name: "both succeeded", status: deployment.StatusSucceeded, eventType: deployment.DeploymentSucceeded},
			{name: "both failed", otherErr: kubernetes.ErrSecretMismatch, executionErr: kubernetes.ErrDeploymentMismatch,
				status: deployment.StatusFailed, eventType: deployment.DeploymentFailed},
			{name: "other failed but worker succeeded", otherErr: kubernetes.ErrSecretMismatch,
				status: deployment.StatusFailed, eventType: deployment.DeploymentFailed, wantConflict: true},
			{name: "other succeeded but worker failed", executionErr: kubernetes.ErrDeploymentMismatch,
				status: deployment.StatusSucceeded, eventType: deployment.DeploymentSucceeded, wantConflict: true},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				db := requireWorkerTestDB(t)
				testutil.ResetDB(t, db)

				state := seedWorkerDeployment(t, db, deployment.StatusRunning)
				message := workerProcessMessage(t, db, state.ID)

				otherService, _, _ := newWorkerTestService(db, nil)

				var otherEvents []eventstore.Event

				executorCalls := 0

				executor := workerTestExecutor(func(ctx context.Context, _ deployment.DeploymentRequestedMessage) error {
					executorCalls++
					// Commit the competing result after Process has loaded its local RUNNING state.
					require.NoError(t, otherService.finishDeployment(ctx, state, testCase.otherErr))

					otherEvents = requireProcessState(
						t,
						otherService,
						state.ID,
						testCase.status,
						deployment.DeploymentCreated, deployment.DeploymentStarted,
						testCase.eventType)

					return testCase.executionErr
				})

				service, _, _ := newWorkerTestService(db, executor)

				err := service.Process(context.Background(), message)

				if testCase.wantConflict {
					require.ErrorContains(t, err, "unexpected status "+string(testCase.status)+" after concurrency conflict")
				} else {
					require.NoError(t, err)
				}

				require.Equal(t, 1, executorCalls)

				after := requireProcessState(
					t,
					service,
					state.ID,
					testCase.status,
					deployment.DeploymentCreated,
					deployment.DeploymentStarted,
					testCase.eventType)

				require.Equal(t, otherEvents, after)
			})
		}
	})

	t.Run("missing event stream does not execute deployment", func(t *testing.T) {
		db := requireWorkerTestDB(t)
		testutil.ResetDB(t, db)

		executorCalls := 0

		executor := workerTestExecutor(func(context.Context, deployment.DeploymentRequestedMessage) error {
			executorCalls++
			return nil
		})

		service, _, eventRepo := newWorkerTestService(db, executor)
		message := deployment.DeploymentRequestedMessage{DeploymentID: uuid.New()}

		err := service.Process(context.Background(), message)

		require.ErrorIs(t, err, deployment.ErrNoEvents)
		require.Zero(t, executorCalls)

		events, err := eventRepo.GetByDeploymentID(context.Background(), message.DeploymentID)

		require.NoError(t, err)
		require.Empty(t, events)
	})
}

func workerProcessMessage(t *testing.T, db *pgxpool.Pool,
	id uuid.UUID) deployment.DeploymentRequestedMessage {
	t.Helper()

	saved, err := deployment.NewRepository(db).GetByID(context.Background(), id)
	require.NoError(t, err)

	return deployment.DeploymentRequestedMessage{
		DeploymentID:    saved.ID,
		ProjectID:       saved.ProjectID,
		ApplicationID:   saved.ApplicationID,
		Image:           saved.Image,
		Replicas:        saved.Replicas,
		ContainerPort:   saved.ContainerPort,
		CPURequestM:     saved.CPURequestM,
		CPULimitM:       saved.CPULimitM,
		MemoryRequestMi: saved.MemoryRequestMi,
		MemoryLimitMi:   saved.MemoryLimitMi,
		Environment:     saved.Environment,
	}
}

func requireProcessState(t *testing.T, service *Service,
	id uuid.UUID, status deployment.Status,
	types ...deployment.EventType) []eventstore.Event {
	t.Helper()
	ctx := context.Background()

	saved, err := service.deploymentRepo.GetByID(ctx, id)
	require.NoError(t, err)
	require.Equal(t, status, saved.Status)
	events, err := service.eventRepo.GetByDeploymentID(ctx, id)
	require.NoError(t, err)
	require.Len(t, events, len(types))

	for index, eventType := range types {
		require.Equal(t, id, events[index].DeploymentID)
		require.Equal(t, string(eventType), events[index].Type)
		require.Equal(t, int64(index+1), events[index].Version)
		require.Equal(t, 1, events[index].SchemaVersion)
	}

	rehydrated, err := deployment.Rehydrate(events)
	require.NoError(t, err)
	require.Equal(t, deployment.State{ID: id, Status: status, Version: int64(len(types))}, rehydrated)
	return events
}
