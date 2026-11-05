package deployment

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/project-service/internal/apperror"
	"example.com/trest/services/project-service/internal/application"
)

type Service struct {
	applicationRepo applicationRepository
	client          deploymentv1.DeploymentServiceClient
}

type applicationRepository interface {
	FindDeployConfigByID(
		ctx context.Context,
		id uuid.UUID,
	) (application.ApplicationDeployConfig, error)
}

func NewService(applicationRepo applicationRepository,
	client deploymentv1.DeploymentServiceClient) *Service {
	return &Service{
		applicationRepo: applicationRepo,
		client:          client}
}

func (s *Service) Deploy(ctx context.Context,
	applicationID uuid.UUID,
	requestID uuid.UUID) (*deploymentv1.CreateDeploymentResponse, error) {

	appConfig, err := s.applicationRepo.FindDeployConfigByID(ctx, applicationID)
	if errors.Is(err, application.ErrNotFound) {
		return nil,
			&apperror.AppError{
				Code:    apperror.NotFound,
				Message: "application not found",
			}
	}
	if err != nil {
		return nil,
			fmt.Errorf("get application: %w", err)
	}

	request := &deploymentv1.CreateDeploymentRequest{
		RequestId:       requestID.String(),
		ApplicationId:   appConfig.ID.String(),
		ProjectId:       appConfig.ProjectID.String(),
		Image:           appConfig.Image,
		Replicas:        appConfig.Replicas,
		ContainerPort:   appConfig.ContainerPort,
		CpuRequestM:     appConfig.CPURequestM,
		CpuLimitM:       appConfig.CPULimitM,
		MemoryRequestMi: appConfig.MemoryRequestMi,
		MemoryLimitMi:   appConfig.MemoryLimitMi,
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	response, err := s.client.CreateDeployment(rpcCtx, request)
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			return nil,
				&apperror.AppError{
					Code:    apperror.Validation,
					Message: "Invalid deployment configuration",
					Cause:   err,
				}

		case codes.AlreadyExists:
			return nil,
				&apperror.AppError{
					Code:    apperror.Conflict,
					Message: "Deployment request already exists",
					Cause:   err,
				}

		default:
			return nil,
				fmt.Errorf("create deployment via gRPC: %w", err)
		}
	}

	return response, nil
}
