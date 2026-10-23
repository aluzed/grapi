package grapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func newRefreshModeServer(t *testing.T, sessions string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	conf := "users-permissions:\n  config:\n    jwtManagement: refresh\n" + sessions
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(func() { ts.Close(); _ = srv.Close() })
	return ts
}

type upClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *upClient) do(method, path, token, body string, header ...string) (int, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0 Safari/537.36")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestUsersPermissionsRefreshMode(t *testing.T) {
	ts := newRefreshModeServer(t, "")
	c := &upClient{t: t, base: ts.URL, http: http.DefaultClient}
	s, b := c.do("POST", "/api/auth/local/register", "", `{"username":"ada","email":"ada@example.com","password":"Passw0rd!"}`)
	if s != 200 || b["jwt"] == nil || b["refreshToken"] == nil {
		t.Fatalf("register: %d %v", s, b)
	}
	s, b = c.do("POST", "/api/auth/local", "", `{"identifier":"ada@example.com","password":"Passw0rd!","deviceId":"laptop"}`)
	if s != 200 || b["refreshToken"] == nil {
		t.Fatalf("login: %d %v", s, b)
	}
	jwt, refresh := b["jwt"].(string), b["refreshToken"].(string)
	if s, b := c.do("GET", "/api/users/me", jwt, ""); s != 200 || b["username"] != "ada" {
		t.Fatalf("me with the access token: %d %v", s, b)
	}
	if s, _ := c.do("GET", "/api/users/me", refresh, ""); s != 401 {
		t.Fatalf("a refresh token is not an access token: %d", s)
	}

	s, b = c.do("GET", "/api/auth/sessions", jwt, "")
	list, _ := b["data"].([]any)
	if s != 200 || len(list) != 2 {
		t.Fatalf("sessions: %d %v", s, b)
	}
	first := list[0].(map[string]any)
	if first["current"] != true || first["deviceId"] != "laptop" || first["deviceName"] != "Chrome on macOS" || first["loginAt"] == nil {
		t.Fatalf("the current session comes first: %v", first)
	}

	if s, b := c.do("POST", "/api/auth/refresh", "", `{}`); s != 400 || !strings.Contains(errorMessage(b), "Missing refresh token") {
		t.Fatalf("missing refresh token: %d %v", s, b)
	}
	s, b = c.do("POST", "/api/auth/refresh", "", `{"refreshToken":"`+refresh+`"}`)
	if s != 200 || b["jwt"] == nil || b["refreshToken"] == nil || b["refreshToken"] == refresh {
		t.Fatalf("refresh rotates: %d %v", s, b)
	}
	jwt = b["jwt"].(string)
	if s, _ := c.do("POST", "/api/auth/refresh", "", `{"refreshToken":"nope"}`); s != 401 {
		t.Fatalf("invalid refresh token: %d", s)
	}

	other := list[1].(map[string]any)["id"].(string)
	if s, _ := c.do("DELETE", "/api/auth/sessions/"+other, jwt, ""); s != 200 {
		t.Fatalf("revoke the other session: %d", s)
	}
	if s, b := c.do("DELETE", "/api/auth/sessions/"+other, jwt, ""); s != 404 || !strings.Contains(errorMessage(b), "Session not found") {
		t.Fatalf("revoked twice: %d %v", s, b)
	}
	if s, b := c.do("POST", "/api/auth/logout", jwt, `{}`); s != 200 || b["ok"] != true {
		t.Fatalf("logout: %d %v", s, b)
	}
	if s, _ := c.do("GET", "/api/users/me", jwt, ""); s != 401 {
		t.Fatalf("the access token dies with its session: %d", s)
	}

	_, b = c.do("POST", "/api/auth/local", "", `{"identifier":"ada@example.com","password":"Passw0rd!"}`)
	jwt = b["jwt"].(string)
	c.do("POST", "/api/auth/local", "", `{"identifier":"ada@example.com","password":"Passw0rd!"}`)
	s, b = c.do("POST", "/api/auth/change-password", jwt, `{"currentPassword":"Passw0rd!","password":"N3wPassw0rd!","passwordConfirmation":"N3wPassw0rd!"}`)
	if s != 200 || b["refreshToken"] == nil {
		t.Fatalf("change password: %d %v", s, b)
	}
	if s, _ := c.do("GET", "/api/users/me", jwt, ""); s != 401 {
		t.Fatalf("a password change ends the other sessions: %d", s)
	}
	_, b = c.do("GET", "/api/auth/sessions", b["jwt"].(string), "")
	if n := len(b["data"].([]any)); n != 1 {
		t.Fatalf("one session left: %d", n)
	}
}

func TestUsersPermissionsRefreshCookie(t *testing.T) {
	ts := newRefreshModeServer(t, "    sessions:\n      cookie:\n        name: up_rt\n")
	jar, _ := cookiejar.New(nil)
	c := &upClient{t: t, base: ts.URL, http: &http.Client{Jar: jar}}
	c.do("POST", "/api/auth/local/register", "", `{"username":"bob","email":"bob@example.com","password":"Passw0rd!"}`)
	s, b := c.do("POST", "/api/auth/local", "", `{"identifier":"bob@example.com","password":"Passw0rd!"}`, "x-strapi-refresh-cookie", "httpOnly")
	if s != 200 || b["jwt"] == nil || b["refreshToken"] != nil {
		t.Fatalf("the refresh token goes in the cookie: %d %v", s, b)
	}
	jwt := b["jwt"].(string)
	s, b = c.do("POST", "/api/auth/refresh", "", `{}`, "x-strapi-refresh-cookie", "httpOnly")
	if s != 200 || b["jwt"] == nil || b["refreshToken"] != nil {
		t.Fatalf("refresh from the cookie: %d %v", s, b)
	}
	if s, _ := c.do("POST", "/api/auth/logout", b["jwt"].(string), `{}`, "x-strapi-refresh-cookie", "httpOnly"); s != 200 {
		t.Fatalf("logout: %d", s)
	}
	if s, _ := c.do("POST", "/api/auth/refresh", "", `{}`, "x-strapi-refresh-cookie", "httpOnly"); s != 400 {
		t.Fatalf("the cookie is cleared: %d", s)
	}
	_ = jwt
}

func TestUsersPermissionsLegacyModeHidesSessions(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer func() { ts.Close(); _ = srv.Close() }()
	c := &upClient{t: t, base: ts.URL, http: http.DefaultClient}
	if s, _ := c.do("POST", "/api/auth/refresh", "", `{"refreshToken":"x"}`); s != 404 {
		t.Fatalf("legacy-support answers 404: %d", s)
	}
	if s, b := c.do("POST", "/api/auth/local/register", "", `{"username":"cyd","email":"cy@example.com","password":"Passw0rd!"}`); s != 200 || b["refreshToken"] != nil {
		t.Fatalf("legacy register has no refresh token: %d %v", s, b)
	}
}

func errorMessage(b map[string]any) string {
	e, _ := b["error"].(map[string]any)
	m, _ := e["message"].(string)
	return m
}
