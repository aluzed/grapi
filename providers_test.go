package grapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

type captureMail struct{ sent []grapi.EmailMessage }

func (c *captureMail) Send(_ context.Context, m grapi.EmailMessage) error {
	c.sent = append(c.sent, m)
	return nil
}

func TestCustomEmailProvider(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	conf := "email:\n  config:\n    provider: capture\n    settings:\n      defaultFrom: no-reply@example.com\n"
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
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
	post := func(path, body string) int {
		res, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	if s := post("/api/auth/local/register", `{"username":"ada","email":"ada@example.com","password":"Passw0rd!"}`); s != 200 {
		t.Fatalf("register %d", s)
	}
	if s := post("/api/auth/forgot-password", `{"email":"ada@example.com"}`); s != 200 {
		t.Fatalf("forgot-password %d", s)
	}
	if len(mail.sent) != 1 || mail.sent[0].To != "ada@example.com" || !strings.Contains(mail.sent[0].HTML, "code=") {
		t.Fatalf("sent %+v", mail.sent)
	}
}
