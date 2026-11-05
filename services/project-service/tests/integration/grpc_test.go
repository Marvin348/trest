package integration

import (
	"context"
	"sync"
	"testing"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testDeploymentServer struct {
	deploymentv1.UnimplementedDeploymentServiceServer

	mu sync.Mutex

	createDeployment func(
		context.Context,
		*deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error)
}

func (s *testDeploymentServer) configureCreateDeployment(
	t *testing.T,
	handler func(
		context.Context,
		*deploymentv1.CreateDeploymentRequest,
	) (*deploymentv1.CreateDeploymentResponse, error),
) {
	t.Helper()

	s.mu.Lock()
	s.createDeployment = handler
	s.mu.Unlock()

	t.Cleanup(func() {
		s.mu.Lock()
		s.createDeployment = nil
		s.mu.Unlock()
	})
}

func (s *testDeploymentServer) CreateDeployment(
	ctx context.Context,
	req *deploymentv1.CreateDeploymentRequest,
) (*deploymentv1.CreateDeploymentResponse, error) {
	s.mu.Lock()
	fn := s.createDeployment
	s.mu.Unlock()

	if fn == nil {
		return nil, status.Error(
			codes.Unimplemented,
			"CreateDeployment not configured",
		)
	}

	return fn(ctx, req)
}
