package grapi_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

func TestCoreRouterAndActionOverride(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.CoreRouter("api::tag.tag").Except("delete")
	app.OverrideCoreAction("api::tag.tag", "find", func(c *grapi.Request, next func() (map[string]any, error)) (map[string]any, error) {
		body, err := next()
		if err != nil {
			return nil, err
		}
		n, err := app.Documents().Count(c.R.Context(), "api::tag.tag", document.Params{})
		if err != nil {
			return nil, err
		}
		meta, _ := body["meta"].(map[string]any)
		meta["extra"] = map[string]any{"total": n}
		return body, nil
	})
	app.OnBootstrap(func(ctx context.Context) error {
		role, err := app.Documents().FindFirst(ctx, "plugin::users-permissions.role", document.Params{"filters": map[string]any{"type": "public"}})
		if err != nil || role == nil {
			return err
		}
		id, _ := role.Get("id")
		for _, action := range []string{"api::tag.tag.find", "api::tag.tag.create", "api::tag.tag.delete"} {
			if _, err := app.Documents().Create(ctx, "plugin::users-permissions.permission", document.Params{"data": map[string]any{"action": action, "role": id}}); err != nil {
				return err
			}
		}
		return nil
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	s, b := c.do("POST", "/api/tags", "", `{"data":{"name":"go"}}`)
	if s != 201 {
		t.Fatalf("create: %d %v", s, b)
	}
	id := b["data"].(map[string]any)["documentId"].(string)
	s, b = c.do("GET", "/api/tags", "", "")
	meta, _ := b["meta"].(map[string]any)
	extra, _ := meta["extra"].(map[string]any)
	if s != 200 || extra["total"] != float64(1) || meta["pagination"] == nil || len(b["data"].([]any)) != 1 {
		t.Fatalf("the override adds meta.extra to the core answer: %d %v", s, b)
	}
	// The path still answers GET and PUT: Koa's allowedMethods gives 405.
	if s, b := c.do("DELETE", "/api/tags/"+id, "", ""); s != 405 {
		t.Fatalf("except delete removes the route: %d %v", s, b)
	}
}

func TestAppLog(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	t.Setenv("GRAPI_LOG_FORMAT", "json")
	var out bytes.Buffer
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &out})
	app.OnBootstrap(func(context.Context) error {
		app.Log().Info("from a hook", "who", "test")
		return nil
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	if !strings.Contains(out.String(), `"msg":"from a hook","who":"test"`) {
		t.Fatalf("app.Log writes with the project's logger: %s", out.String())
	}
}
