package grapi_test

import (
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/hook"
)

func TestStrapiErrorClasses(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.PublicRoute("GET", "/later", func(*grapi.Request) error { return grapi.NotImplementedError("") })
	app.Policy("global::deny", func(*grapi.PolicyContext, any) (bool, error) { return false, nil })
	app.Policy("global::weekdays", func(*grapi.PolicyContext, any) (bool, error) {
		return false, grapi.PolicyError("Only on weekdays", map[string]any{"day": "sunday"})
	})
	ok := func(c *grapi.Request) error { return c.JSON(200, map[string]any{"ok": true}) }
	app.PublicRoute("GET", "/guarded", ok).Policies("global::deny")
	app.PublicRoute("GET", "/guarded-custom", ok).Policies("global::weekdays")
	app.PublicRoute("POST", "/tags-via-hook", func(c *grapi.Request) error {
		_, err := app.Documents().Create(c.R.Context(), "api::tag.tag", map[string]any{"data": map[string]any{"name": "x"}})
		return err
	})
	app.OnRowCreate("api::tag.tag").Bind(func(*hook.RowEvent) error {
		return fmt.Errorf("tag rule: %w", grapi.ValidationError("Tags are closed"))
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	for _, tc := range []struct {
		method, path  string
		status        int
		name, message string
		details       string
	}{
		{"GET", "/api/later", 501, "NotImplementedError", "This feature is not implemented yet", "map[]"},
		{"GET", "/api/guarded", 403, "PolicyError", "Policy Failed", "map[]"},
		{"GET", "/api/guarded-custom", 403, "PolicyError", "Only on weekdays", "map[day:sunday]"},
		{"POST", "/api/tags-via-hook", 400, "ValidationError", "Tags are closed", "map[]"},
	} {
		s, b := c.do(tc.method, tc.path, "", "{}")
		e, _ := b["error"].(map[string]any)
		if s != tc.status || e["name"] != tc.name || e["message"] != tc.message || fmt.Sprint(e["details"]) != tc.details || e["status"] != float64(tc.status) || b["data"] != nil {
			t.Errorf("%s %s: %d %v", tc.method, tc.path, s, b)
		}
	}
}
