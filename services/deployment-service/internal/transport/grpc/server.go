package grpc

import (
	"context"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"

	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	deploymentv1.UnimplementedDeploymentServiceServer
	service *deployment.Service
}

func NewServer(service *deployment.Service) *Server {
	return &Server{
		service: service,
	}
}

func (s *Server) CreateDeployment(
	ctx context.Context,
	req *deploymentv1.CreateDeploymentRequest,
) (*deploymentv1.CreateDeploymentResponse, error) {

	params, err := parseCreateDeploymentRequest(req)
	if err != nil {
		return nil, err
	}

	result, err := s.service.Create(ctx, params)

	if err != nil {
		switch {
		case errors.Is(err, deployment.ErrInvalidDeployment):
			return nil, status.Error(codes.InvalidArgument, err.Error())

		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")

		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "deadline exceeded")

		case errors.Is(err, eventstore.ErrConcurrencyConflict):
			return nil, status.Error(
				codes.Aborted,
				"deployment was modified concurrently",
			)

		case errors.Is(err, deployment.ErrIdempotencyConflict):
			return nil, status.Error(codes.AlreadyExists,
				"request_id already used for a different deployment")
		}

		return nil, status.Error(codes.Internal, "failed to create deployment")
	}

	return &deploymentv1.CreateDeploymentResponse{
		DeploymentId: result.ID.String(),
		Status:       string(result.Status),
	}, nil
}
