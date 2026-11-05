package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"uuid"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/project-service/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deploymentResponse struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
}

type deploymentFixture struct {
	ApplicationID string
	ProjectID     string
}

func TestDeployApplication(t *testing.T) {
	testutil.ResetDB(t, testDB)
	fixture := createDeploymentFixture(t)
	requestID := "22222222-2222-4222-8222-222222222222"
	receivedRequests := make(chan *deploymentv1.CreateDeploymentRequest, 1)

	testDeploymentGRPCServer.configureCreateDeployment(t, func(
		_ context.Context,
		request *deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error) {
		receivedRequests <- copyDeploymentRequest(request)

		return &deploymentv1.CreateDeploymentResponse{
			DeploymentId: "44444444-4444-4444-8444-444444444444",
			Status:       "QUEUED",
		}, nil
	})

	recorder := performDeployRequest(
		newTestRouter(),
		fixture.ApplicationID,
		requestID,
	)

	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

	response := decodeDeploymentResponse(t, recorder)
	require.Equal(t, "44444444-4444-4444-8444-444444444444", response.DeploymentID)
	require.Equal(t, "QUEUED", response.Status)

	received := <-receivedRequests
	require.Equal(t, requestID, received.GetRequestId())
	require.Equal(t, fixture.ApplicationID, received.GetApplicationId())
	require.Equal(t, fixture.ProjectID, received.GetProjectId())
	require.Equal(t, "nginx:1.27", received.GetImage())
	require.Equal(t, int32(3), received.GetReplicas())
	require.Equal(t, int32(8080), received.GetContainerPort())
	require.Equal(t, int32(100), received.GetCpuRequestM())
	require.Equal(t, int32(250), received.GetCpuLimitM())
	require.Equal(t, int32(256), received.GetMemoryRequestMi())
	require.Equal(t, int32(512), received.GetMemoryLimitMi())
}

func TestDeployApplicationRejectsInvalidRequest(t *testing.T) {
	testutil.ResetDB(t, testDB)
	fixture := createDeploymentFixture(t)
	validRequestID := "22222222-2222-4222-8222-222222222222"
	var grpcCalls atomic.Int32

	testDeploymentGRPCServer.configureCreateDeployment(t, func(
		_ context.Context,
		_ *deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error) {
		grpcCalls.Add(1)
		return &deploymentv1.CreateDeploymentResponse{}, nil
	})

	testCases := []struct {
		name            string
		applicationID   string
		idempotencyKey  string
		expectedStatus  int
		expectedCode    string
		expectedMessage string
	}{
		{
			name:            "malformed application ID",
			applicationID:   "not-a-uuid",
			idempotencyKey:  validRequestID,
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "VALIDATION_ERROR",
			expectedMessage: "Invalid applicationId",
		},
		{
			name:            "nil application ID",
			applicationID:   uuid.Nil().String(),
			idempotencyKey:  validRequestID,
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "VALIDATION_ERROR",
			expectedMessage: "Invalid applicationId",
		},
		{
			name:            "missing idempotency key",
			applicationID:   fixture.ApplicationID,
			idempotencyKey:  "",
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "VALIDATION_ERROR",
			expectedMessage: "Invalid Idempotency-Key",
		},
		{
			name:            "malformed idempotency key",
			applicationID:   fixture.ApplicationID,
			idempotencyKey:  "not-a-uuid",
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "VALIDATION_ERROR",
			expectedMessage: "Invalid Idempotency-Key",
		},
		{
			name:            "nil idempotency key",
			applicationID:   fixture.ApplicationID,
			idempotencyKey:  uuid.Nil().String(),
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "VALIDATION_ERROR",
			expectedMessage: "Invalid Idempotency-Key",
		},
		{
			name:            "unknown application",
			applicationID:   "55555555-5555-4555-8555-555555555555",
			idempotencyKey:  validRequestID,
			expectedStatus:  http.StatusNotFound,
			expectedCode:    "NOT_FOUND",
			expectedMessage: "application not found",
		},
	}

	router := newTestRouter()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := performDeployRequest(
				router,
				testCase.applicationID,
				testCase.idempotencyKey,
			)

			require.Equal(t, testCase.expectedStatus, recorder.Code, recorder.Body.String())
			assertErrorResponse(
				t,
				recorder,
				testCase.expectedCode,
				testCase.expectedMessage,
			)
		})
	}

	require.Zero(t, grpcCalls.Load(), "invalid requests must not reach gRPC")
}

