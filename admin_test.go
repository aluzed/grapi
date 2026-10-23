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
	"regexp"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

type adminClient struct {
	t      *testing.T
	base   string
	client *http.Client
	token  string
}

func (c *adminClient) do(method, path, body string) (int, map[string]any, *http.Response) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+"/admin/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out, res
}

func dataOf(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)
	return d
}

func newAdminProject(t *testing.T) (*httptest.Server, *captureMail, func()) {
	t.Helper()
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
	t.Setenv("ADMIN_JWT_SECRET", "admin-test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	mail := &captureMail{}
	app.UseEmailProvider("capture", mail)
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	return ts, mail, func() { ts.Close(); _ = srv.Close() }
}

func newAdminClient(t *testing.T, base string) *adminClient {
	jar, _ := cookiejar.New(nil)
	return &adminClient{t: t, base: base, client: &http.Client{Jar: jar}}
}

func refreshCookie(res *http.Response) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == "grapi_admin_refresh" {
			return c
		}
	}
	return nil
}

func TestAdminFirstAdminAndSessions(t *testing.T) {
	ts, mail, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)

	if s, b, _ := c.do("GET", "/auth/init", ""); s != 200 || dataOf(b)["hasAdmin"] != false {
		t.Fatalf("init: %d %v", s, b)
	}
	if s, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"short"}`); s != 400 || b["error"].(map[string]any)["name"] != "ValidationError" {
		t.Fatalf("weak password: %d %v", s, b)
	}
	s, b, res := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","lastname":"Lovelace","email":"Ada@Example.com","password":"Passw0rd"}`)
	if s != 200 {
		t.Fatalf("register-admin: %d %v", s, b)
	}
	ck := refreshCookie(res)
	if ck == nil || !ck.HttpOnly || ck.MaxAge != 0 || ck.Path != "/admin/api/v1/auth" {
		t.Fatalf("session cookie without remember me: %+v", ck)
	}
	c.token = dataOf(b)["token"].(string)
	if s, b, _ := c.do("GET", "/auth/init", ""); dataOf(b)["hasAdmin"] != true {
		t.Fatalf("init after: %d %v", s, b)
	}
	if s, _, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Eve","email":"eve@example.com","password":"Passw0rd"}`); s != 400 {
		t.Fatalf("second register-admin: %d", s)
	}
	s, b, _ = c.do("GET", "/users/me", "")
	me := dataOf(b)
	if s != 200 || me["email"] != "ada@example.com" || me["password"] != nil {
		t.Fatalf("me: %d %v", s, b)
	}
	if roles := me["roles"].([]any); len(roles) != 1 || roles[0].(map[string]any)["code"] != "strapi-super-admin" {
		t.Fatalf("roles: %v", roles)
	}
	_, b, _ = c.do("GET", "/users/me/permissions", "")
	if perms, _ := b["data"].([]any); len(perms) < 20 {
		t.Fatalf("super admin permissions: %v", b)
	}

	anon := newAdminClient(t, ts.URL)
	if s, _, _ := anon.do("GET", "/users/me", ""); s != 401 {
		t.Fatalf("anonymous me: %d", s)
	}
	if s, b, _ := anon.do("POST", "/auth/login", `{"email":"ada@example.com","password":"wrong"}`); s != 400 || b["error"].(map[string]any)["message"] != "Invalid credentials" {
		t.Fatalf("wrong password: %d %v", s, b)
	}
	s, b, res = anon.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd","rememberMe":true}`)
	if s != 200 {
		t.Fatalf("login: %d %v", s, b)
	}
	first := refreshCookie(res)
	if first == nil || first.MaxAge <= 0 {
		t.Fatalf("remember me gives a persistent cookie: %+v", first)
	}
	s, b, res = anon.do("POST", "/auth/access-token", "")
	if s != 200 || dataOf(b)["token"] == "" {
		t.Fatalf("access-token: %d %v", s, b)
	}
	anon.token = dataOf(b)["token"].(string)
	if refreshCookie(res).Value == first.Value {
		t.Fatal("the refresh token rotates")
	}
	if s, _, _ := anon.do("POST", "/auth/access-token", ""); s != 200 {
		t.Fatalf("second rotation: %d", s)
	}

	thief := newAdminClient(t, ts.URL)
	u, _ := http.NewRequest("GET", ts.URL+"/admin/api/v1/auth", nil)
	thief.client.Jar.SetCookies(u.URL, []*http.Cookie{{Name: first.Name, Value: first.Value, Path: first.Path}})
	if s, _, _ := thief.do("POST", "/auth/access-token", ""); s != 401 {
		t.Fatalf("replayed refresh token: %d", s)
	}
	if s, _, _ := anon.do("POST", "/auth/access-token", ""); s != 401 {
		t.Fatalf("the family is revoked after a replay: %d", s)
	}

	_, b, _ = c.do("GET", "/users/me/sessions", "")
	list := b["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["isCurrent"] != true {
		t.Fatalf("sessions: %v", list)
	}

	if s, _, _ := anon.do("POST", "/auth/forgot-password", `{"email":"ada@example.com"}`); s != 200 {
		t.Fatalf("forgot-password: %d", s)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("reset mail: %+v", mail.sent)
	}
	code := regexp.MustCompile(`code=([0-9a-f]+)`).FindStringSubmatch(mail.sent[0].Text)
	if code == nil {
		t.Fatalf("no link in %q", mail.sent[0].Text)
	}
	if s, _, _ := anon.do("POST", "/auth/forgot-password", `{"email":"nobody@example.com"}`); s != 200 || len(mail.sent) != 1 {
		t.Fatalf("unknown e-mail answers the same: %d", s)
	}
	if s, b, _ := anon.do("POST", "/auth/reset-password", `{"resetPasswordToken":"`+code[1]+`","password":"N3wPassword"}`); s != 200 {
		t.Fatalf("reset-password: %d %v", s, b)
	}
	if s, _, _ := c.do("GET", "/users/me", ""); s != 401 {
		t.Fatalf("a reset ends the other sessions: %d", s)
	}
	if s, _, _ := anon.do("POST", "/auth/reset-password", `{"resetPasswordToken":"`+code[1]+`","password":"N3wPassword"}`); s != 400 {
		t.Fatalf("a reset token works once: %d", s)
	}
	if s, b, _ := anon.do("POST", "/auth/login", `{"email":"ada@example.com","password":"N3wPassword"}`); s != 200 {
		t.Fatalf("login with the new password: %d %v", s, b)
	}
}

