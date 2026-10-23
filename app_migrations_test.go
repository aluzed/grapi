package grapi_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/project"
)

// The project's database/migrations run at start, once.
func TestProjectMigrationsRun(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "database", "migrations"), 0o750); err != nil {
		t.Fatal(err)
	}
	sql := "CREATE TABLE grapi_probe (n integer); INSERT INTO grapi_probe VALUES (1);"
	if err := os.WriteFile(filepath.Join(dir, "database", "migrations", "2026.10.23T00.00.00.probe.sql"), []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	for i := 0; i < 2; i++ {
		srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
		if err != nil {
			t.Fatal(err)
		}
		_ = srv.Close()
	}
	p, err := project.Load(dir, "", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.DB.Close() }()
	var n int
	if err := p.DB.QueryRow(context.Background(), "select count(*) from grapi_probe").Scan(&n); err != nil || n != 1 {
		t.Fatalf("the migration ran once: %d %v", n, err)
	}
}
