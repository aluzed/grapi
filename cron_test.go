package grapi_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluzed/grapi"
)

func TestCronEvery(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "server.yaml"), []byte("cron:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	var runs atomic.Int32
	app.CronEvery("each-second", grapi.CronRecurrence{Second: grapi.CronRange{Start: 0, End: 59}}, grapi.CronOptions{}, func(context.Context) error {
		runs.Add(1)
		return nil
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	deadline := time.Now().Add(4 * time.Second)
	for runs.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("runs: %d", runs.Load())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
