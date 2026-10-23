package grapi_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/project"
)

func TestEmailConfirmationLink(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte("email:\n  config:\n    provider: capture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	mail := &captureMail{}
	app.UseEmailProvider("capture", mail)
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	p, err := project.Load(dir, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	st := store.Store{DB: p.DB}
	scope := store.Scope{Type: "plugin", Name: "users-permissions"}
	var adv map[string]any
	if _, err := st.Get(context.Background(), scope, "advanced", &adv); err != nil {
		t.Fatal(err)
	}
	adv["email_confirmation"] = true
	adv["email_confirmation_redirection"] = "https://front.example.com/confirmed"
	if err := st.Set(context.Background(), scope, "advanced", adv); err != nil {
		t.Fatal(err)
	}
	_ = p.DB.Close()

	post := func(path, body string) (int, string) {
		res, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if s, b := post("/api/auth/local/register", `{"username":"ada","email":"ada@example.com","password":"Passw0rd!"}`); s != 200 || strings.Contains(b, `"jwt"`) {
		t.Fatalf("register with confirmation sends no jwt: %d %s", s, b)
	}
	if s, b := post("/api/auth/local", `{"identifier":"ada@example.com","password":"Passw0rd!"}`); s != 400 || !strings.Contains(b, "Your account email is not confirmed") {
		t.Fatalf("login before confirmation: %d %s", s, b)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("one confirmation email: %+v", mail.sent)
	}
	code := regexp.MustCompile(`confirmation=([0-9a-f]+)`).FindStringSubmatch(mail.sent[0].HTML)
	if code == nil {
		t.Fatalf("no link in %s", mail.sent[0].HTML)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(ts.URL + "/api/auth/email-confirmation?confirmation=" + code[1])
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://front.example.com/confirmed" {
		t.Fatalf("confirmation link: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if s, b := post("/api/auth/local", `{"identifier":"ada@example.com","password":"Passw0rd!"}`); s != 200 || !strings.Contains(b, `"confirmed":true`) {
		t.Fatalf("login after confirmation: %d %s", s, b)
	}
}
