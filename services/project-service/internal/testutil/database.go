package testutil

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ResetDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	rows, err := pool.Query(ctx, `
        SELECT quote_ident(schemaname) || '.' || quote_ident(tablename)
        FROM pg_tables
        WHERE schemaname = 'public'
          AND tablename <> 'schema_migrations'
        ORDER BY tablename
    `)
	if err != nil {
		t.Fatalf("get database tables: %v", err)
	}

	var tables []string

	for rows.Next() {
		var table string

		if err := rows.Scan(&table); err != nil {
			rows.Close()
			t.Fatalf("scan table: %v", err)
		}

		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate tables: %v", err)
	}

	rows.Close()

	if len(tables) == 0 {
		return
	}

	query := fmt.Sprintf(
		"TRUNCATE TABLE %s RESTART IDENTITY CASCADE",
		strings.Join(tables, ", "),
	)

	if _, err := pool.Exec(ctx, query); err != nil {
		t.Fatalf("reset database: %v", err)
	}
}
