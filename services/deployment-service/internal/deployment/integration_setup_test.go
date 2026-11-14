package deployment_test

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"

	deploymentv1 "example.com/trest/contracts/deployment/v1"
	"example.com/trest/services/deployment-service/internal/deployment"
	"example.com/trest/services/deployment-service/internal/eventstore"
	"example.com/trest/services/deployment-service/internal/idempotency"
	"example.com/trest/services/deployment-service/internal/outbox"
	"example.com/trest/services/deployment-service/internal/testutil"
	deploymentgrpc "example.com/trest/services/deployment-service/internal/transport/grpc"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var testDB *pgxpool.Pool
var testDBCleanup func()
var testDBErr error
var testDBOnce sync.Once

func TestMain(m *testing.M) {
	code := m.Run()
	if testDBCleanup != nil {
		testDBCleanup()
	}
	os.Exit(code)
}

func requireTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	testDBOnce.Do(func() {
		testDB, testDBCleanup, testDBErr = testutil.StartPostgres(
			context.Background(),
		)
	})
	require.NoError(t, testDBErr)

	return testDB
}

func resetTestDB(t *testing.T) {
	t.Helper()
	testutil.ResetDB(t, requireTestDB(t))
}

func startTestGRPCServer(t *testing.T) deploymentv1.DeploymentServiceClient {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	db := requireTestDB(t)
	repo := deployment.NewRepository(db)
	eventRepo := eventstore.NewRepository(db)
	idempotencyRepo := idempotency.NewRepository(db)
	outboxRepo := outbox.NewRepository(db)
	service := deployment.NewService(
		db,
		repo,
		eventRepo,
		idempotencyRepo,
		outboxRepo,
	)
	handler := deploymentgrpc.NewServer(service)

	server := grpc.NewServer()
	deploymentv1.RegisterDeploymentServiceServer(server, handler)

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
