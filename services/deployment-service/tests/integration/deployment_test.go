package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
	"uuid"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const rpcTimeout = 5 * time.Second

type savedDeployment struct {
	ID              uuid.UUID
	RequestID       uuid.UUID
	ProjectID       uuid.UUID
	ApplicationID   uuid.UUID
	Image           string
	Replicas        int32
	ContainerPort   int32
	CPURequestM     int32
	CPULimitM       int32
	MemoryRequestMi int32
	MemoryLimitMi   int32
	Status          string
}

type savedEvent struct {
	ID            uuid.UUID
	DeploymentID  uuid.UUID
	Version       int64
	Type          string
	SchemaVersion int
	Payload       json.RawMessage
}

type savedProcessedRequest struct {
	Operation    string
	RequestID    uuid.UUID
	DeploymentID uuid.UUID
	Fingerprint  []byte
}

func TestCreateDeployment(t *testing.T) {
	testutil.ResetDB(t, testDB)
	client := startTestGRPCServer(t)
	request := validCreateDeploymentRequest()

	response, err := callCreateDeployment(t, client, request)

	require.NoError(t, err)
	require.Equal(t, "QUEUED", response.GetStatus())

	deploymentID, err := uuid.Parse(response.GetDeploymentId())
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil(), deploymentID)

	saved := findDeploymentByRequestID(t, request.GetRequestId())
	require.Equal(t, deploymentID, saved.ID)
	require.Equal(t, uuid.MustParse(request.GetRequestId()), saved.RequestID)
	require.Equal(t, uuid.MustParse(request.GetProjectId()), saved.ProjectID)
	require.Equal(t, uuid.MustParse(request.GetApplicationId()), saved.ApplicationID)
	require.Equal(t, request.GetImage(), saved.Image)
	require.Equal(t, request.GetReplicas(), saved.Replicas)
	require.Equal(t, request.GetContainerPort(), saved.ContainerPort)
	require.Equal(t, request.GetCpuRequestM(), saved.CPURequestM)
	require.Equal(t, request.GetCpuLimitM(), saved.CPULimitM)
	require.Equal(t, request.GetMemoryRequestMi(), saved.MemoryRequestMi)
	require.Equal(t, request.GetMemoryLimitMi(), saved.MemoryLimitMi)
	require.Equal(t, response.GetStatus(), saved.Status)

	event := findEventByDeploymentID(t, deploymentID)
	require.NotEqual(t, uuid.Nil(), event.ID)
	require.Equal(t, deploymentID, event.DeploymentID)
	require.Equal(t, int64(1), event.Version)
	require.Equal(t, "DeploymentCreated", event.Type)
	require.Equal(t, 1, event.SchemaVersion)

	var payload deployment.DeploymentCreatedPayload
	require.NoError(t, json.Unmarshal(event.Payload, &payload))
	require.Equal(t, uuid.MustParse(request.GetRequestId()), payload.RequestID)
	require.Equal(t, uuid.MustParse(request.GetProjectId()), payload.ProjectID)
	require.Equal(t, uuid.MustParse(request.GetApplicationId()), payload.ApplicationID)
	require.Equal(t, request.GetImage(), payload.Image)
	require.Equal(t, request.GetReplicas(), payload.Replicas)
	require.Equal(t, request.GetContainerPort(), payload.ContainerPort)
	require.Equal(t, request.GetCpuRequestM(), payload.CPURequestM)
	require.Equal(t, request.GetCpuLimitM(), payload.CPULimitM)
	require.Equal(t, request.GetMemoryRequestMi(), payload.MemoryRequestMi)
	require.Equal(t, request.GetMemoryLimitMi(), payload.MemoryLimitMi)

	processed := findProcessedRequest(t, request.GetRequestId())
	require.Equal(t, "deployment.create", processed.Operation)
	require.Equal(t, uuid.MustParse(request.GetRequestId()), processed.RequestID)
	require.Equal(t, deploymentID, processed.DeploymentID)
	require.Len(t, processed.Fingerprint, 32)
	assertWriteModelCounts(t, 1)
}

func TestCreateDeploymentAcceptsBoundaryValues(t *testing.T) {
	testCases := []struct {
		name   string
		change func(*deploymentv1.CreateDeploymentRequest)
	}{
		{
			name: "minimum container port and equal resource limits",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ContainerPort = 1
				request.CpuRequestM = 1
				request.CpuLimitM = 1
				request.MemoryRequestMi = 1
				request.MemoryLimitMi = 1
			},
		},
		{
			name: "maximum container port",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ContainerPort = 65535
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testutil.ResetDB(t, testDB)
			client := startTestGRPCServer(t)
			request := validCreateDeploymentRequest()
			testCase.change(request)

			response, err := callCreateDeployment(t, client, request)

			require.NoError(t, err)
			require.Equal(t, "QUEUED", response.GetStatus())
			assertWriteModelCounts(t, 1)
		})
	}
}