func TestAdminProfile(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	other := newAdminClient(t, ts.URL)
	_, b, _ = other.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd"}`)
	other.token = dataOf(b)["token"].(string)

	if s, b, _ := c.do("PUT", "/users/me", `{"preferedLanguage":"fr","lastname":"Lovelace"}`); s != 200 || dataOf(b)["preferedLanguage"] != "fr" {
		t.Fatalf("update me: %d %v", s, b)
	}
	if s, _, _ := c.do("PUT", "/users/me", `{"preferedLanguage":"xx"}`); s != 400 {
		t.Fatalf("unknown language: %d", s)
	}
	if s, b, _ := c.do("PUT", "/users/me", `{"currentPassword":"bad","password":"An0therOne"}`); s != 400 || !strings.Contains(b["error"].(map[string]any)["message"].(string), "Current password") {
		t.Fatalf("wrong current password: %d %v", s, b)
	}
	if s, b, _ := c.do("PUT", "/users/me", `{"currentPassword":"Passw0rd","password":"An0therOne"}`); s != 200 {
		t.Fatalf("change password: %d %v", s, b)
	}
	if s, _, _ := other.do("GET", "/users/me", ""); s != 401 {
		t.Fatalf("a password change ends the other sessions: %d", s)
	}
	if s, _, _ := c.do("GET", "/users/me", ""); s != 200 {
		t.Fatalf("but keeps the current one: %d", s)
	}
	if s, _, _ := c.do("GET", "/nothing", ""); s != 404 {
		t.Fatalf("unknown admin route: %d", s)
	}
	res, err := http.Get(ts.URL + "/admin/content-manager")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("SPA route: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}
