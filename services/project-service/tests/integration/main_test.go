package integration

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/trest/services/project-service/internal/testutil"
)

var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup, err := testutil.StartPostgres(context.Background())
	if err != nil {
		log.Fatalf("setup integration tests: %v", err)
	}

	testDB = pool

	code := m.Run()

	cleanup()

	os.Exit(code)
}