func TestCreateDeploymentRejectsInvalidRequest(t *testing.T) {
	testCases := []struct {
		name            string
		change          func(*deploymentv1.CreateDeploymentRequest)
		expectedMessage string
	}{
		{
			name: "missing request ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.RequestId = ""
			},
			expectedMessage: "invalid request_id",
		},
		{
			name: "malformed request ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.RequestId = "not-a-uuid"
			},
			expectedMessage: "invalid request_id",
		},
		{
			name: "nil request ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.RequestId = uuid.Nil().String()
			},
			expectedMessage: "invalid request_id",
		},
		{
			name: "missing application ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ApplicationId = ""
			},
			expectedMessage: "invalid application_id",
		},
		{
			name: "malformed application ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ApplicationId = "not-a-uuid"
			},
			expectedMessage: "invalid application_id",
		},
		{
			name: "nil application ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ApplicationId = uuid.Nil().String()
			},
			expectedMessage: "invalid application_id",
		},
		{
			name: "missing project ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ProjectId = ""
			},
			expectedMessage: "invalid project_id",
		},
		{
			name: "malformed project ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ProjectId = "not-a-uuid"
			},
			expectedMessage: "invalid project_id",
		},
		{
			name: "nil project ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ProjectId = uuid.Nil().String()
			},
			expectedMessage: "invalid project_id",
		},
		{
			name: "empty image",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.Image = ""
			},
			expectedMessage: "image cannot be empty: invalid deployment",
		},
		{
			name: "blank image",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.Image = "   "
			},
			expectedMessage: "image cannot be empty: invalid deployment",
		},
		{
			name: "zero replicas",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.Replicas = 0
			},
			expectedMessage: "replicas must be at least 1 (got 0): invalid deployment",
		},
		{
			name: "negative replicas",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.Replicas = -1
			},
			expectedMessage: "replicas must be at least 1 (got -1): invalid deployment",
		},
		{
			name: "zero container port",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ContainerPort = 0
			},
			expectedMessage: "container port must be between 1 and 65535 (got 0): invalid deployment",
		},
		{
			name: "negative container port",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ContainerPort = -1
			},
			expectedMessage: "container port must be between 1 and 65535 (got -1): invalid deployment",
		},
		{
			name: "container port above maximum",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ContainerPort = 65536
			},
			expectedMessage: "container port must be between 1 and 65535 (got 65536): invalid deployment",
		},
		{
			name: "zero CPU request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.CpuRequestM = 0
			},
			expectedMessage: "cpu request must be greater than 0 (got 0): invalid deployment",
		},
		{
			name: "negative CPU request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.CpuRequestM = -1
			},
			expectedMessage: "cpu request must be greater than 0 (got -1): invalid deployment",
		},
		{
			name: "CPU limit below request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.CpuLimitM = request.CpuRequestM - 1
			},
			expectedMessage: "cpu limit (99m) cannot be less than cpu request (100m): invalid deployment",
		},
		{
			name: "zero memory request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.MemoryRequestMi = 0
			},
			expectedMessage: "memory request must be greater than 0 (got 0): invalid deployment",
		},
		{
			name: "negative memory request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.MemoryRequestMi = -1
			},
			expectedMessage: "memory request must be greater than 0 (got -1): invalid deployment",
		},
		{
			name: "memory limit below request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.MemoryLimitMi = request.MemoryRequestMi - 1
			},
			expectedMessage: "memory limit (255Mi) cannot be less than memory request (256Mi): invalid deployment",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testutil.ResetDB(t, testDB)
			client := startTestGRPCServer(t)
			request := validCreateDeploymentRequest()
			testCase.change(request)

			response, err := callCreateDeployment(t, client, request)

			require.Nil(t, response)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Equal(t, testCase.expectedMessage, status.Convert(err).Message())
			assertWriteModelCounts(t, 0)
		})
	}
}

func TestCreateDeploymentIsIdempotent(t *testing.T) {
	testutil.ResetDB(t, testDB)
	client := startTestGRPCServer(t)
	request := validCreateDeploymentRequest()

	first, firstErr := callCreateDeployment(t, client, request)
	second, secondErr := callCreateDeployment(t, client, request)

	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.Equal(t, first, second)
	assertWriteModelCounts(t, 1)
}