func TestDeployApplicationRejectsWrongHTTPMethod(t *testing.T) {
	testutil.ResetDB(t, testDB)
	fixture := createDeploymentFixture(t)
	var grpcCalls atomic.Int32

	testDeploymentGRPCServer.configureCreateDeployment(t, func(
		_ context.Context,
		_ *deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error) {
		grpcCalls.Add(1)
		return &deploymentv1.CreateDeploymentResponse{}, nil
	})

	request := httptest.NewRequest(
		http.MethodGet,
		"/applications/"+fixture.ApplicationID+"/deploy",
		nil,
	)
	request.Header.Set("Idempotency-Key", "22222222-2222-4222-8222-222222222222")
	recorder := httptest.NewRecorder()

	newTestRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code, recorder.Body.String())
	require.Equal(t, http.MethodPost, recorder.Header().Get("Allow"))
	require.Zero(t, grpcCalls.Load())
}

func TestDeployApplicationMapsGRPCErrors(t *testing.T) {
	testutil.ResetDB(t, testDB)
	fixture := createDeploymentFixture(t)

	testCases := []struct {
		name            string
		grpcCode        codes.Code
		expectedStatus  int
		expectedCode    string
		expectedMessage string
	}{
		{
			name:            "invalid deployment configuration",
			grpcCode:        codes.InvalidArgument,
			expectedStatus:  http.StatusBadRequest,
			expectedCode:    "VALIDATION_ERROR",
			expectedMessage: "Invalid deployment configuration",
		},
		{
			name:            "idempotency conflict",
			grpcCode:        codes.AlreadyExists,
			expectedStatus:  http.StatusConflict,
			expectedCode:    "CONFLICT",
			expectedMessage: "Deployment request already exists",
		},
		{
			name:            "deployment service unavailable",
			grpcCode:        codes.Unavailable,
			expectedStatus:  http.StatusInternalServerError,
			expectedCode:    "INTERNAL_ERROR",
			expectedMessage: "Internal server error",
		},
		{
			name:            "deployment request timed out",
			grpcCode:        codes.DeadlineExceeded,
			expectedStatus:  http.StatusInternalServerError,
			expectedCode:    "INTERNAL_ERROR",
			expectedMessage: "Internal server error",
		},
		{
			name:            "deployment service internal error",
			grpcCode:        codes.Internal,
			expectedStatus:  http.StatusInternalServerError,
			expectedCode:    "INTERNAL_ERROR",
			expectedMessage: "Internal server error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var grpcCalls atomic.Int32
			testDeploymentGRPCServer.configureCreateDeployment(t, func(
				_ context.Context,
				_ *deploymentv1.CreateDeploymentRequest,
			) (*deploymentv1.CreateDeploymentResponse, error) {
				grpcCalls.Add(1)
				return nil, status.Error(testCase.grpcCode, "test gRPC error")
			})

			recorder := performDeployRequest(
				newTestRouter(),
				fixture.ApplicationID,
				uuid.New().String(),
			)

			require.Equal(t, testCase.expectedStatus, recorder.Code, recorder.Body.String())
			assertErrorResponse(
				t,
				recorder,
				testCase.expectedCode,
				testCase.expectedMessage,
			)
			require.Equal(t, int32(1), grpcCalls.Load())
		})
	}
}

