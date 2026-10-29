package grapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

// The grapi token options: reset and confirmation codes expire, and a
// legacy JWT stops working once its user's password changes.
func TestUsersTokenPolicy(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	conf := `email:
  config:
    provider: capture
users-permissions:
  config:
    resetPassword:
      expiresIn: 1h
    emailConfirmation:
      expiresIn: 3600
    jwt:
      revokeOnPasswordChange: true
`
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	dbFile := filepath.Join(dir, "data.db")
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", dbFile)
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
	d, err := sql.Open("sqlite", "file:"+dbFile+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	do := func(method, path, jwt, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if jwt != "" {
			req.Header.Set("Authorization", "Bearer "+jwt)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		out := map[string]any{}
		_ = json.Unmarshal(b, &out)
		return res.StatusCode, out
	}
	jwtOf := func(path, body string) string {
		t.Helper()
		s, out := do("POST", path, "", body)
		jwt, _ := out["jwt"].(string)
		if s != 200 || jwt == "" {
			t.Fatalf("%s: %d %v", path, s, out)
		}
		return jwt
	}
	me := func(jwt string) int {
		s, _ := do("GET", "/api/users/me", jwt, "")
		return s
	}
	codeRe := regexp.MustCompile(`code=([0-9a-f]+\.[0-9]+)`)
	forgot := func() string {
		t.Helper()
		if s, out := do("POST", "/api/auth/forgot-password", "", `{"email":"ada@example.com"}`); s != 200 {
			t.Fatalf("forgot-password: %d %v", s, out)
		}
		m := codeRe.FindStringSubmatch(mail.sent[len(mail.sent)-1].HTML)
		if m == nil {
			t.Fatalf("reset code with an expiry: %s", mail.sent[len(mail.sent)-1].HTML)
		}
		return m[1]
	}
	reset := func(code, password string) (int, map[string]any) {
		return do("POST", "/api/auth/reset-password", "", `{"code":"`+code+`","password":"`+password+`","passwordConfirmation":"`+password+`"}`)
	}

	first := jwtOf("/api/auth/local/register", `{"username":"ada","email":"ada@example.com","password":"Passw0rd!"}`)
	if s := me(first); s != 200 {
		t.Fatalf("fresh jwt: %d", s)
	}

	// A reset code works once, then the tokens issued before it stop.
	code := forgot()
	if s, out := reset(code, "Second0ne!"); s != 200 || out["jwt"] == nil {
		t.Fatalf("reset: %d %v", s, out)
	}
	if s, _ := reset(code, "Third0ne!"); s != 400 {
		t.Fatalf("reused code: %d", s)
	}
	if s := me(first); s != 401 {
		t.Fatalf("jwt from before the reset: %d", s)
	}
	second := jwtOf("/api/auth/local", `{"identifier":"ada@example.com","password":"Second0ne!"}`)
	if s := me(second); s != 200 {
		t.Fatalf("jwt after the reset: %d", s)
	}

	// An expired code, or one issued before the option (no expiry), is
	// refused and cleared.
	for _, stale := range []string{strings.Repeat("ab", 64) + ".1000000000", strings.Repeat("cd", 64)} {
		if _, err := d.Exec(`update up_users set reset_password_token = ? where email = 'ada@example.com'`, stale); err != nil {
			t.Fatal(err)
		}
		if s, out := reset(stale, "Third0ne!"); s != 400 {
			t.Fatalf("stale code %s: %d %v", stale, s, out)
		}
		var left sql.NullString
		if err := d.QueryRow(`select reset_password_token from up_users where email = 'ada@example.com'`).Scan(&left); err != nil || left.Valid {
			t.Fatalf("stale code kept: %v %v", left, err)
		}
	}

	// change-password answers a new jwt and revokes the previous ones.
	s, out := do("POST", "/api/auth/change-password", second, `{"currentPassword":"Second0ne!","password":"Third0ne!","passwordConfirmation":"Third0ne!"}`)
	third, _ := out["jwt"].(string)
	if s != 200 || third == "" {
		t.Fatalf("change-password: %d %v", s, out)
	}
	if s := me(second); s != 401 {
		t.Fatalf("jwt from before change-password: %d", s)
	}
	if s := me(third); s != 200 {
		t.Fatalf("jwt from change-password: %d", s)
	}

	// An expired confirmation token is refused.
	if _, err := d.Exec(`update up_users set confirmation_token = ?, confirmed = false where email = 'ada@example.com'`, "ef.1000000000"); err != nil {
		t.Fatal(err)
	}
	if s, out := do("GET", "/api/auth/email-confirmation?confirmation=ef.1000000000", "", ""); s != 400 {
		t.Fatalf("expired confirmation: %d %v", s, out)
	}
}

// A wrong value stops the start rather than leaving tokens unlimited.
func TestUsersTokenPolicyInvalid(t *testing.T) {
	for _, conf := range []string{
		"users-permissions:\n  config:\n    resetPassword:\n      expiresIn: soon\n",
		"users-permissions:\n  config:\n    emailConfirmation:\n      expiresIn: 0\n",
		"users-permissions:\n  config:\n    jwt:\n      revokeOnPasswordChange: yes please\n",
	} {
		dir := t.TempDir()
		if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATABASE_CLIENT", "sqlite")
		t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
		t.Setenv("JWT_SECRET", "test")
		srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
		if err == nil {
			_ = srv.Close()
			t.Fatalf("started with %q", conf)
		}
	}
}
