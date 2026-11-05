package deployment

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"uuid"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/project-service/internal/apperror"
	"example.com/trest/services/project-service/internal/application"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeApplicationRepository struct {
	findDeployConfigByID func(
		ctx context.Context,
		id uuid.UUID,
	) (application.ApplicationDeployConfig, error)
	calls int
}

func (f *fakeApplicationRepository) FindDeployConfigByID(
	ctx context.Context,
	id uuid.UUID,
) (application.ApplicationDeployConfig, error) {
	f.calls++
	return f.findDeployConfigByID(ctx, id)
}

type fakeDeploymentClient struct {
	createDeployment func(
		ctx context.Context,
		request *deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error)
	calls int
}

func (f *fakeDeploymentClient) CreateDeployment(
	ctx context.Context,
	request *deploymentv1.CreateDeploymentRequest,
	_ ...grpc.CallOption,
) (*deploymentv1.CreateDeploymentResponse, error) {
	f.calls++
	return f.createDeployment(ctx, request)
}

func TestServiceDeploy(t *testing.T) {
	applicationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	requestID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	appConfig := application.ApplicationDeployConfig{
		ID:              applicationID,
		ProjectID:       uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		Image:           "nginx:1.27",
		Replicas:        3,
		ContainerPort:   8080,
		CPURequestM:     100,
		CPULimitM:       250,
		MemoryRequestMi: 256,
		MemoryLimitMi:   512,
	}

	t.Run("passes application configuration to CreateDeployment", func(t *testing.T) {
		expectedResponse := &deploymentv1.CreateDeploymentResponse{
			DeploymentId: "44444444-4444-4444-8444-444444444444",
			Status:       "QUEUED",
		}

		repo := &fakeApplicationRepository{
			findDeployConfigByID: func(
				_ context.Context,
				id uuid.UUID,
			) (application.ApplicationDeployConfig, error) {
				require.Equal(t, applicationID, id)
				return appConfig, nil
			},
		}

		client := &fakeDeploymentClient{
			createDeployment: func(
				ctx context.Context,
				request *deploymentv1.CreateDeploymentRequest,
			) (*deploymentv1.CreateDeploymentResponse, error) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok, "gRPC context should have a deadline")
				require.WithinDuration(t, time.Now().Add(3*time.Second), deadline, 250*time.Millisecond)

				require.Equal(t, requestID.String(), request.GetRequestId())
				require.Equal(t, appConfig.ID.String(), request.GetApplicationId())
				require.Equal(t, appConfig.ProjectID.String(), request.GetProjectId())
				require.Equal(t, appConfig.Image, request.GetImage())
				require.Equal(t, appConfig.Replicas, request.GetReplicas())
				require.Equal(t, appConfig.ContainerPort, request.GetContainerPort())
				require.Equal(t, appConfig.CPURequestM, request.GetCpuRequestM())
				require.Equal(t, appConfig.CPULimitM, request.GetCpuLimitM())
				require.Equal(t, appConfig.MemoryRequestMi, request.GetMemoryRequestMi())
				require.Equal(t, appConfig.MemoryLimitMi, request.GetMemoryLimitMi())

				return expectedResponse, nil
			},
		}

		service := NewService(repo, client)
		response, err := service.Deploy(context.Background(), applicationID, requestID)

		require.NoError(t, err)
		require.Same(t, expectedResponse, response)
		require.Equal(t, 1, repo.calls)
		require.Equal(t, 1, client.calls)
	})

	t.Run("maps application not found to NotFound", func(t *testing.T) {
		repo := &fakeApplicationRepository{
			findDeployConfigByID: func(
				_ context.Context,
				_ uuid.UUID,
			) (application.ApplicationDeployConfig, error) {
				return application.ApplicationDeployConfig{},
					fmt.Errorf("find application: %w", application.ErrNotFound)
			},
		}

		client := &fakeDeploymentClient{
			createDeployment: func(
				_ context.Context,
				_ *deploymentv1.CreateDeploymentRequest,
			) (*deploymentv1.CreateDeploymentResponse, error) {
				t.Fatal("CreateDeployment must not be called when the application is missing")
				return nil, nil
			},
		}

		service := NewService(repo, client)
		response, err := service.Deploy(context.Background(), applicationID, requestID)

		require.Nil(t, response)
		requireAppError(t, err, apperror.NotFound, "application not found")
		require.Equal(t, 1, repo.calls)
		require.Zero(t, client.calls)
	})

	t.Run("wraps unexpected repository error", func(t *testing.T) {
		repositoryErr := errors.New("database unavailable")
		repo := &fakeApplicationRepository{
			findDeployConfigByID: func(
				_ context.Context,
				_ uuid.UUID,
			) (application.ApplicationDeployConfig, error) {
				return application.ApplicationDeployConfig{}, repositoryErr
			},
		}

		client := &fakeDeploymentClient{
			createDeployment: func(
				_ context.Context,
				_ *deploymentv1.CreateDeploymentRequest,
			) (*deploymentv1.CreateDeploymentResponse, error) {
				t.Fatal("CreateDeployment must not be called after a repository error")
				return nil, nil
			},
		}

		service := NewService(repo, client)
		response, err := service.Deploy(context.Background(), applicationID, requestID)

		require.Nil(t, response)
		require.ErrorContains(t, err, "get application")
		require.ErrorIs(t, err, repositoryErr)
		require.Equal(t, 1, repo.calls)
		require.Zero(t, client.calls)
	})

	for _, testCase := range []struct {
		name         string
		grpcCode     codes.Code
		appErrorCode apperror.Code
		message      string
	}{
		{
			name:         "maps InvalidArgument to Validation",
			grpcCode:     codes.InvalidArgument,
			appErrorCode: apperror.Validation,
			message:      "Invalid deployment configuration",
		},
		{
			name:         "maps AlreadyExists to Conflict",
			grpcCode:     codes.AlreadyExists,
			appErrorCode: apperror.Conflict,
			message:      "Deployment request already exists",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			grpcErr := status.Error(testCase.grpcCode, "gRPC failure")
			repo := successfulApplicationRepository(appConfig)
			client := &fakeDeploymentClient{
				createDeployment: func(
					_ context.Context,
					_ *deploymentv1.CreateDeploymentRequest,
				) (*deploymentv1.CreateDeploymentResponse, error) {
					return nil, grpcErr
				},
			}

			service := NewService(repo, client)
			response, err := service.Deploy(context.Background(), applicationID, requestID)

			require.Nil(t, response)
			requireAppError(t, err, testCase.appErrorCode, testCase.message)
			require.ErrorIs(t, err, grpcErr)
			require.Equal(t, 1, repo.calls)
			require.Equal(t, 1, client.calls)
		})
	}

	for _, testCase := range []struct {
		name     string
		grpcCode codes.Code
	}{
		{name: "wraps unavailable gRPC service", grpcCode: codes.Unavailable},
		{name: "wraps gRPC timeout", grpcCode: codes.DeadlineExceeded},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			grpcErr := status.Error(testCase.grpcCode, "gRPC failure")
			repo := successfulApplicationRepository(appConfig)
			client := &fakeDeploymentClient{
				createDeployment: func(
					_ context.Context,
					_ *deploymentv1.CreateDeploymentRequest,
				) (*deploymentv1.CreateDeploymentResponse, error) {
					return nil, grpcErr
				},
			}

			service := NewService(repo, client)
			response, err := service.Deploy(context.Background(), applicationID, requestID)

			require.Nil(t, response)
			require.ErrorContains(t, err, "create deployment via gRPC")
			require.Equal(t, testCase.grpcCode, status.Code(err))

			var appErr *apperror.AppError
			require.False(t, errors.As(err, &appErr))
			require.Equal(t, 1, repo.calls)
			require.Equal(t, 1, client.calls)
		})
	}
}

func successfulApplicationRepository(
	config application.ApplicationDeployConfig,
) *fakeApplicationRepository {
	return &fakeApplicationRepository{
		findDeployConfigByID: func(
			_ context.Context,
			_ uuid.UUID,
		) (application.ApplicationDeployConfig, error) {
			return config, nil
		},
	}
}

func requireAppError(
	t *testing.T,
	err error,
	code apperror.Code,
	message string,
) {
	t.Helper()

	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, code, appErr.Code)
	require.Equal(t, message, appErr.Message)
}