func TestDeployApplicationPreservesIdempotencyKeyForRepeatedRequests(t *testing.T) {
	testutil.ResetDB(t, testDB)
	fixture := createDeploymentFixture(t)
	requestID := "22222222-2222-4222-8222-222222222222"

	var mu sync.Mutex
	responses := make(map[string]deploymentResponse)
	calls := make(map[string]int)

	testDeploymentGRPCServer.configureCreateDeployment(t, func(
		_ context.Context,
		request *deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error) {
		mu.Lock()
		defer mu.Unlock()

		response, exists := responses[request.GetRequestId()]
		if !exists {
			response = deploymentResponse{
				DeploymentID: "44444444-4444-4444-8444-444444444444",
				Status:       "QUEUED",
			}
			responses[request.GetRequestId()] = response
		}
		calls[request.GetRequestId()]++

		return &deploymentv1.CreateDeploymentResponse{
			DeploymentId: response.DeploymentID,
			Status:       response.Status,
		}, nil
	})

	router := newTestRouter()
	first := performDeployRequest(router, fixture.ApplicationID, requestID)
	second := performDeployRequest(router, fixture.ApplicationID, requestID)

	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())
	require.Equal(t, decodeDeploymentResponse(t, first), decodeDeploymentResponse(t, second))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, responses, 1)
	require.Equal(t, 2, calls[requestID])
}

func TestDeployApplicationPreservesIdempotencyKeyForConcurrentRequests(t *testing.T) {
	testutil.ResetDB(t, testDB)
	fixture := createDeploymentFixture(t)
	requestID := "22222222-2222-4222-8222-222222222222"

	var handlerMu sync.Mutex
	seenKeys := make(map[string]int)

	testDeploymentGRPCServer.configureCreateDeployment(t, func(
		_ context.Context,
		request *deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error) {
		handlerMu.Lock()
		seenKeys[request.GetRequestId()]++
		handlerMu.Unlock()

		return &deploymentv1.CreateDeploymentResponse{
			DeploymentId: "44444444-4444-4444-8444-444444444444",
			Status:       "QUEUED",
		}, nil
	})

	const requestCount = 32
	router := newTestRouter()
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, requestCount)

	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)

	for range requestCount {
		go func() {
			defer waitGroup.Done()
			<-start
			results <- performDeployRequest(router, fixture.ApplicationID, requestID)
		}()
	}

	close(start)
	waitGroup.Wait()
	close(results)

	for recorder := range results {
		require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
		response := decodeDeploymentResponse(t, recorder)
		require.Equal(t, "44444444-4444-4444-8444-444444444444", response.DeploymentID)
		require.Equal(t, "QUEUED", response.Status)
	}

	handlerMu.Lock()
	defer handlerMu.Unlock()
	require.Len(t, seenKeys, 1)
	require.Equal(t, requestCount, seenKeys[requestID])
}

func createDeploymentFixture(t *testing.T) deploymentFixture {
	t.Helper()

	projectID := testutil.CreateTestProject(t, testDB)
	planID := testutil.CreateTestPlan(t, testDB)
	applicationID := testutil.CreateTestApplication(t, testDB, projectID, planID)

	return deploymentFixture{
		ApplicationID: applicationID,
		ProjectID:     projectID,
	}
}

func performDeployRequest(
	handler http.Handler,
	applicationID string,
	idempotencyKey string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/applications/"+applicationID+"/deploy",
		nil,
	)
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func decodeDeploymentResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) deploymentResponse {
	t.Helper()

	var response deploymentResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	return response
}

func copyDeploymentRequest(
	request *deploymentv1.CreateDeploymentRequest,
) *deploymentv1.CreateDeploymentRequest {
	return &deploymentv1.CreateDeploymentRequest{
		RequestId:       request.GetRequestId(),
		ApplicationId:   request.GetApplicationId(),
		ProjectId:       request.GetProjectId(),
		Image:           request.GetImage(),
		Replicas:        request.GetReplicas(),
		ContainerPort:   request.GetContainerPort(),
		CpuRequestM:     request.GetCpuRequestM(),
		CpuLimitM:       request.GetCpuLimitM(),
		MemoryRequestMi: request.GetMemoryRequestMi(),
		MemoryLimitMi:   request.GetMemoryLimitMi(),
	}
}
