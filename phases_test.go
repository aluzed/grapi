package grapi_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

func TestPhases(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	var calls []string
	app.OnRegister(func(context.Context) error {
		calls = append(calls, "register")
		if app.Documents() != nil {
			t.Error("register runs before the services")
		}
		return nil
	})
	app.OnBootstrap(func(ctx context.Context) error {
		calls = append(calls, "bootstrap")
		_, err := app.Documents().Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "seeded"}})
		return err
	})
	app.OnDestroy(func(context.Context) error {
		calls = append(calls, "destroy")
		return nil
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "register,bootstrap,destroy" {
		t.Fatalf("calls %v", calls)
	}

	app = grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	app.OnBootstrap(func(context.Context) error { return errors.New("seed failed") })
	if _, err := app.Setup(); err == nil || !strings.Contains(err.Error(), "bootstrap: seed failed") {
		t.Fatalf("a bootstrap error stops the start: %v", err)
	}
}
