package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
	"github.com/aluzed/grapi/hook"
	"github.com/aluzed/grapi/query"
)

// Mirrors the dev-freelance needs from the EXT-002 sketch; it only checks
// that the public API compiles and registers.
func TestPublicAPI(t *testing.T) {
	app := grapi.New(grapi.Options{Dir: t.TempDir(), Stderr: &bytes.Buffer{}})
	off := app.OnRowCreate("api::post.post", "api::category.category").Bind(func(e *hook.RowEvent) error {
		if e.String("title") == "" {
			return errors.New("title required")
		}
		e.Data["slug"] = e.String("title")
		return nil
	})
	defer off()
	app.OnRowWrite("api::subscriber.subscriber").Bind(func(*hook.RowEvent) error { return nil })
	app.UseDocuments(func(c *document.Context, next document.Next) (any, error) {
		if c.UID == "api::post.post" && c.Action == document.Create {
			c.Params["status"] = "draft"
		}
		return next()
	})
}

func TestHooksRunThroughHTTP(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	app.OnRowCreate("plugin::users-permissions.user").Bind(func(e *hook.RowEvent) error {
		e.Data["username"] = strings.ToUpper(e.String("username"))
		return nil
	})
	var actions []string
	app.UseDocuments(func(c *document.Context, next document.Next) (any, error) {
		actions = append(actions, c.UID+" "+string(c.Action))
		return next()
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	res, err := http.Post(ts.URL+"/api/auth/local/register", "application/json", strings.NewReader(`{"username":"alex","email":"alex@example.com","password":"Passw0rd!"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var body struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || res.StatusCode != 200 {
		t.Fatalf("status %d, %v", res.StatusCode, err)
	}
	if body.User.Username != "ALEX" {
		t.Fatalf("username = %q, the row hook did not run", body.User.Username)
	}
	if len(actions) == 0 || actions[0] != "plugin::users-permissions.user create" {
		t.Fatalf("document middlewares saw %v", actions)
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		b, err := os.ReadFile(p) //nolint:gosec // test fixture
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
}

func TestCustomRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	app.Route("POST", "/account/anonymize", "api::account.account.anonymize", func(c *grapi.Request) error {
		if c.User == nil {
			return grapi.Error(http.StatusUnauthorized, "Authentication required")
		}
		if c.Body()["currentPassword"] == nil {
			return grapi.Error(http.StatusBadRequest, "Current password is required")
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "user": c.User["username"]})
	})
	app.PublicRoute("GET", "/ping/:name", func(c *grapi.Request) error {
		return c.JSON(http.StatusOK, map[string]any{"pong": c.Params["name"]})
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	do := func(method, path, token, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	_, reg := do("POST", "/api/auth/local/register", "", `{"username":"alex","email":"alex@example.com","password":"Passw0rd!"}`)
	jwt, _ := reg["jwt"].(string)
	if code, _ := do("GET", "/api/ping/go", "", ""); code != 200 {
		t.Fatalf("public route: %d", code)
	}
	if code, out := do("POST", "/api/account/anonymize", jwt, `{}`); code != 403 {
		t.Fatalf("without the permission: %d %v", code, out)
	}
	role, err := app.Query("plugin::users-permissions.role").FindOne(context.Background(), query.Params{Where: map[string]any{"type": "authenticated"}})
	if err != nil || role == nil {
		t.Fatal(err)
	}
	roleID, _ := role.Get("id")
	if _, err := app.Query("plugin::users-permissions.permission").Create(context.Background(), query.Data{"action": "api::account.account.anonymize", "role": roleID}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	code, out := do("POST", "/api/account/anonymize", jwt, `{}`)
	errBody, _ := out["error"].(map[string]any)
	if code != 400 || errBody["name"] != "BadRequestError" || errBody["message"] != "Current password is required" {
		t.Fatalf("bad request: %d %v", code, out)
	}
	if code, out := do("POST", "/api/account/anonymize", jwt, `{"currentPassword":"x"}`); code != 200 || out["user"] != "alex" {
		t.Fatalf("authorized: %d %v", code, out)
	}
}
