package grapi_test

import (
	"bytes"
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
