package integration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/project-service/internal/platform/deploymentgrpc"
	"example.com/trest/services/project-service/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

var (
	testDB                   *pgxpool.Pool
	testDeploymentClient     deploymentv1.DeploymentServiceClient
	testDeploymentGRPCServer *testDeploymentServer
)

func TestMain(m *testing.M) {
	os.Exit(runIntegrationTests(m))
}

func runIntegrationTests(m *testing.M) int {
	pool, cleanup, err := testutil.StartPostgres(context.Background())
	if err != nil {
		log.Printf("setup integration database: %v", err)
		return 1
	}
	defer cleanup()

	testDB = pool

	client, deploymentServer, cleanupGRPC, err := startTestDeploymentGRPC(
		context.Background(),
	)
	if err != nil {
		log.Printf("setup deployment gRPC client: %v", err)
		return 1
	}
	defer cleanupGRPC()

	testDeploymentClient = client
	testDeploymentGRPCServer = deploymentServer

	return m.Run()
}

func startTestDeploymentGRPC(
	ctx context.Context,
) (deploymentv1.DeploymentServiceClient, *testDeploymentServer, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("listen for test deployment gRPC server: %w", err)
	}

	server := grpc.NewServer()
	deploymentServer := &testDeploymentServer{}
	deploymentv1.RegisterDeploymentServiceServer(server, deploymentServer)

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Printf("test deployment gRPC server: %v", err)
		}
	}()

	client, conn, err := deploymentgrpc.NewClient(listener.Addr().String())
	if err != nil {
		server.Stop()
		_ = listener.Close()
		return nil, nil, nil, fmt.Errorf("create test deployment gRPC client: %w", err)
	}

	cleanup := func() {
		_ = conn.Close()
		server.GracefulStop()
		_ = listener.Close()
	}

	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := waitForGRPCReady(readyCtx, conn); err != nil {
		cleanup()
		return nil, nil, nil, err
	}

	return client, deploymentServer, cleanup, nil
}

func waitForGRPCReady(ctx context.Context, conn *grpc.ClientConn) error {
	conn.Connect()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return fmt.Errorf("test deployment gRPC connection shut down")
		}

		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("wait for test deployment gRPC connection: %w", ctx.Err())
		}
	}
}
