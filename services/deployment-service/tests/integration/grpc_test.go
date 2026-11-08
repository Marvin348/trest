package integration

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/idempotency"
	deploymentgrpc "example.com/trest/services/deployment-service/internal/transport/grpc"
)

func startTestGRPCServer(
	t *testing.T,
) deploymentv1.DeploymentServiceClient {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	repo := deployment.NewRepository(testDB)
	eventRepo := eventstore.NewRepository(testDB)
	idempotencyRepo := idempotency.NewRepository(testDB)
	service := deployment.NewService(testDB, repo, eventRepo, idempotencyRepo)
	handler := deploymentgrpc.NewServer(service)

	server := grpc.NewServer()

	deploymentv1.RegisterDeploymentServiceServer(
		server,
		handler,
	)

	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
	})

	return deploymentv1.NewDeploymentServiceClient(conn)
}
