//go:build integration

package grapi_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Under the integration tag, the admin tests run on Postgres when a test
// database is given (scripts/with-homelab-postgres.sh), each on an empty
// public schema.
func init() {
	dsn := os.Getenv("GRAPI_TEST_PG_DSN")
	if dsn == "" {
		return
	}
	adminDatabase = func(t *testing.T) {
		t.Helper()
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.ExecContext(context.Background(), "drop schema public cascade; create schema public"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATABASE_CLIENT", "postgres")
		t.Setenv("DATABASE_URL", dsn)
	}
}
