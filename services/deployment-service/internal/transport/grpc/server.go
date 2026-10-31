package grpc

import (
	"context"

	deploymentv1 "example.com/trest/contracts/deployment/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	deploymentv1.UnimplementedDeploymentServiceServer
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) CreateDeployment(
	ctx context.Context,
	req *deploymentv1.CreateDeploymentRequest,
) (*deploymentv1.CreateDeploymentResponse, error) {
	return nil, status.Error(
		codes.Unimplemented,
		"CreateDeployment is not implemented yet")
}
