package worker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/testutil"
	"github.com/Marvin348/trest/packages/secretcrypto"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"uuid"
)

var workerTestDB *pgxpool.Pool
var workerTestDBCleanup func()
var workerTestDBErr error
var workerTestDBOnce sync.Once

type workerTestExecutor func(
	context.Context,
	deployment.DeploymentRequestedMessage) error

func (execute workerTestExecutor) Deploy(
	ctx context.Context,
	message deployment.DeploymentRequestedMessage) error {
	return execute(ctx, message)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if workerTestDBCleanup != nil {
		workerTestDBCleanup()
	}
	os.Exit(code)
}

func TestStartDeployment(t *testing.T) {
	db := requireWorkerTestDB(t)
	testutil.ResetDB(t, db)

	service, deploymentRepo, eventRepo := newWorkerTestService(db, nil)
	state := seedWorkerDeployment(t, db, deployment.StatusQueued)

	updatedState, err := service.startDeployment(context.Background(), state)

	require.NoError(t, err)
	require.Equal(t, state.ID, updatedState.ID)
	require.Equal(t, deployment.StatusRunning, updatedState.Status)
	require.Equal(t, int64(2), updatedState.Version)

	savedDeployment, err := deploymentRepo.GetByID(context.Background(), state.ID)
	require.NoError(t, err)
	require.Equal(t, deployment.StatusRunning, savedDeployment.Status)

	events, err := eventRepo.GetByDeploymentID(context.Background(), state.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, string(deployment.DeploymentStarted), events[1].Type)
	require.Equal(t, int64(2), events[1].Version)
	require.Equal(t, 1, events[1].SchemaVersion)
	require.JSONEq(t, `{}`, string(events[1].Payload))

	rehydrated, err := deployment.Rehydrate(events)
	require.NoError(t, err)
	require.Equal(t, updatedState, rehydrated)
}

func TestFinishDeployment(t *testing.T) {
	testCases := []struct {
		name           string
		executionErr   error
		expectedEvent  deployment.EventType
		expectedStatus deployment.Status
	}{
		{
			name:           "successful execution",
			expectedEvent:  deployment.DeploymentSucceeded,
			expectedStatus: deployment.StatusSucceeded,
		},
		{
			name:           "failed execution",
			executionErr:   errors.New("kubernetes deployment failed"),
			expectedEvent:  deployment.DeploymentFailed,
			expectedStatus: deployment.StatusFailed,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := requireWorkerTestDB(t)
			testutil.ResetDB(t, db)

			service, deploymentRepo, eventRepo := newWorkerTestService(db, nil)
			state := seedWorkerDeployment(t, db, deployment.StatusRunning)

			err := service.finishDeployment(
				context.Background(),
				state,
				testCase.executionErr,
			)

			require.NoError(t, err)

			savedDeployment, err := deploymentRepo.GetByID(
				context.Background(),
				state.ID,
			)
			require.NoError(t, err)
			require.Equal(t, testCase.expectedStatus, savedDeployment.Status)

			events, err := eventRepo.GetByDeploymentID(
				context.Background(),
				state.ID,
			)
			require.NoError(t, err)
			require.Len(t, events, 3)
			require.Equal(t, string(testCase.expectedEvent), events[2].Type)
			require.Equal(t, int64(3), events[2].Version)
			require.Equal(t, 1, events[2].SchemaVersion)
			if testCase.executionErr == nil {
				require.JSONEq(t, `{}`, string(events[2].Payload))
			} else {
				var payload deployment.DeploymentFailedPayload
				require.NoError(t, json.Unmarshal(events[2].Payload, &payload))
				require.Equal(t, "DeploymentExecutionFailed", payload.Reason)
				require.Equal(t, testCase.executionErr.Error(), payload.Message)
			}

			rehydrated, err := deployment.Rehydrate(events)
			require.NoError(t, err)
			require.Equal(t, state.ID, rehydrated.ID)
			require.Equal(t, testCase.expectedStatus, rehydrated.Status)
			require.Equal(t, int64(3), rehydrated.Version)
		})
	}
}

func requireWorkerTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	workerTestDBOnce.Do(func() {
		workerTestDB, workerTestDBCleanup, workerTestDBErr =
			testutil.StartPostgres(context.Background())
	})
	require.NoError(t, workerTestDBErr)

	return workerTestDB
}

func newWorkerTestService(
	db *pgxpool.Pool,
	executor DeploymentExecutor,
) (*Service, *deployment.Repository, *eventstore.Repository) {
	deploymentRepo := deployment.NewRepository(db)
	eventRepo := eventstore.NewRepository(db)

	return NewService(eventRepo, deploymentRepo, db, executor), deploymentRepo, eventRepo
}

func seedWorkerDeployment(
	t *testing.T,
	db *pgxpool.Pool,
	status deployment.Status,
) deployment.State {
	t.Helper()

	ctx := context.Background()
	deploymentID := uuid.New()
	data := deployment.DeploymentCreatedPayload{
		RequestID:       uuid.New(),
		ProjectID:       uuid.New(),
		ApplicationID:   uuid.New(),
		Image:           "nginx:1.27",
		Replicas:        2,
		ContainerPort:   8080,
		CPURequestM:     100,
		CPULimitM:       250,
		MemoryRequestMi: 256,
		MemoryLimitMi:   512,
		Environment:     map[string]secretcrypto.EncryptedValue{},
	}
	createdPayload, err := json.Marshal(data)
	require.NoError(t, err)
	
	environment, err := json.Marshal(data.Environment)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO deployments (
			id,
			request_id,
			project_id,
			application_id,
			image,
			replicas,
			container_port,
			cpu_request_m,
			cpu_limit_m,
			memory_request_mi,
			memory_limit_mi,
			environment,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		deploymentID,
		data.RequestID,
		data.ProjectID,
		data.ApplicationID,
		data.Image,
		data.Replicas,
		data.ContainerPort,
		data.CPURequestM,
		data.CPULimitM,
		data.MemoryRequestMi,
		data.MemoryLimitMi,
		environment,
		status,
	)
	require.NoError(t, err)

	eventTypes := []deployment.EventType{deployment.DeploymentCreated}
	if status == deployment.StatusRunning {
		eventTypes = append(eventTypes, deployment.DeploymentStarted)
	}

	for index, eventType := range eventTypes {
		payload := json.RawMessage(`{}`)
		if eventType == deployment.DeploymentCreated {
			payload = json.RawMessage(createdPayload)
		}

		_, err := db.Exec(ctx, `
			INSERT INTO deployment_events (
				deployment_id,
				version,
				type,
				schema_version,
				payload
			)
			VALUES ($1, $2, $3, $4, $5)
		`,
			deploymentID,
			int64(index+1),
			string(eventType),
			1,
			payload,
		)
		require.NoError(t, err)
	}

	return deployment.State{
		ID:      deploymentID,
		Status:  status,
		Version: int64(len(eventTypes)),
	}
}
