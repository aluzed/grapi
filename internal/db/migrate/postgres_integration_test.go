//go:build integration

package migrate_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
)

func openPostgres(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("GRAPI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("GRAPI_TEST_PG_DSN not set")
	}
	d, err := db.Open(context.Background(), db.Config{Client: "postgres", ConnectionString: dsn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// Two instances (two pools, so two sessions, like two processes) start at
// once: the advisory lock lets one migrate, the other then finds nothing to do.
func TestPostgresConcurrentStartupMigratesOnce(t *testing.T) {
	ctx := context.Background()
	setup := openPostgres(t)
	if _, err := setup.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatal(err)
	}
	dir := fstest.MapFS{
		"001_slow.sql": {Data: []byte("create table counter (n integer); select pg_sleep(1); insert into counter values (1);")},
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		d := openPostgres(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = migrate.WithLock(ctx, d, func(ctx context.Context) error {
				_, err := migrate.Run(ctx, d, migrate.Options{Dir: dir, RunMigrations: true})
				return err
			})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := setup.QueryRow(ctx, "select count(*) from counter").Scan(&n); err != nil || n != 1 {
		t.Fatalf("%d rows, %v", n, err)
	}
	if got := executed(t, setup, migrate.UserTable); len(got) != 1 {
		t.Fatalf("recorded %v", got)
	}
	if got := executed(t, setup, migrate.InternalTable); len(got) != len(migrate.StrapiInternal) {
		t.Fatalf("internal %v", got)
	}
}