func TestCreateDeploymentRejectsIdempotencyConflict(t *testing.T) {
	testCases := []struct {
		name   string
		change func(*deploymentv1.CreateDeploymentRequest)
	}{
		{
			name: "different project ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ProjectId = uuid.New().String()
			},
		},
		{
			name: "different application ID",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ApplicationId = uuid.New().String()
			},
		},
		{
			name: "different image",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.Image = "caddy:2.10"
			},
		},
		{
			name: "different replicas",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.Replicas++
			},
		},
		{
			name: "different container port",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.ContainerPort++
			},
		},
		{
			name: "different CPU request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.CpuRequestM++
			},
		},
		{
			name: "different CPU limit",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.CpuLimitM++
			},
		},
		{
			name: "different memory request",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.MemoryRequestMi++
			},
		},
		{
			name: "different memory limit",
			change: func(request *deploymentv1.CreateDeploymentRequest) {
				request.MemoryLimitMi++
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testutil.ResetDB(t, testDB)
			client := startTestGRPCServer(t)
			firstRequest := validCreateDeploymentRequest()
			conflictingRequest := validCreateDeploymentRequest()
			testCase.change(conflictingRequest)

			first, firstErr := callCreateDeployment(t, client, firstRequest)
			second, secondErr := callCreateDeployment(t, client, conflictingRequest)

			require.NoError(t, firstErr)
			require.NotNil(t, first)
			require.Nil(t, second)
			require.Equal(t, codes.AlreadyExists, status.Code(secondErr))
			require.Equal(
				t,
				"request_id already used for a different deployment",
				status.Convert(secondErr).Message(),
			)
			require.Equal(
				t,
				firstRequest.GetImage(),
				findDeploymentByRequestID(t, firstRequest.GetRequestId()).Image,
			)
			assertWriteModelCounts(t, 1)
		})
	}
}

func TestCreateDeploymentCreatesIndependentDeployments(t *testing.T) {
	testutil.ResetDB(t, testDB)
	client := startTestGRPCServer(t)
	firstRequest := validCreateDeploymentRequest()
	secondRequest := validCreateDeploymentRequest()
	secondRequest.RequestId = uuid.New().String()

	first, firstErr := callCreateDeployment(t, client, firstRequest)
	second, secondErr := callCreateDeployment(t, client, secondRequest)

	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NotEqual(t, first.GetDeploymentId(), second.GetDeploymentId())
	assertWriteModelCounts(t, 2)
}

func TestCreateDeploymentHandlesConcurrentIdenticalRequests(t *testing.T) {
	testutil.ResetDB(t, testDB)
	client := startTestGRPCServer(t)
	request := validCreateDeploymentRequest()

	const requestCount = 16
	responses := make([]*deploymentv1.CreateDeploymentResponse, requestCount)
	errs := make([]error, requestCount)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup

	waitGroup.Add(requestCount)
	for index := range requestCount {
		go func() {
			defer waitGroup.Done()
			<-start

			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			defer cancel()
			responses[index], errs[index] = client.CreateDeployment(ctx, request)
		}()
	}

	close(start)
	waitGroup.Wait()

	for index := range requestCount {
		require.NoError(t, errs[index], "request %d", index)
		require.NotNil(t, responses[index], "request %d", index)
		require.Equal(t, responses[0], responses[index], "request %d", index)
	}
	assertWriteModelCounts(t, 1)
}

func TestCreateDeploymentHandlesConcurrentConflictingRequests(t *testing.T) {
	testutil.ResetDB(t, testDB)
	client := startTestGRPCServer(t)
	requests := []*deploymentv1.CreateDeploymentRequest{
		validCreateDeploymentRequest(),
		validCreateDeploymentRequest(),
	}
	requests[1].Image = "caddy:2.10"

	responses := make([]*deploymentv1.CreateDeploymentResponse, len(requests))
	errs := make([]error, len(requests))
	start := make(chan struct{})
	var waitGroup sync.WaitGroup

	waitGroup.Add(len(requests))
	for index, request := range requests {
		go func() {
			defer waitGroup.Done()
			<-start

			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			defer cancel()
			responses[index], errs[index] = client.CreateDeployment(ctx, request)
		}()
	}

	close(start)
	waitGroup.Wait()

	successIndex := -1
	conflictIndex := -1
	for index := range requests {
		switch status.Code(errs[index]) {
		case codes.OK:
			require.NotNil(t, responses[index])
			successIndex = index
		case codes.AlreadyExists:
			require.Nil(t, responses[index])
			conflictIndex = index
		default:
			require.Failf(t, "unexpected gRPC result", "request %d returned %v", index, errs[index])
		}
	}

	require.NotEqual(t, -1, successIndex, "exactly one request should succeed")
	require.NotEqual(t, -1, conflictIndex, "exactly one request should conflict")
	require.NotEqual(t, successIndex, conflictIndex)
	require.Equal(
		t,
		requests[successIndex].GetImage(),
		findDeploymentByRequestID(t, requests[successIndex].GetRequestId()).Image,
	)
	assertWriteModelCounts(t, 1)
}

