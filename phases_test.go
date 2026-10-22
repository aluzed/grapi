package grapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
	"github.com/aluzed/grapi/hook"
	"github.com/aluzed/grapi/internal/db/store"
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
	after := app.Hooks().Get(hook.AfterSync).(*hook.Parallel[hook.SyncEvent])
	after.Register(func(ctx context.Context, ev hook.SyncEvent) (hook.SyncEvent, error) {
		calls = append(calls, "afterSync")
		if len(ev.ContentTypes) == 0 {
			t.Error("afterSync gets the content types")
		}
		_, err := app.Documents().Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "from afterSync"}})
		return ev, err
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
	if strings.Join(calls, ",") != "register,afterSync,bootstrap,destroy" {
		t.Fatalf("calls %v", calls)
	}

	app = grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	app.OnBootstrap(func(context.Context) error { return errors.New("seed failed") })
	if _, err := app.Setup(); err == nil || !strings.Contains(err.Error(), "bootstrap: seed failed") {
		t.Fatalf("a bootstrap error stops the start: %v", err)
	}
}

func TestSyncHookOldContentTypes(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	p, err := grapi.LoadProject(dir, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// What Strapi records at start.
	if err := (store.Store{DB: p.DB}).Set(context.Background(), store.Scope{Type: "strapi"}, "content_types_schema", map[string]any{"api::old.old": map[string]any{}, "api::tag.tag": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	_ = p.DB.Close()
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	var got hook.SyncEvent
	app.Hooks().Get(hook.BeforeSync).(*hook.Parallel[hook.SyncEvent]).Register(func(_ context.Context, ev hook.SyncEvent) (hook.SyncEvent, error) {
		got = ev
		return ev, nil
	})
	srv, err = app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	if strings.Join(got.OldContentTypes, ",") != "api::old.old,api::tag.tag" || len(got.ContentTypes) == 0 {
		t.Fatalf("event %+v", got)
	}
}
