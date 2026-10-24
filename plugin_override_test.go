package grapi_test

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
)

// A plugin's route can be wrapped like a core action, as a Strapi
// extension wraps plugin.controllers.user.me; a route policy can close a
// plugin route.
func TestOverridePluginRoute(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.OverrideCoreAction("plugin::users-permissions.user", "me", func(c *grapi.Request, next func() (map[string]any, error)) (map[string]any, error) {
		body, err := next()
		if err != nil || body == nil || c.User == nil {
			return body, err
		}
		body["owner"] = c.User["email"]
		return body, nil
	})
	app.Policy("global::closed", func(*grapi.PolicyContext, any) (bool, error) { return false, grapi.Error(403, "Forbidden") })
	app.RoutePolicies("plugin::users-permissions.user.find", "closed")
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	s, b := c.do("POST", "/api/auth/local/register", "", `{"username":"ada","email":"ada@example.com","password":"Passw0rd!"}`)
	if s != 200 {
		t.Fatalf("register: %d %v", s, b)
	}
	jwt, _ := b["jwt"].(string)
	s, b = c.do("GET", "/api/users/me", jwt, "")
	if s != 200 || b["username"] != "ada" || b["owner"] != "ada@example.com" {
		t.Fatalf("me: %d %v", s, b)
	}
	s, b = c.do("GET", "/api/users", jwt, "")
	e, _ := b["error"].(map[string]any)
	if s != 403 || e["name"] != "ForbiddenError" || e["message"] != "Forbidden" {
		t.Fatalf("closed directory: %d %v", s, b)
	}
}
