package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"example.com/trest/services/project-service/internal/app"
	"example.com/trest/services/project-service/internal/testutil"
)

type createApplicationResponse struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	PlanID          string    `json:"plan_id"`
	Image           string    `json:"image"`
	Name            string    `json:"name"`
	Replicas        int       `json:"replicas"`
	CPURequestM     int       `json:"cpu_request_m"`
	CPULimitM       int       `json:"cpu_limit_m"`
	MemoryRequestMi int       `json:"memory_request_mi"`
	MemoryLimitMi   int       `json:"memory_limit_mi"`
	ContainerPort   int       `json:"container_port"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestCreateApplication(t *testing.T) {
	testutil.ResetDB(t, testDB)

	projectID := testutil.CreateTestProject(t, testDB)
	planID := testutil.CreateTestPlan(t, testDB)
	mux := newTestRouter()

	rec := createApplicationRequest(t, mux, projectID, map[string]any{
		"name":           "Backend",
		"image":          "nginx:latest",
		"plan_id":        planID,
		"container_port": 8080,
	})

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var response createApplicationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))

	require.NotEmpty(t, response.ID)
	require.Equal(t, projectID, response.ProjectID)
	require.Equal(t, planID, response.PlanID)
	require.Equal(t, "Backend", response.Name)
	require.Equal(t, "nginx:latest", response.Image)
	require.Equal(t, 1, response.Replicas)
	require.Equal(t, 100, response.CPURequestM)
	require.Equal(t, 200, response.CPULimitM)
	require.Equal(t, 128, response.MemoryRequestMi)
	require.Equal(t, 256, response.MemoryLimitMi)
	require.Equal(t, 8080, response.ContainerPort)
	require.False(t, response.CreatedAt.IsZero())
	require.False(t, response.UpdatedAt.IsZero())

	var saved createApplicationResponse
	err := testDB.QueryRow(
		context.Background(),
		`SELECT id, project_id, plan_id, image, name, replicas,
                cpu_request_m, cpu_limit_m, memory_request_mi,
                memory_limit_mi, container_port, created_at, updated_at
         FROM applications
         WHERE id = $1`,
		response.ID,
	).Scan(
		&saved.ID,
		&saved.ProjectID,
		&saved.PlanID,
		&saved.Image,
		&saved.Name,
		&saved.Replicas,
		&saved.CPURequestM,
		&saved.CPULimitM,
		&saved.MemoryRequestMi,
		&saved.MemoryLimitMi,
		&saved.ContainerPort,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	require.NoError(t, err)
	require.Equal(t, response, saved)
}

func TestCreateApplicationConflict(t *testing.T) {
	testutil.ResetDB(t, testDB)

	projectID := testutil.CreateTestProject(t, testDB)
	planID := testutil.CreateTestPlan(t, testDB)
	mux := newTestRouter()
	body := validApplicationBody(planID)

	first := createApplicationRequest(t, mux, projectID, body)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	duplicate := createApplicationRequest(t, mux, projectID, body)
	require.Equal(t, http.StatusConflict, duplicate.Code, duplicate.Body.String())
	assertErrorResponse(
		t,
		duplicate,
		"CONFLICT",
		"Application already exists",
	)
	require.Equal(t, 1, applicationCount(t))
}

func TestCreateApplicationValidation(t *testing.T) {
	tests := []struct {
		name           string
		changeBody     func(map[string]any)
		expectedStatus int
	}{
		{
			name: "empty name",
			changeBody: func(body map[string]any) {
				body["name"] = ""
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "name too short",
			changeBody: func(body map[string]any) {
				body["name"] = "AB"
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "minimum values are accepted",
			changeBody: func(body map[string]any) {
				body["name"] = "ABC"
				body["container_port"] = 1
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "maximum values are accepted",
			changeBody: func(body map[string]any) {
				body["name"] = strings.Repeat("A", 50)
				body["container_port"] = 65535
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "name too long",
			changeBody: func(body map[string]any) {
				body["name"] = strings.Repeat("A", 51)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty image",
			changeBody: func(body map[string]any) {
				body["image"] = ""
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty plan ID",
			changeBody: func(body map[string]any) {
				body["plan_id"] = ""
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid plan ID",
			changeBody: func(body map[string]any) {
				body["plan_id"] = "not-a-uuid"
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "zero container port",
			changeBody: func(body map[string]any) {
				body["container_port"] = 0
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "negative container port",
			changeBody: func(body map[string]any) {
				body["container_port"] = -1
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "container port too high",
			changeBody: func(body map[string]any) {
				body["container_port"] = 65536
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.ResetDB(t, testDB)

			projectID := testutil.CreateTestProject(t, testDB)
			planID := testutil.CreateTestPlan(t, testDB)
			body := validApplicationBody(planID)
			tt.changeBody(body)

			rec := createApplicationRequest(t, newTestRouter(), projectID, body)
			require.Equal(t, tt.expectedStatus, rec.Code, rec.Body.String())

			if tt.expectedStatus == http.StatusBadRequest {
				assertErrorResponse(
					t,
					rec,
					"VALIDATION_ERROR",
					"Application validation failed",
				)
				require.Equal(t, 0, applicationCount(t))
				return
			}

			require.Equal(t, 1, applicationCount(t))
		})
	}
}

func TestCreateApplicationRejectsInvalidProjectID(t *testing.T) {
	testutil.ResetDB(t, testDB)

	planID := testutil.CreateTestPlan(t, testDB)
	rec := createApplicationRequest(
		t,
		newTestRouter(),
		"not-a-uuid",
		validApplicationBody(planID),
	)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assertErrorResponse(t, rec, "VALIDATION_ERROR", "Invalid projectId")
	require.Equal(t, 0, applicationCount(t))
}

func TestCreateApplicationRejectsInvalidJSON(t *testing.T) {
	testutil.ResetDB(t, testDB)

	projectID := testutil.CreateTestProject(t, testDB)
	req := httptest.NewRequest(
		http.MethodPost,
		"/projects/"+projectID+"/applications",
		strings.NewReader(`{"name":`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assertErrorResponse(t, rec, "VALIDATION_ERROR", "Invalid JSON body")
	require.Equal(t, 0, applicationCount(t))
}

func TestCreateApplicationReturnsNotFoundForUnknownPlan(t *testing.T) {
	testutil.ResetDB(t, testDB)

	projectID := testutil.CreateTestProject(t, testDB)
	rec := createApplicationRequest(
		t,
		newTestRouter(),
		projectID,
		validApplicationBody("11111111-1111-4111-8111-111111111111"),
	)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assertErrorResponse(t, rec, "NOT_FOUND", "Plan not found")
	require.Equal(t, 0, applicationCount(t))
}

func TestCreateApplicationReturnsNotFoundForUnknownProject(t *testing.T) {
	testutil.ResetDB(t, testDB)

	planID := testutil.CreateTestPlan(t, testDB)
	rec := createApplicationRequest(
		t,
		newTestRouter(),
		"22222222-2222-4222-8222-222222222222",
		validApplicationBody(planID),
	)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assertErrorResponse(t, rec, "NOT_FOUND", "Project not found")
	require.Equal(t, 0, applicationCount(t))
}

func newTestRouter() *http.ServeMux {
	mux := http.NewServeMux()
	app.RegisterRoutes(mux, testDB)
	return mux
}

func validApplicationBody(planID string) map[string]any {
	return map[string]any{
		"name":           "Backend",
		"image":          "nginx:latest",
		"plan_id":        planID,
		"container_port": 8080,
	}
}

func createApplicationRequest(
	t *testing.T,
	mux http.Handler,
	projectID string,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()

	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPost,
		"/projects/"+projectID+"/applications",
		bytes.NewReader(jsonBody),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func assertErrorResponse(
	t *testing.T,
	rec *httptest.ResponseRecorder,
	expectedCode string,
	expectedMessage string,
) {
	t.Helper()

	var response errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, expectedCode, response.Error.Code)
	require.Equal(t, expectedMessage, response.Error.Message)
}

func applicationCount(t *testing.T) int {
	t.Helper()

	var count int
	err := testDB.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM applications",
	).Scan(&count)
	require.NoError(t, err)
	return count
}
