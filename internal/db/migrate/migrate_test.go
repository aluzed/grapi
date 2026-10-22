package migrate_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
)

func openSQLite(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func executed(t *testing.T, d *db.DB, table string) []string {
	t.Helper()
	names, err := migrate.Executed(context.Background(), d, table)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func TestFreshDatabaseRecordsStrapiInternalMigrations(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	if _, err := migrate.Run(ctx, d, migrate.Options{RunMigrations: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(executed(t, d, migrate.InternalTable), ","); got != strings.Join(migrate.StrapiInternal, ",") {
		t.Fatalf("got %s", got)
	}
	ran, err := migrate.Run(ctx, d, migrate.Options{RunMigrations: true})
	if err != nil || ran {
		t.Fatalf("second run: ran %v, %v", ran, err)
	}
	if n := len(executed(t, d, migrate.InternalTable)); n != len(migrate.StrapiInternal) {
		t.Fatalf("%d internal migrations recorded", n)
	}
}

func TestUserSQLMigrationRunsOnce(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	dir := fstest.MapFS{
		"2026.01.01T00.00.00.create.sql": {Data: []byte("create table notes (id integer primary key, v text); insert into notes (v) values ('a');")},
		"2026.01.02T00.00.00.more.sql":   {Data: []byte("insert into notes (v) values ('b');")},
		".hidden.sql":                    {Data: []byte("this is not run")},
		"readme.md":                      {Data: []byte("ignored")},
	}
	for range 3 {
		if _, err := migrate.Run(ctx, d, migrate.Options{Dir: dir, RunMigrations: true}); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := d.QueryRow(ctx, "select count(*) from notes").Scan(&n); err != nil || n != 2 {
		t.Fatalf("%d rows, %v", n, err)
	}
	if got := strings.Join(executed(t, d, migrate.UserTable), ","); got != "2026.01.01T00.00.00.create.sql,2026.01.02T00.00.00.more.sql" {
		t.Fatalf("recorded %s", got)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	dir := fstest.MapFS{"001_bad.sql": {Data: []byte("create table half (id integer); insert into nowhere values (1);")}}
	_, err := migrate.Run(ctx, d, migrate.Options{Dir: dir, RunMigrations: true})
	if err == nil || !strings.Contains(err.Error(), "001_bad.sql") {
		t.Fatalf("got %v", err)
	}
	var n int
	_ = d.QueryRow(ctx, "select count(*) from sqlite_master where name = 'half'").Scan(&n)
	if n != 0 {
		t.Fatal("the failed migration was not rolled back")
	}
	if len(executed(t, d, migrate.UserTable)) != 0 {
		t.Fatal("the failed migration was recorded")
	}
}

func TestRunMigrationsFalseSkipsUserMigrations(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	dir := fstest.MapFS{"001.sql": {Data: []byte("create table x (id integer);")}}
	if _, err := migrate.Run(ctx, d, migrate.Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if len(executed(t, d, migrate.UserTable)) != 0 {
		t.Fatal("user migration ran with runMigrations false")
	}
}

func TestJavaScriptMigrationStopsStartup(t *testing.T) {
	d := openSQLite(t)
	dir := fstest.MapFS{"2026.01.01.js": {Data: []byte("module.exports = { async up() {} }")}}
	_, err := migrate.Run(context.Background(), d, migrate.Options{Dir: dir, RunMigrations: true})
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("got %v", err)
	}
}

func TestGoMigrations(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	calls := 0
	m := migrate.Migration{Name: "2026.01.01-go", Up: func(ctx context.Context, d *db.DB) error {
		calls++
		_, err := d.Exec(ctx, "create table from_go (id integer)")
		return err
	}}
	for range 2 {
		if _, err := migrate.Run(ctx, d, migrate.Options{Go: []migrate.Migration{m}, RunMigrations: true}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("ran %d times", calls)
	}
}

func TestStrapi4DatabaseIsRefused(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	if _, err := d.Exec(ctx, "create table admin_users (id integer primary key, email varchar(255))"); err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.Run(ctx, d, migrate.Options{}); !errors.Is(err, migrate.ErrStrapi4) {
		t.Fatalf("got %v", err)
	}
}

func TestOlderStrapiDatabaseIsRefused(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	if _, err := d.Exec(ctx, "create table strapi_database_schema (id integer primary key)"); err != nil {
		t.Fatal(err)
	}
	_, err := migrate.Run(ctx, d, migrate.Options{})
	if err == nil || !strings.Contains(err.Error(), "open it once with Strapi 5.59") {
		t.Fatalf("got %v", err)
	}
}

func TestGrapiInternalMigrationsRunAfterStrapis(t *testing.T) {
	ctx := context.Background()
	d := openSQLite(t)
	own := migrate.Migration{Name: "grapi::0.1.0-01-example", Up: func(context.Context, *db.DB) error { return nil }}
	if _, err := migrate.Run(ctx, d, migrate.Options{Internal: []migrate.Migration{own}}); err != nil {
		t.Fatal(err)
	}
	names := executed(t, d, migrate.InternalTable)
	if names[len(names)-1] != own.Name {
		t.Fatalf("got %v", names)
	}
}
