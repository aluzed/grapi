package grapi_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func policyApp(t *testing.T) *grapi.App {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	return grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
}

func TestPolicies(t *testing.T) {
	app := policyApp(t)
	var seen any
	app.Policy("global::closed", func(*grapi.PolicyContext, any) (bool, error) { return false, nil })
	app.Policy("global::has-header", func(c *grapi.PolicyContext, config any) (bool, error) {
		seen = config
		if c.R.Header.Get("X-Key") == "" {
			return false, grapi.Error(http.StatusUnauthorized, "missing key")
		}
		return true, nil
	})
	app.PublicRoute("GET", "/ping", func(c *grapi.Request) error { return c.JSON(200, map[string]any{"ok": true}) }).
		Policies(grapi.PolicyRef{Name: "has-header", Config: map[string]any{"header": "X-Key"}})
	app.RoutePolicies("plugin::users-permissions.auth.register", "global::closed")
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	call := func(req *http.Request) (int, string) {
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/local/register", strings.NewReader(`{"username":"a","email":"a@example.com","password":"Passw0rd!"}`))
	req.Header.Set("Content-Type", "application/json")
	if status, body := call(req); status != 403 || !strings.Contains(body, `"name":"PolicyError"`) || !strings.Contains(body, `"message":"Policy Failed"`) {
		t.Errorf("register: %d %s", status, body)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/ping", nil)
	if status, body := call(req); status != 401 || !strings.Contains(body, `"message":"missing key"`) {
		t.Errorf("ping without key: %d %s", status, body)
	}
	req.Header.Set("X-Key", "1")
	if status, body := call(req); status != 200 || seen.(map[string]any)["header"] != "X-Key" {
		t.Errorf("ping with key: %d %s %v", status, body, seen)
	}
}

func TestPolicyResolution(t *testing.T) {
	app := policyApp(t)
	app.Policy("api::account.is-owner", func(*grapi.PolicyContext, any) (bool, error) { return true, nil })
	app.Route("GET", "/account/me", "api::account.account.me", func(*grapi.Request) error { return nil }).Policies("is-owner")
	srv, err := app.Setup()
	if err != nil {
		t.Fatalf("is-owner resolves to api::account.is-owner: %v", err)
	}
	_ = srv.Close()

	app = policyApp(t)
	app.Route("GET", "/account/me", "api::account.account.me", func(*grapi.Request) error { return nil }).Policies("is-owner")
	if _, err := app.Setup(); err == nil || !strings.Contains(err.Error(), `route GET /account/me: policy "is-owner" not found (looked for api::account.is-owner, global::is-owner)`) {
		t.Fatalf("unknown policy: %v", err)
	}
}
