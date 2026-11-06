package integration

import (
	"context"
	"log"
	"os"
	"testing"

	"example.com/trest/services/deployment-service/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testDB *pgxpool.Pool

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

	return m.Run()
}