func TestCreateDeploymentRollsBackEventAndIdempotencyClaimWhenProjectionFails(
	t *testing.T,
) {
	testutil.ResetDB(t, testDB)

	const (
		constraintName = "chk_deployments_integration_forced_failure"
		failureImage   = "integration-force-projection-failure"
	)

	_, err := testDB.Exec(
		context.Background(),
		`ALTER TABLE deployments
		 ADD CONSTRAINT `+constraintName+`
		 CHECK (image <> '`+failureImage+`')`,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := testDB.Exec(
			context.Background(),
			"ALTER TABLE deployments DROP CONSTRAINT IF EXISTS "+constraintName,
		)
		require.NoError(t, cleanupErr)
	})

	client := startTestGRPCServer(t)
	request := validCreateDeploymentRequest()
	request.Image = failureImage

	response, requestErr := callCreateDeployment(t, client, request)

	require.Nil(t, response)
	require.Equal(t, codes.Internal, status.Code(requestErr))
	require.Equal(t, "failed to create deployment", status.Convert(requestErr).Message())
	assertWriteModelCounts(t, 0)
}

func validCreateDeploymentRequest() *deploymentv1.CreateDeploymentRequest {
	return &deploymentv1.CreateDeploymentRequest{
		RequestId:       "11111111-1111-4111-8111-111111111111",
		ApplicationId:   "22222222-2222-4222-8222-222222222222",
		ProjectId:       "33333333-3333-4333-8333-333333333333",
		Image:           "nginx:1.27",
		Replicas:        3,
		ContainerPort:   8080,
		CpuRequestM:     100,
		CpuLimitM:       250,
		MemoryRequestMi: 256,
		MemoryLimitMi:   512,
	}
}

func callCreateDeployment(
	t *testing.T,
	client deploymentv1.DeploymentServiceClient,
	request *deploymentv1.CreateDeploymentRequest,
) (*deploymentv1.CreateDeploymentResponse, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	return client.CreateDeployment(ctx, request)
}

func findDeploymentByRequestID(t *testing.T, requestID string) savedDeployment {
	t.Helper()

	var saved savedDeployment
	err := testDB.QueryRow(
		context.Background(),
		`SELECT id, request_id, project_id, application_id, image, replicas,
		        container_port, cpu_request_m, cpu_limit_m, memory_request_mi,
		        memory_limit_mi, status
		 FROM deployments
		 WHERE request_id = $1`,
		requestID,
	).Scan(
		&saved.ID,
		&saved.RequestID,
		&saved.ProjectID,
		&saved.ApplicationID,
		&saved.Image,
		&saved.Replicas,
		&saved.ContainerPort,
		&saved.CPURequestM,
		&saved.CPULimitM,
		&saved.MemoryRequestMi,
		&saved.MemoryLimitMi,
		&saved.Status,
	)
	require.NoError(t, err)

	return saved
}

func findEventByDeploymentID(t *testing.T, deploymentID uuid.UUID) savedEvent {
	t.Helper()

	var saved savedEvent
	err := testDB.QueryRow(
		context.Background(),
		`SELECT id, deployment_id, version, type, schema_version, payload
		 FROM deployment_events
		 WHERE deployment_id = $1`,
		deploymentID,
	).Scan(
		&saved.ID,
		&saved.DeploymentID,
		&saved.Version,
		&saved.Type,
		&saved.SchemaVersion,
		&saved.Payload,
	)
	require.NoError(t, err)

	return saved
}

func findProcessedRequest(t *testing.T, requestID string) savedProcessedRequest {
	t.Helper()

	var saved savedProcessedRequest
	err := testDB.QueryRow(
		context.Background(),
		`SELECT operation, request_id, deployment_id, fingerprint
		 FROM processed_requests
		 WHERE operation = $1 AND request_id = $2`,
		"deployment.create",
		requestID,
	).Scan(
		&saved.Operation,
		&saved.RequestID,
		&saved.DeploymentID,
		&saved.Fingerprint,
	)
	require.NoError(t, err)

	return saved
}

func assertWriteModelCounts(t *testing.T, expected int) {
	t.Helper()

	require.Equal(t, expected, deploymentCount(t), "deployment projection count")
	require.Equal(t, expected, eventCount(t), "deployment event count")
	require.Equal(t, expected, processedRequestCount(t), "processed request count")
}

func deploymentCount(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, testDB.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM deployments",
	).Scan(&count))

	return count
}

func eventCount(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, testDB.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM deployment_events",
	).Scan(&count))

	return count
}

func processedRequestCount(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, testDB.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM processed_requests",
	).Scan(&count))

	return count
}
