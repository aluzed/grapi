package grapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/aluzed/grapi"
)

func TestDocumentationPlugin(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "config"), 0o750)
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte("documentation:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	if err := os.WriteFile(filepath.Join(dir, "config", "server.yaml"), []byte("app:\n  keys:\n    - keyA\n    - keyB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string) (int, string, string) {
		res, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Location"), string(b)
	}
	if s, _, body := get("/documentation"); s != 200 || !strings.Contains(body, "SwaggerUIBundle") || !strings.Contains(body, `"/articles"`) {
		t.Fatalf("open documentation: %d %.200s", s, body)
	}
	if s, _, _ := get("/plugins/documentation/swagger-ui-bundle.js"); s != 200 {
		t.Fatalf("Swagger UI files: %d", s)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte("Docs1234"), 10)
	if _, err := app.SQL().Exec(context.Background(), "insert into strapi_core_store_settings (key, value, type) values ('plugin_documentation_config', '{\"restrictedAccess\":true,\"password\":\""+string(hash)+"\"}', 'object')"); err != nil {
		t.Fatal(err)
	}
	if s, loc, _ := get("/documentation/v1.0.0"); s != 302 || loc != "/documentation/login" {
		t.Fatalf("restricted: %d %q", s, loc)
	}
	post := func(password string) (int, string) {
		res, err := client.PostForm(ts.URL+"/documentation/login", url.Values{"password": {password}})
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode, res.Header.Get("Location")
	}
	if s, loc := post("nope"); s != 302 || loc != "/documentation?error=password" {
		t.Fatalf("wrong password: %d %q", s, loc)
	}
	if _, _, body := get("/documentation/login?error=password"); !strings.Contains(body, "Wrong password...") {
		t.Fatalf("login page with the error: %.300s", body)
	}
	if s, loc := post("Docs1234"); s != 302 || loc != "/documentation" {
		t.Fatalf("right password: %d %q", s, loc)
	}
	if s, _, body := get("/documentation"); s != 200 || !strings.Contains(body, "SwaggerUIBundle") {
		t.Fatalf("after login: %d", s)
	}
}
