package grapi_test

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestRoutePipelineOrder(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	var steps []string
	policy := func(name string) grapi.Policy {
		return func(*grapi.PolicyContext, any) (bool, error) { steps = append(steps, name); return true, nil }
	}
	mw := func(name string) grapi.RouteMiddleware {
		return func(c *grapi.Request, next func() error) error {
			steps = append(steps, name+">")
			err := next()
			steps = append(steps, "<"+name)
			return err
		}
	}
	app.Policy("global::first", policy("policy1"))
	app.Policy("global::second", policy("policy2"))
	app.PublicRoute("GET", "/pipeline", func(c *grapi.Request) error {
		steps = append(steps, "handler")
		return c.JSON(200, map[string]any{"ok": true})
	}).Policies("global::first", "global::second").Middlewares(mw("m1"), mw("m2"))
	app.Route("GET", "/protected", "api::article.article.popular", func(c *grapi.Request) error {
		return c.JSON(200, map[string]any{"ok": true})
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	if s, b := c.do("GET", "/api/pipeline", "", ""); s != 200 || b["ok"] != true {
		t.Fatalf("auth: false answers without a token: %d %v", s, b)
	}
	if got := strings.Join(steps, " "); got != "policy1 policy2 m1> m2> handler <m2 <m1" {
		t.Fatalf("policies in order, then the middlewares around the handler: %s", got)
	}
	if s, b := c.do("GET", "/api/protected", "", ""); s != 403 || errorMessage(b) != "Forbidden" {
		t.Fatalf("a protected route without the Public permission: %d %v", s, b)
	}
}
