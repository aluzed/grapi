package grapi_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

// adminDatabase, when set, points the admin tests at another database.
var adminDatabase func(t *testing.T)

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
	return newAdminFixture(t, "basic")
}

func newAdminFixture(t *testing.T, fixture string) (*httptest.Server, *captureMail, func()) {
	t.Helper()
	ts, mail, stop, _ := newAdminApp(t, fixture, false)
	return ts, mail, stop
}

// newAdminApp also returns the project directory; develop sets AutoReload.
func newAdminApp(t *testing.T, fixture string, develop bool) (*httptest.Server, *captureMail, func(), string) {
	t.Helper()
	return newAdminAppWith(t, fixture, develop, nil)
}

// newAdminAppWith lets setup customize the App before it starts.
func newAdminAppWith(t *testing.T, fixture string, develop bool, setup func(app *grapi.App, dir string)) (*httptest.Server, *captureMail, func(), string) {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", fixture, "src"), filepath.Join(dir, "src")); err != nil {
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
	if adminDatabase != nil {
		adminDatabase(t)
	}
	t.Setenv("JWT_SECRET", "test")
	t.Setenv("ADMIN_JWT_SECRET", "admin-test")
	t.Setenv("API_TOKEN_SALT", "salt-test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}, AutoReload: develop})
	mail := &captureMail{}
	app.UseEmailProvider("capture", mail)
	if setup != nil {
		setup(app, dir)
	}
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	return ts, mail, func() { ts.Close(); _ = srv.Close() }, dir
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
	res, err := http.Get(ts.URL + "/admin/content-manager/collection-types/api::article.article")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("SPA route: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func errMessage(b map[string]any) string {
	e, _ := b["error"].(map[string]any)
	m, _ := e["message"].(string)
	return m
}

func TestAdminUsersAndRoles(t *testing.T) {
	ts, mail, stop := newAdminProject(t)
	defer stop()
	super := newAdminClient(t, ts.URL)
	_, b, _ := super.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	super.token = dataOf(b)["token"].(string)
	_, b, _ = super.do("GET", "/roles", "")
	roles := map[string]float64{}
	for _, r := range b["data"].([]any) {
		m := r.(map[string]any)
		roles[m["code"].(string)] = m["id"].(float64)
	}
	if len(roles) != 3 {
		t.Fatalf("seeded roles: %v", roles)
	}
	editor := strconv.Itoa(int(roles["strapi-editor"]))

	s, b, _ := super.do("POST", "/users", `{"firstname":"Ed","email":"ed@example.com","roles":[`+editor+`]}`)
	if s != 201 {
		t.Fatalf("invite: %d %v", s, b)
	}
	tok := dataOf(b)["registrationToken"].(string)
	if len(mail.sent) != 1 || !strings.Contains(mail.sent[0].Text, "registrationToken="+tok) || mail.sent[0].To != "ed@example.com" {
		t.Fatalf("invitation mail: %+v", mail.sent)
	}
	if s, _, _ := super.do("POST", "/users", `{"firstname":"Ed","email":"ed@example.com","roles":[`+editor+`]}`); s != 400 {
		t.Fatalf("duplicate e-mail: %d", s)
	}
	ed := newAdminClient(t, ts.URL)
	if s, b, _ := ed.do("GET", "/auth/registration-info?registrationToken="+tok, ""); s != 200 || dataOf(b)["email"] != "ed@example.com" {
		t.Fatalf("registration-info: %d %v", s, b)
	}
	if s, _, _ := ed.do("POST", "/auth/login", `{"email":"ed@example.com","password":"Passw0rd"}`); s != 400 {
		t.Fatalf("an invited user cannot log in yet: %d", s)
	}
	s, b, _ = ed.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"Edward","password":"Ed1torPass"}}`)
	if s != 200 {
		t.Fatalf("register: %d %v", s, b)
	}
	ed.token = dataOf(b)["token"].(string)
	if s, _, _ := ed.do("GET", "/auth/registration-info?registrationToken="+tok, ""); s != 400 {
		t.Fatalf("the invitation is used once: %d", s)
	}
	if s, _, _ := ed.do("GET", "/users", ""); s != 403 {
		t.Fatalf("an editor cannot list admins: %d", s)
	}
	_, b, _ = ed.do("GET", "/users/me/permissions", "")
	perms := b["data"].([]any)
	if len(perms) == 0 || strings.HasPrefix(perms[0].(map[string]any)["action"].(string), "admin::") {
		t.Fatalf("editor permissions: %v", perms)
	}

	_, b, _ = super.do("GET", "/users?search=edw&pageSize=5", "")
	if list := b["data"].([]any); len(list) != 1 || b["meta"].(map[string]any)["pagination"].(map[string]any)["total"].(float64) != 1 {
		t.Fatalf("search: %v", b)
	}
	_, b, _ = super.do("GET", "/users/me", "")
	ada := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	if s, b, _ := super.do("DELETE", "/users/"+ada, ""); s != 400 || !strings.Contains(errMessage(b), "super admin") {
		t.Fatalf("the last super admin cannot be deleted: %d %v", s, b)
	}
	if s, _, _ := super.do("PUT", "/users/"+ada, `{"roles":[`+editor+`]}`); s != 400 {
		t.Fatalf("the last super admin keeps the role: %d", s)
	}
	_, b, _ = super.do("GET", "/users?search=edw", "")
	edID := strconv.Itoa(int(b["data"].([]any)[0].(map[string]any)["id"].(float64)))
	if s, _, _ := super.do("PUT", "/users/"+edID, `{"isActive":false}`); s != 200 {
		t.Fatalf("deactivate: %d", s)
	}
	if s, _, _ := ed.do("GET", "/users/me", ""); s != 401 {
		t.Fatalf("a deactivated admin is logged out: %d", s)
	}

	s, b, _ = super.do("POST", "/roles", `{"name":"Reviewer","description":"Reads"}`)
	if s != 201 {
		t.Fatalf("create role: %d %v", s, b)
	}
	rid := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	if s, _, _ := super.do("POST", "/roles", `{"name":"reviewer"}`); s != 400 {
		t.Fatalf("duplicate role name: %d", s)
	}
	if s, b, _ := super.do("PUT", "/roles/"+rid+"/permissions", `{"permissions":[{"action":"plugin::content-manager.explorer.read","subject":"api::nope.nope"}]}`); s != 400 {
		t.Fatalf("unknown subject: %d %v", s, b)
	}
	s, b, _ = super.do("PUT", "/roles/"+rid+"/permissions", `{"permissions":[{"action":"plugin::content-manager.explorer.read","subject":"api::article.article","properties":{"fields":["title"]},"conditions":["admin::is-creator"]},{"action":"admin::webhooks.read","subject":null}]}`)
	if s != 200 || len(b["data"].([]any)) != 2 {
		t.Fatalf("set permissions: %d %v", s, b)
	}
	if s, _, _ := super.do("PUT", "/roles/"+strconv.Itoa(int(roles["strapi-super-admin"]))+"/permissions", `{"permissions":[]}`); s != 400 {
		t.Fatalf("super admin permissions are fixed: %d", s)
	}
	if s, _, _ := super.do("DELETE", "/roles/"+editor, ""); s != 400 {
		t.Fatalf("a role with users cannot be deleted: %d", s)
	}
	if s, _, _ := super.do("DELETE", "/roles/"+rid, ""); s != 200 {
		t.Fatalf("delete role: %d", s)
	}
	_, b, _ = super.do("GET", "/permissions", "")
	sections := dataOf(b)["sections"].(map[string]any)
	if len(sections["contentTypes"].([]any)) == 0 || len(sections["settings"].([]any)) == 0 || len(dataOf(b)["conditions"].([]any)) != 2 {
		t.Fatalf("catalog: %v", b)
	}
}

func TestAdminContentManager(t *testing.T) {
	ts, mail, stop := newAdminProject(t)
	defer stop()
	super := newAdminClient(t, ts.URL)
	_, b, _ := super.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	super.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"

	_, b, _ = super.do("GET", "/content-manager/models", "")
	var article map[string]any
	for _, m := range b["data"].([]any) {
		if m.(map[string]any)["uid"] == "api::article.article" {
			article = m.(map[string]any)
		}
	}
	if article == nil || article["mainField"] != "title" || article["permissions"].(map[string]any)["delete"] != true {
		t.Fatalf("models: %v", b)
	}

	ids := map[string]string{}
	for _, title := range []string{"Alpha", "Beta", ""} {
		body := `{"data":{"title":"` + title + `","views":` + strconv.Itoa(len(title)) + `}}`
		if title == "" {
			// required means not null in Strapi: an empty string publishes.
			body = `{"data":{"views":0}}`
		}
		s, b, _ := super.do("POST", base, body)
		if s != 201 {
			t.Fatalf("create %q: %d %v", title, s, b)
		}
		ids[title] = dataOf(b)["documentId"].(string)
	}
	s, b, _ := super.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+ids["Alpha"]+`","`+ids["Beta"]+`","`+ids[""]+`"]}`)
	res := dataOf(b)
	if s != 200 || res["count"].(float64) != 2 || len(res["errors"].([]any)) != 1 || res["errors"].([]any)[0].(map[string]any)["documentId"] != ids[""] {
		t.Fatalf("bulk publish: %d %v", s, b)
	}
	if s, b, _ := super.do("PUT", base+"/"+ids["Beta"], `{"data":{"views":99}}`); s != 200 {
		t.Fatalf("update: %d %v", s, b)
	}
	_, b, _ = super.do("GET", base+"?sort=title:asc", "")
	status := map[string]string{}
	for _, r := range b["data"].([]any) {
		m := r.(map[string]any)
		status[m["documentId"].(string)] = m["status"].(string)
	}
	if status[ids["Alpha"]] != "published" || status[ids["Beta"]] != "modified" || status[ids[""]] != "draft" {
		t.Fatalf("statuses: %v", status)
	}
	if total := b["meta"].(map[string]any)["pagination"].(map[string]any)["total"].(float64); total != 3 {
		t.Fatalf("total: %v", total)
	}
	_, b, _ = super.do("GET", base+"?_q=alph", "")
	if len(b["data"].([]any)) != 1 {
		t.Fatalf("search: %v", b)
	}
	_, b, _ = super.do("GET", base+`?filters={"views":{"$gt":50}}`, "")
	if list := b["data"].([]any); len(list) != 1 || list[0].(map[string]any)["title"] != "Beta" {
		t.Fatalf("filters on the draft: %v", b)
	}
	_, b, _ = super.do("GET", base+"?status=published", "")
	if len(b["data"].([]any)) != 2 {
		t.Fatalf("published list: %v", b)
	}

	_, b, _ = super.do("GET", "/roles", "")
	var author string
	for _, r := range b["data"].([]any) {
		if m := r.(map[string]any); m["code"] == "strapi-author" {
			author = strconv.Itoa(int(m["id"].(float64)))
		}
	}
	_, b, _ = super.do("POST", "/users", `{"firstname":"Au","email":"au@example.com","roles":[`+author+`]}`)
	tok := dataOf(b)["registrationToken"].(string)
	_ = mail
	au := newAdminClient(t, ts.URL)
	_, b, _ = au.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"Au","password":"Auth0rPass"}}`)
	au.token = dataOf(b)["token"].(string)
	s, b, _ = au.do("POST", base, `{"data":{"title":"Mine"}}`)
	if s != 201 {
		t.Fatalf("author create: %d %v", s, b)
	}
	mine := dataOf(b)["documentId"].(string)
	_, b, _ = au.do("GET", base, "")
	if list := b["data"].([]any); len(list) != 1 || list[0].(map[string]any)["documentId"] != mine {
		t.Fatalf("an author sees only their entries: %v", b)
	}
	if s, _, _ := au.do("GET", base+"/"+ids["Alpha"], ""); s != 404 {
		t.Fatalf("an author cannot read another's entry: %d", s)
	}
	if s, _, _ := au.do("PUT", base+"/"+ids["Alpha"], `{"data":{"title":"Hacked"}}`); s != 404 {
		t.Fatalf("an author cannot update another's entry: %d", s)
	}
	_, b, _ = au.do("POST", base+"/actions/bulk-delete", `{"documentIds":["`+ids["Alpha"]+`","`+mine+`"]}`)
	if res := dataOf(b); res["count"].(float64) != 1 || len(res["errors"].([]any)) != 1 {
		t.Fatalf("author bulk delete: %v", b)
	}
	if s, _, _ := au.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+mine+`"]}`); s != 403 {
		t.Fatalf("an author cannot publish: %d", s)
	}
	_, b, _ = super.do("GET", base, "")
	if len(b["data"].([]any)) != 3 {
		t.Fatalf("the super admin's entries are untouched: %v", b)
	}
	if s, _, _ := super.do("DELETE", base+"/"+ids[""], ""); s != 200 {
		t.Fatalf("delete: %d", s)
	}
}

func TestAdminRelations(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "relations")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	create := func(uid, data string) string {
		t.Helper()
		s, b, _ := c.do("POST", "/content-manager/collection-types/"+uid, `{"data":`+data+`}`)
		if s != 201 {
			t.Fatalf("create %s: %d %v", uid, s, b)
		}
		return dataOf(b)["documentId"].(string)
	}
	writer := create("api::writer.writer", `{"name":"Ursula"}`)
	tags := []string{create("api::tag.tag", `{"name":"go"}`), create("api::tag.tag", `{"name":"cms"}`), create("api::tag.tag", `{"name":"admin"}`)}

	s, b, _ := c.do("GET", "/content-manager/relations/api::post.post/tags?_q=cms", "")
	if s != 200 || len(b["data"].([]any)) != 1 || b["data"].([]any)[0].(map[string]any)["label"] != "cms" {
		t.Fatalf("relation targets: %d %v", s, b)
	}
	if s, _, _ := c.do("GET", "/content-manager/relations/api::post.post/title", ""); s != 404 {
		t.Fatalf("not a relation: %d", s)
	}

	post := create("api::post.post", `{"title":"Hello","writer":{"set":[{"documentId":"`+writer+`"}]},"tags":{"set":[{"documentId":"`+tags[2]+`"},{"documentId":"`+tags[0]+`"}]}}`)
	_, b, _ = c.do("GET", "/content-manager/collection-types/api::post.post/"+post, "")
	got := dataOf(b)
	if w, _ := got["writer"].(map[string]any); w == nil || w["name"] != "Ursula" {
		t.Fatalf("writer: %v", got["writer"])
	}
	names := func(v any) []string {
		var out []string
		for _, e := range v.([]any) {
			out = append(out, e.(map[string]any)["name"].(string))
		}
		return out
	}
	if n := names(got["tags"]); strings.Join(n, ",") != "admin,go" {
		t.Fatalf("tags in set order: %v", n)
	}
	if s, b, _ := c.do("PUT", "/content-manager/collection-types/api::post.post/"+post, `{"data":{"writer":{"set":[]},"tags":{"set":[{"documentId":"`+tags[1]+`"},{"documentId":"`+tags[2]+`"}]}}}`); s != 200 {
		t.Fatalf("update relations: %d %v", s, b)
	}
	_, b, _ = c.do("GET", "/content-manager/collection-types/api::post.post/"+post, "")
	got = dataOf(b)
	if got["writer"] != nil || strings.Join(names(got["tags"]), ",") != "cms,admin" {
		t.Fatalf("after update: writer %v tags %v", got["writer"], got["tags"])
	}
}

func TestAdminSingleType(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const st = "/content-manager/single-types/api::homepage.homepage"
	if s, b, _ := c.do("GET", st, ""); s != 200 || b["data"] != nil {
		t.Fatalf("empty single type: %d %v", s, b)
	}
	if s, b, _ := c.do("PUT", st, `{"data":{"title":"Welcome"}}`); s != 200 || dataOf(b)["title"] != "Welcome" {
		t.Fatalf("create: %d %v", s, b)
	}
	if s, b, _ := c.do("PUT", st, `{"data":{"tagline":"Hi"}}`); s != 200 || dataOf(b)["title"] != "Welcome" || dataOf(b)["tagline"] != "Hi" {
		t.Fatalf("update: %d %v", s, b)
	}
	_, b, _ = c.do("GET", "/content-manager/collection-types/api::homepage.homepage", "")
	if s, _, _ := c.do("GET", "/content-manager/collection-types/api::homepage.homepage", ""); s != 404 {
		t.Fatalf("a single type has no list: %d %v", s, b)
	}
	if s, _, _ := c.do("POST", st+"/actions/publish", `{}`); s != 400 {
		t.Fatalf("no draft and publish: %d", s)
	}
	if s, _, _ := c.do("DELETE", st, ""); s != 200 {
		t.Fatalf("delete: %d", s)
	}
	if _, b, _ := c.do("GET", st, ""); b["data"] != nil {
		t.Fatalf("deleted: %v", b)
	}
}

func TestAdminComponents(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "components")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("GET", "/content-manager/components", "")
	if list := b["data"].([]any); len(list) != 8 {
		t.Fatalf("components: %v", b)
	}
	const base = "/content-manager/collection-types/api::page.page"
	s, b, _ := c.do("POST", base, `{"data":{"title":"Home","seo":{"metaTitle":"Home","noIndex":true},
		"layout":{"heading":"Grid","columns":[{"width":6,"cell":{"text":"left"}},{"width":6,"cell":{"text":"right"}}]},
		"body":[{"__component":"blocks.hero","headline":"Hi","cta":{"label":"Go","url":"/go"}},{"__component":"blocks.gallery","caption":"G","slides":3}]}}`)
	if s != 201 {
		t.Fatalf("create: %d %v", s, b)
	}
	id := dataOf(b)["documentId"].(string)
	read := func() map[string]any {
		_, b, _ := c.do("GET", base+"/"+id, "")
		return dataOf(b)
	}
	got := read()
	cols := got["layout"].(map[string]any)["columns"].([]any)
	if len(cols) != 2 || cols[1].(map[string]any)["cell"].(map[string]any)["text"] != "right" {
		t.Fatalf("nested components are populated: %v", got["layout"])
	}
	hero := got["body"].([]any)[0].(map[string]any)
	if hero["__component"] != "blocks.hero" || hero["cta"].(map[string]any)["url"] != "/go" {
		t.Fatalf("dynamic zone: %v", got["body"])
	}
	// The edit view sends the whole document back, ids included.
	got["title"] = "Home page"
	got["seo"].(map[string]any)["metaTitle"] = "Welcome"
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"title": got["title"], "seo": got["seo"], "layout": got["layout"], "body": got["body"]}})
	if s, b, _ := c.do("PUT", base+"/"+id, string(body)); s != 200 {
		t.Fatalf("update: %d %v", s, b)
	}
	_, b, _ = c.do("POST", "/content-manager/collection-types/api::author.author", `{"data":{"name":"Le Guin"}}`)
	author := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("GET", "/content-manager/relations/api::page.page/author?component=shared.link", "")
	if list := b["data"].([]any); len(list) != 1 || list[0].(map[string]any)["label"] != "Le Guin" {
		t.Fatalf("relation targets in a component: %v", b)
	}
	if s, b, _ := c.do("PUT", base+"/"+id, `{"data":{"links":[{"label":"Author","url":"/a","author":{"set":[{"documentId":"`+author+`"}]}}]}}`); s != 200 {
		t.Fatalf("relation in a component: %d %v", s, b)
	}
	got = read()
	if l := got["links"].([]any); len(l) != 1 || l[0].(map[string]any)["author"].(map[string]any)["name"] != "Le Guin" {
		t.Fatalf("component relation: %v", got["links"])
	}
	cols = got["layout"].(map[string]any)["columns"].([]any)
	if got["seo"].(map[string]any)["metaTitle"] != "Welcome" || len(cols) != 2 || cols[0].(map[string]any)["cell"].(map[string]any)["text"] != "left" || len(got["body"].([]any)) != 2 {
		t.Fatalf("after update: %v", got)
	}
}

func TestAdminValidationDetails(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "components")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	s, b, _ := c.do("POST", "/content-manager/collection-types/api::page.page", `{"data":{"title":"T","links":[{"url":"/x"}]}}`)
	errs, _ := b["error"].(map[string]any)["details"].(map[string]any)["errors"].([]any)
	if s != 400 || len(errs) == 0 {
		t.Fatalf("validation details: %d %v", s, b)
	}
	if path := errs[0].(map[string]any)["path"].([]any); len(path) < 2 || path[0] != "links" {
		t.Fatalf("error path: %v", errs)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for x := 0; x < 40; x++ {
		img.Set(x, x%30, color.RGBA{R: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (c *adminClient) upload(path, name string, content []byte) (int, map[string]any) {
	c.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("files", name)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/admin/api/v1"+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		var list []any
		if json.Unmarshal(raw, &list) == nil {
			return res.StatusCode, map[string]any{"list": list}
		}
	}
	return res.StatusCode, out
}

func TestAdminMedia(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)

	s, b, _ := c.do("POST", "/upload/folders", `{"name":"Photos"}`)
	if s != 201 {
		t.Fatalf("create folder: %d %v", s, b)
	}
	folder := dataOf(b)
	fid := strconv.Itoa(int(folder["id"].(float64)))
	if folder["path"] != "/1" {
		t.Fatalf("folder path: %v", folder)
	}
	if s, _, _ := c.do("POST", "/upload/folders", `{"name":"Photos"}`); s != 400 {
		t.Fatalf("duplicate folder: %d", s)
	}
	if s, b, _ := c.do("POST", "/upload/folders", `{"name":"2026","parent":`+fid+`}`); s != 201 || dataOf(b)["path"] != "/1/2" {
		t.Fatalf("subfolder: %d %v", s, b)
	}

	s, up := c.upload("/upload", "red.png", pngBytes(t))
	if s != 201 {
		t.Fatalf("upload: %d %v", s, up)
	}
	file := up["list"].([]any)[0].(map[string]any)
	id := strconv.Itoa(int(file["id"].(float64)))
	if file["mime"] != "image/png" || file["width"].(float64) != 40 {
		t.Fatalf("uploaded file: %v", file)
	}
	_, b, _ = c.do("GET", "/upload/files", "")
	if list := b["data"].([]any); len(list) != 1 {
		t.Fatalf("root files: %v", b)
	}
	if s, b, _ := c.do("PUT", "/upload/files/"+id, `{"alternativeText":"A red line","folder":`+fid+`}`); s != 200 || dataOf(b)["folderPath"] != "/1" {
		t.Fatalf("move file: %d %v", s, b)
	}
	_, b, _ = c.do("GET", "/upload/files", "")
	if len(b["data"].([]any)) != 0 {
		t.Fatalf("the root is empty after the move: %v", b)
	}
	_, b, _ = c.do("GET", "/upload/files?folder="+fid+"&_q=red%20line&type=image", "")
	if len(b["data"].([]any)) != 1 {
		t.Fatalf("search in the folder: %v", b)
	}
	_, b, _ = c.do("GET", "/upload/folders", "")
	if f := b["data"].([]any)[0].(map[string]any); f["filesCount"].(float64) != 1 || f["childrenCount"].(float64) != 1 {
		t.Fatalf("folder counts: %v", b)
	}
	if s, up = c.upload("/upload?folder="+fid, "second.png", pngBytes(t)); s != 201 {
		t.Fatalf("upload into a folder: %d %v", s, up)
	}
	if _, b, _ = c.do("GET", "/upload/files?folder="+fid, ""); len(b["data"].([]any)) != 2 {
		t.Fatalf("the upload lands in the folder: %v", b)
	}
	if s, _, _ := c.do("DELETE", "/upload/files/"+id, ""); s != 200 {
		t.Fatalf("delete: %d", s)
	}
	if s, _, _ := c.do("GET", "/upload/files/"+id, ""); s != 404 {
		t.Fatalf("deleted file: %d", s)
	}
}

func TestAdminUID(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	gen := func(title string) string {
		t.Helper()
		_, b, _ := c.do("POST", "/content-manager/uid/generate", `{"contentTypeUID":"api::article.article","field":"slug","data":{"title":"`+title+`"}}`)
		s, _ := b["data"].(string)
		return s
	}
	if got := gen("Hello Wörld, ça va?"); got != "hello-woerld-ca-va" {
		t.Fatalf("generate: %q", got)
	}
	if _, b, _ := c.do("POST", "/content-manager/collection-types/api::article.article", `{"data":{"title":"A","slug":"hello-woerld-ca-va"}}`); b["error"] != nil {
		t.Fatal(b)
	}
	if got := gen("Hello Wörld, ça va?"); got != "hello-woerld-ca-va-1" {
		t.Fatalf("generate a taken uid: %q", got)
	}
	if got := gen(""); got != "article" {
		t.Fatalf("without the target field: %q", got)
	}
	_, b, _ = c.do("POST", "/content-manager/uid/check-availability", `{"contentTypeUID":"api::article.article","field":"slug","value":"hello-woerld-ca-va"}`)
	if d := dataOf(b); d["isAvailable"] != false || d["suggestion"] != "hello-woerld-ca-va-1" {
		t.Fatalf("check: %v", b)
	}
	if s, _, _ := c.do("POST", "/content-manager/uid/generate", `{"contentTypeUID":"api::article.article","field":"title"}`); s != 400 {
		t.Fatalf("not a uid field: %d", s)
	}
}

func TestAdminHomepage(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	var ids []string
	for _, title := range []string{"One", "Two"} {
		_, b, _ := c.do("POST", "/content-manager/collection-types/api::article.article", `{"data":{"title":"`+title+`"}}`)
		ids = append(ids, dataOf(b)["documentId"].(string))
	}
	c.do("POST", "/content-manager/collection-types/api::article.article/actions/bulk-publish", `{"documentIds":["`+ids[0]+`"]}`)
	_, b, _ = c.do("GET", "/homepage", "")
	d := dataOf(b)
	var articles float64
	for _, x := range d["counts"].([]any) {
		if m := x.(map[string]any); m["uid"] == "api::article.article" {
			articles = m["total"].(float64)
		}
	}
	edited := d["lastEdited"].([]any)
	pub := d["lastPublished"].([]any)
	if articles != 2 || len(edited) < 2 || len(pub) != 1 || pub[0].(map[string]any)["title"] != "One" {
		t.Fatalf("homepage: %v", d)
	}
}

func TestAdminLocales(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "i18n-dp")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	if s, b, _ := c.do("POST", "/i18n/locales", `{"code":"fr"}`); s != 201 || dataOf(b)["name"] != "French (fr)" {
		t.Fatalf("create locale: %d %v", s, b)
	}
	if s, _, _ := c.do("POST", "/i18n/locales", `{"code":"xx-nope"}`); s != 400 {
		t.Fatalf("unknown code: %d", s)
	}
	_, b, _ = c.do("GET", "/content-manager/locales", "")
	locales := b["data"].([]any)
	if len(locales) != 2 {
		t.Fatalf("locales: %v", locales)
	}
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Hello","views":3}}`)
	id := dataOf(b)["documentId"].(string)
	if s, b, _ := c.do("PUT", base+"/"+id, `{"locale":"fr","data":{"title":"Bonjour","views":7}}`); s != 200 || dataOf(b)["locale"] != "fr" {
		t.Fatalf("create the fr version: %d %v", s, b)
	}
	_, b, _ = c.do("GET", base+"/"+id+"?locale=fr", "")
	if d := dataOf(b); d["title"] != "Bonjour" {
		t.Fatalf("fr version: %v", d)
	}
	_, b, _ = c.do("GET", base+"/"+id+"?locale=en", "")
	if d := dataOf(b); d["title"] != "Hello" || d["views"].(float64) != 7 {
		t.Fatalf("en version keeps its title and shares the non localized views: %v", d)
	}
	_, b, _ = c.do("GET", base+"?locale=fr", "")
	if len(b["data"].([]any)) != 1 {
		t.Fatalf("fr list: %v", b)
	}
	var frID string
	for _, l := range locales {
		if m := l.(map[string]any); m["code"] == "fr" {
			frID = strconv.Itoa(int(m["id"].(float64)))
		}
	}
	_, b, _ = c.do("POST", "/releases", `{"name":"French launch"}`)
	frRelease := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	if s, b, _ := c.do("POST", frRelease+"/actions", `{"type":"publish","uid":"api::article.article","documentId":"`+id+`","locale":"fr"}`); s != 201 {
		t.Fatalf("fr entry in a release: %d %v", s, b)
	}
	if s, _, _ := c.do("DELETE", "/i18n/locales/"+frID, ""); s != 200 {
		t.Fatalf("delete locale: %d", s)
	}
	if s, _, _ := c.do("GET", base+"/"+id+"?locale=en", ""); s != 200 {
		t.Fatalf("the en version stays: %d", s)
	}
	_, b, _ = c.do("GET", base+"?locale=fr", "")
	if len(b["data"].([]any)) != 0 {
		t.Fatalf("the fr entries went with the locale: %v", b)
	}
	if _, b, _ = c.do("GET", frRelease, ""); dataOf(b)["count"].(float64) != 0 {
		t.Fatalf("and left the pending releases: %v", dataOf(b))
	}
}

func TestAdminAPITokens(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("GET", "/api-tokens/actions", "")
	actions := b["data"].([]any)
	found := false
	for _, a := range actions {
		found = found || a == "api::article.article.find"
	}
	if !found {
		t.Fatalf("actions: %v", actions)
	}
	if s, _, _ := c.do("POST", "/api-tokens", `{"name":"Bad","type":"custom","permissions":["api::nope.nope.find"]}`); s != 400 {
		t.Fatalf("unknown action: %d", s)
	}
	s, b, _ := c.do("POST", "/api-tokens", `{"name":"Front","description":"Site","type":"custom","lifespan":30,"permissions":["api::article.article.find"]}`)
	if s != 201 {
		t.Fatalf("create: %d %v", s, b)
	}
	tok := dataOf(b)
	key, id := tok["accessKey"].(string), strconv.Itoa(int(tok["id"].(float64)))
	if len(key) != 256 || tok["expiresAt"] == nil {
		t.Fatalf("token: %v", tok)
	}
	api := func(path, key string) int {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	if api("/api/articles", key) != 200 || api("/api/tags", key) != 403 {
		t.Fatalf("custom token scope: %d %d", api("/api/articles", key), api("/api/tags", key))
	}
	_, b, _ = c.do("GET", "/api-tokens", "")
	if l := b["data"].([]any); len(l) != 1 || l[0].(map[string]any)["accessKey"] != nil {
		t.Fatalf("list hides keys: %v", l)
	}
	if s, b, _ := c.do("PUT", "/api-tokens/"+id, `{"permissions":["api::article.article.find","api::tag.tag.find"]}`); s != 200 || len(dataOf(b)["permissions"].([]any)) != 2 {
		t.Fatalf("update: %d %v", s, b)
	}
	if api("/api/tags", key) != 200 {
		t.Fatal("updated permissions apply")
	}
	_, b, _ = c.do("POST", "/api-tokens/"+id+"/regenerate", "")
	next := dataOf(b)["accessKey"].(string)
	if api("/api/articles", key) != 401 || api("/api/articles", next) != 200 {
		t.Fatal("regenerate replaces the key")
	}
	if s, _, _ := c.do("DELETE", "/api-tokens/"+id, ""); s != 200 || api("/api/articles", next) != 401 {
		t.Fatalf("delete: %d", s)
	}
}

func TestAdminWebhooks(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	hits := make(chan string, 10)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.Header.Get("X-Strapi-Event") + " " + r.Header.Get("X-Secret")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer target.Close()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	if s, _, _ := c.do("POST", "/webhooks", `{"name":"x","url":"ftp://nope","events":["entry.nope"]}`); s != 400 {
		t.Fatalf("validation: %d", s)
	}
	s, b, _ := c.do("POST", "/webhooks", `{"name":"Build","url":"`+target.URL+`","headers":{"X-Secret":"s3"},"events":["entry.create"]}`)
	if s != 201 {
		t.Fatalf("create: %d %v", s, b)
	}
	id := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	_, b, _ = c.do("POST", "/webhooks/"+id+"/trigger", "")
	if dataOf(b)["statusCode"].(float64) != 202 || <-hits != "trigger-test s3" {
		t.Fatalf("trigger: %v", b)
	}
	c.do("POST", "/content-manager/collection-types/api::article.article", `{"data":{"title":"Hook"}}`)
	select {
	case got := <-hits:
		if got != "entry.create s3" {
			t.Fatalf("event: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no entry.create delivery")
	}
	if s, b, _ := c.do("PUT", "/webhooks/"+id, `{"name":"Build","url":"`+target.URL+`","events":["entry.create"],"isEnabled":false}`); s != 200 || dataOf(b)["isEnabled"] != false {
		t.Fatalf("disable: %d %v", s, b)
	}
	if s, _, _ := c.do("DELETE", "/webhooks/"+id, ""); s != 200 {
		t.Fatalf("delete: %d", s)
	}
	if _, b, _ := c.do("GET", "/webhooks", ""); len(b["data"].([]any)) != 0 {
		t.Fatalf("list after delete: %v", b)
	}
}

func TestAdminUsersPermissions(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	public := func(method, path, body string) int {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	_, b, _ = c.do("GET", "/users-permissions/roles", "")
	var publicID string
	for _, r := range b["data"].([]any) {
		if m := r.(map[string]any); m["type"] == "public" {
			publicID = strconv.Itoa(int(m["id"].(float64)))
		}
	}
	_, b, _ = c.do("GET", "/users-permissions/roles/"+publicID, "")
	perms := dataOf(b)["permissions"].([]any)
	if public("GET", "/api/articles", "") != 403 {
		t.Fatal("articles are not public yet")
	}
	list := `"api::article.article.find"`
	for _, p := range perms {
		list += `,"` + p.(string) + `"`
	}
	if s, b, _ := c.do("PUT", "/users-permissions/roles/"+publicID, `{"permissions":[`+list+`]}`); s != 200 {
		t.Fatalf("grant: %d %v", s, b)
	}
	if public("GET", "/api/articles", "") != 200 {
		t.Fatal("the public role now reads articles")
	}
	if s, _, _ := c.do("PUT", "/users-permissions/roles/"+publicID, `{"permissions":["api::nope.nope.find"]}`); s != 400 {
		t.Fatalf("unknown action: %d", s)
	}
	if s, _, _ := c.do("DELETE", "/users-permissions/roles/"+publicID, ""); s != 400 {
		t.Fatalf("public cannot be deleted: %d", s)
	}
	reg := `{"username":"bob","email":"bob@example.com","password":"Passw0rd!"}`
	if s, b, _ := c.do("PUT", "/users-permissions/advanced", `{"allow_register":false}`); s != 200 || dataOf(b)["allow_register"] != false {
		t.Fatalf("advanced: %d %v", s, b)
	}
	if public("POST", "/api/auth/local/register", reg) != 400 {
		t.Fatal("registration is closed")
	}
	if s, _, _ := c.do("PUT", "/users-permissions/advanced", `{"nope":1}`); s != 400 {
		t.Fatalf("unknown setting: %d", s)
	}
	if s, b, _ := c.do("PUT", "/users-permissions/providers", `{"github":{"enabled":true,"key":"k"}}`); s != 200 || dataOf(b)["github"].(map[string]any)["key"] != "k" || dataOf(b)["github"].(map[string]any)["callbackUrl"] == nil {
		t.Fatalf("providers merge: %d %v", s, b)
	}
	s, b, _ := c.do("POST", "/users-permissions/roles", `{"name":"Editors of the site","permissions":["api::tag.tag.find"]}`)
	if s != 201 || dataOf(b)["type"] != "editors_of_the_site" {
		t.Fatalf("create role: %d %v", s, b)
	}
	rid := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	if s, _, _ := c.do("DELETE", "/users-permissions/roles/"+rid, ""); s != 200 {
		t.Fatalf("delete role: %d", s)
	}
}

func TestAdminEmailTest(t *testing.T) {
	ts, mail, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	if _, b, _ := c.do("GET", "/email/settings", ""); dataOf(b)["provider"] != "capture" {
		t.Fatalf("settings: %v", b)
	}
	if s, _, _ := c.do("POST", "/email/test", `{"to":"nope"}`); s != 400 {
		t.Fatalf("invalid address: %d", s)
	}
	if s, b, _ := c.do("POST", "/email/test", `{"to":"ops@example.com"}`); s != 200 || len(mail.sent) != 1 || mail.sent[0].To != "ops@example.com" {
		t.Fatalf("test mail: %d %v %v", s, b, mail.sent)
	}
}

func TestAdminContentTypeBuilder(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "basic", true)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("GET", "/content-type-builder/schema", "")
	if cts := dataOf(b)["contentTypes"].([]any); len(cts) < 3 {
		t.Fatalf("schema: %v", b)
	}
	tagFile := filepath.Join(dir, "src/api/tag/content-types/tag/schema.json")
	bad := `{"contentTypes":[{"action":"create","data":{"kind":"collectionType","info":{"displayName":"Book","singularName":"book","pluralName":"books"},"options":{"draftAndPublish":true},
		"attributes":[{"name":"title","type":"string"},{"name":"tags","type":"relation","relation":"manyToMany","target":"api::nope.nope","targetAttribute":"books"}]}}]}`
	if s, _, _ := c.do("POST", "/content-type-builder/update-schema", bad); s != 400 {
		t.Fatalf("invalid batch: %d", s)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/api/book")); err == nil {
		t.Fatal("an invalid batch writes nothing")
	}
	good := `{"components":[{"action":"create","data":{"category":"shared","info":{"displayName":"Note"},"attributes":[{"name":"text","type":"text"}]}}],
		"contentTypes":[{"action":"create","data":{"kind":"collectionType","info":{"displayName":"Book","singularName":"book","pluralName":"books"},"options":{"draftAndPublish":true},
		"attributes":[{"name":"title","type":"string","required":true},{"name":"pages","type":"integer"},{"name":"note","type":"component","component":"shared.note","repeatable":false},
		{"name":"tags","type":"relation","relation":"manyToMany","target":"api::tag.tag","targetAttribute":"books"}]}}]}`
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", good); s != 200 {
		t.Fatalf("update-schema: %d %v", s, b)
	}
	book, err := os.ReadFile(filepath.Join(dir, "src/api/book/content-types/book/schema.json"))
	if err != nil || !strings.Contains(string(book), `"inversedBy": "books"`) || !strings.Contains(string(book), `"collectionName": "books"`) || !regexp.MustCompile(`"title": \{\s*"type": "string"`).Match(book) {
		t.Fatalf("book schema: %s %v", book, err)
	}
	tag, _ := os.ReadFile(tagFile)
	if !strings.Contains(string(tag), `"mappedBy": "tags"`) {
		t.Fatalf("the inverse side: %s", tag)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/components/shared/note.json")); err != nil {
		t.Fatal(err)
	}
	// The develop reload: a new app on the same directory serves the type.
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}, AutoReload: true})
	srv, err := app.Setup()
	if err != nil {
		t.Fatalf("the written schemas load: %v", err)
	}
	ts2 := httptest.NewServer(srv.Handler)
	defer func() { ts2.Close(); _ = srv.Close() }()
	if res, _ := http.Get(ts2.URL + "/api/books"); res.StatusCode != 403 {
		t.Fatalf("the new route exists: %d", res.StatusCode)
	}
	c2 := newAdminClient(t, ts2.URL)
	_, b, _ = c2.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd"}`)
	c2.token = dataOf(b)["token"].(string)
	if s, b, _ := c2.do("POST", "/content-type-builder/update-schema", `{"contentTypes":[{"action":"delete","uid":"api::book.book"}],"components":[{"action":"delete","uid":"shared.note"}]}`); s != 200 {
		t.Fatalf("delete: %d %v", s, b)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/api/book")); err == nil {
		t.Fatal("the api folder is removed")
	}
	if after, _ := os.ReadFile(tagFile); strings.Contains(string(after), "books") || len(after) == 0 {
		t.Fatalf("the inverse relation goes with the type: %s", after)
	}
}

func TestAdminContentTypeBuilderProduction(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	if s, _, _ := c.do("POST", "/content-type-builder/update-schema", `{"contentTypes":[]}`); s != 403 {
		t.Fatalf("writes need grapi develop: %d", s)
	}
}

func TestAdminViewConfiguration(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const path = "/content-manager/configuration/api::article.article"
	_, b, _ = c.do("GET", path, "")
	cfg := dataOf(b)
	layouts := cfg["layouts"].(map[string]any)
	if list := layouts["list"].([]any); len(list) != 4 || list[0] != "id" {
		t.Fatalf("default list: %v", list)
	}
	firstRow := layouts["edit"].([]any)[0].([]any)
	if f := firstRow[0].(map[string]any); f["name"] != "title" || f["size"].(float64) != 6 {
		t.Fatalf("default edit layout: %v", firstRow)
	}
	settings := cfg["settings"].(map[string]any)
	settings["pageSize"], settings["mainField"], settings["defaultSortBy"], settings["defaultSortOrder"] = 20, "title", "views", "DESC"
	layouts["list"] = []any{"id", "title", "views"}
	layouts["edit"] = []any{[]any{map[string]any{"name": "views", "size": 4}, map[string]any{"name": "title", "size": 8}}}
	cfg["metadatas"].(map[string]any)["title"].(map[string]any)["list"].(map[string]any)["label"] = "Titre"
	body, _ := json.Marshal(cfg)
	if s, b, _ := c.do("PUT", path, string(body)); s != 200 {
		t.Fatalf("put: %d %v", s, b)
	}
	_, b, _ = c.do("GET", "/content-manager/models", "")
	var m map[string]any
	for _, x := range b["data"].([]any) {
		if x.(map[string]any)["uid"] == "api::article.article" {
			m = x.(map[string]any)
		}
	}
	edit := m["editLayout"].([]any)
	if m["settings"].(map[string]any)["pageSize"].(float64) != 20 || len(m["listColumns"].([]any)) != 3 || m["labels"].(map[string]any)["title"] != "Titre" ||
		edit[0].([]any)[0].(map[string]any)["name"] != "views" || len(edit) < 2 {
		t.Fatalf("model after configuration: %v", m)
	}
	layouts["edit"] = []any{[]any{map[string]any{"name": "content", "size": 6}}}
	body, _ = json.Marshal(cfg)
	if s, _, _ := c.do("PUT", path, string(body)); s != 400 {
		t.Fatalf("blocks take a full row: %d", s)
	}
}

// Every admin read of every fixture answers: a smoke test over the
// schemas of the compat bench, devfreelance included.
func TestAdminSmokeFixtures(t *testing.T) {
	for _, fx := range []string{"basic", "relations", "components", "i18n-dp", "devfreelance", "naming"} {
		t.Run(fx, func(t *testing.T) {
			ts, _, stop := newAdminFixture(t, fx)
			defer stop()
			c := newAdminClient(t, ts.URL)
			_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
			c.token = dataOf(b)["token"].(string)
			for _, path := range []string{"/homepage", "/content-manager/components", "/content-type-builder/schema", "/permissions", "/users-permissions/roles", "/upload/files"} {
				if s, b, _ := c.do("GET", path, ""); s != 200 {
					t.Errorf("%s: %d %v", path, s, b)
				}
			}
			s, b, _ := c.do("GET", "/content-manager/models", "")
			if s != 200 {
				t.Fatalf("models: %d %v", s, b)
			}
			read := 0
			defer func() {
				if read == 0 {
					t.Error("no entry was read")
				}
				t.Logf("%d entries read", read)
			}()
			for _, x := range b["data"].([]any) {
				m := x.(map[string]any)
				uid := m["uid"].(string)
				if s, b, _ := c.do("GET", "/content-manager/configuration/"+uid, ""); s != 200 {
					t.Errorf("configuration %s: %d %v", uid, s, b)
				}
				if m["kind"] == "singleType" {
					if s, b, _ := c.do("GET", "/content-manager/single-types/"+uid, ""); s != 200 {
						t.Errorf("single type %s: %d %v", uid, s, b)
					}
					continue
				}
				// A draft takes empty data; a type without draft and publish
				// may refuse it (required fields), which is fine here.
				if s, b, _ := c.do("POST", "/content-manager/collection-types/"+uid, `{"data":{}}`); s != 201 && s != 400 {
					t.Errorf("create %s: %d %v", uid, s, b)
				}
				s, b, _ := c.do("GET", "/content-manager/collection-types/"+uid+"?pageSize=5", "")
				if s != 200 {
					t.Errorf("list %s: %d %v", uid, s, b)
					continue
				}
				if list := b["data"].([]any); len(list) > 0 {
					id := list[0].(map[string]any)["documentId"].(string)
					if s, b, _ := c.do("GET", "/content-manager/collection-types/"+uid+"/"+id, ""); s != 200 {
						t.Errorf("entry %s: %d %v", uid, s, b)
					}
					read++
				}
				for name, a := range m["attributes"].(map[string]any) {
					attr := a.(map[string]any)
					if attr["type"] == "relation" && attr["target"] != nil && !strings.HasPrefix(fmt.Sprint(attr["relation"]), "morph") {
						if s, b, _ := c.do("GET", "/content-manager/relations/"+uid+"/"+name, ""); s != 200 {
							t.Errorf("relation %s.%s: %d %v", uid, name, s, b)
						}
					}
				}
			}
		})
	}
}

func TestAdminContentTypeBuilderRename(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "basic", true)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Kept","views":42}}`)
	id := dataOf(b)["documentId"].(string)
	// A stored view configuration, so that the rename moves it.
	_, b, _ = c.do("GET", "/content-manager/configuration/api::article.article", "")
	cfg := dataOf(b)
	cfg["layouts"].(map[string]any)["list"] = []any{"id", "title", "views"}
	body, _ := json.Marshal(cfg)
	c.do("PUT", "/content-manager/configuration/api::article.article", string(body))

	_, b, _ = c.do("GET", "/content-type-builder/schema", "")
	var article map[string]any
	for _, x := range dataOf(b)["contentTypes"].([]any) {
		if x.(map[string]any)["uid"] == "api::article.article" {
			article = x.(map[string]any)
		}
	}
	for _, a := range article["attributes"].([]any) {
		if m := a.(map[string]any); m["name"] == "views" {
			m["name"], m["renamedFrom"] = "readers", "views"
		}
	}
	batch, _ := json.Marshal(map[string]any{"contentTypes": []any{map[string]any{"action": "update", "uid": "api::article.article", "data": article}}})
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", string(batch)); s != 200 {
		t.Fatalf("rename: %d %v", s, b)
	}
	migrations, _ := filepath.Glob(filepath.Join(dir, "database/migrations/*.sql"))
	if len(migrations) != 1 {
		t.Fatalf("one migration: %v", migrations)
	}
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}, AutoReload: true})
	srv, err := app.Setup()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	ts2 := httptest.NewServer(srv.Handler)
	defer func() { ts2.Close(); _ = srv.Close() }()
	c2 := newAdminClient(t, ts2.URL)
	_, b, _ = c2.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd"}`)
	c2.token = dataOf(b)["token"].(string)
	_, b, _ = c2.do("GET", base+"/"+id, "")
	if d := dataOf(b); d["readers"] != float64(42) || d["views"] != nil {
		t.Fatalf("the values follow the rename: %v", d)
	}
	_, b, _ = c2.do("GET", "/content-manager/configuration/api::article.article", "")
	if list := dataOf(b)["layouts"].(map[string]any)["list"].([]any); len(list) != 3 || list[2] != "readers" {
		t.Fatalf("the view configuration follows: %v", list)
	}
	for _, a := range article["attributes"].([]any) {
		if m := a.(map[string]any); m["name"] == "readers" {
			m["name"], m["renamedFrom"] = "title2", "nope"
		}
	}
	batch, _ = json.Marshal(map[string]any{"contentTypes": []any{map[string]any{"action": "update", "uid": "api::article.article", "data": article}}})
	if s, _, _ := c2.do("POST", "/content-type-builder/update-schema", string(batch)); s != 400 {
		t.Fatalf("renaming a missing attribute is refused: %d", s)
	}
}

func TestAdminMediaBulk(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("POST", "/upload/folders", `{"name":"A"}`)
	a := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	_, b, _ = c.do("POST", "/upload/folders", `{"name":"B","parent":`+a+`}`)
	bID := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	var ids []string
	for _, n := range []string{"one.png", "two.png"} {
		_, up := c.upload("/upload", n, pngBytes(t))
		ids = append(ids, strconv.Itoa(int(up["list"].([]any)[0].(map[string]any)["id"].(float64))))
	}
	c.upload("/upload?folder="+bID, "deep.png", pngBytes(t))
	if s, b, _ := c.do("POST", "/upload/actions/bulk-move", `{"fileIds":[`+ids[0]+`,`+ids[1]+`],"destination":`+a+`}`); s != 200 || dataOf(b)["files"].(float64) != 2 {
		t.Fatalf("move: %d %v", s, b)
	}
	if _, b, _ := c.do("GET", "/upload/files?folder="+a, ""); len(b["data"].([]any)) != 2 {
		t.Fatalf("files moved into A: %v", b)
	}
	if s, b, _ := c.do("PUT", "/upload/folders/"+bID, `{"name":"Renamed"}`); s != 200 || dataOf(b)["name"] != "Renamed" {
		t.Fatalf("rename folder: %d %v", s, b)
	}
	s, b, _ := c.do("DELETE", "/upload/folders/"+a, "")
	if s != 200 || dataOf(b)["files"].(float64) != 3 || dataOf(b)["folders"].(float64) != 2 {
		t.Fatalf("delete folder with its content: %d %v", s, b)
	}
	if _, b, _ := c.do("GET", "/upload/files?all=true", ""); len(b["data"].([]any)) != 0 {
		t.Fatalf("no file left: %v", b)
	}
}

func TestAdminHistory(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"First","views":1}}`)
	id := dataOf(b)["documentId"].(string)
	c.do("PUT", base+"/"+id, `{"data":{"title":"Second","views":2}}`)
	c.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+id+`"]}`)
	_, b, _ = c.do("GET", "/content-manager/history/api::article.article/"+id, "")
	versions := b["data"].([]any)
	if len(versions) != 3 {
		t.Fatalf("three versions: %v", versions)
	}
	statuses := []string{}
	for _, v := range versions {
		statuses = append(statuses, v.(map[string]any)["status"].(string))
	}
	if strings.Join(statuses, ",") != "published,draft,draft" || versions[0].(map[string]any)["createdBy"].(map[string]any)["firstname"] != "Ada" {
		t.Fatalf("statuses newest first: %v %v", statuses, versions[0])
	}
	first := strconv.Itoa(int(versions[2].(map[string]any)["id"].(float64)))
	_, b, _ = c.do("GET", "/content-manager/history/versions/"+first, "")
	if d := dataOf(b)["data"].(map[string]any); d["title"] != "First" {
		t.Fatalf("version content: %v", d)
	}
	if s, b, _ := c.do("POST", "/content-manager/history/versions/"+first+"/restore", ""); s != 200 {
		t.Fatalf("restore: %d %v", s, b)
	}
	_, b, _ = c.do("GET", base+"/"+id, "")
	if d := dataOf(b); d["title"] != "First" || d["views"].(float64) != 1 {
		t.Fatalf("restored draft: %v", d)
	}
	_, b, _ = c.do("GET", base+"?status=published", "")
	if p := b["data"].([]any)[0].(map[string]any); p["title"] != "Second" {
		t.Fatalf("the published version is untouched: %v", p)
	}
	_, b, _ = c.do("GET", base, "")
	if st := b["data"].([]any)[0].(map[string]any)["status"]; st != "modified" {
		t.Fatalf("the document is modified: %v", st)
	}
	_, b, _ = c.do("GET", "/content-manager/history/api::article.article/"+id, "")
	if n := len(b["data"].([]any)); n != 4 {
		t.Fatalf("the restore adds a version: %d", n)
	}
	c.do("DELETE", base+"/"+id, "")
	_, b, _ = c.do("GET", "/content-manager/history/api::article.article/"+id, "")
	if n := len(b["data"].([]any)); n != 0 {
		t.Fatalf("versions go with the document: %d", n)
	}
}

func TestAdminAuditLogs(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	anon := newAdminClient(t, ts.URL)
	anon.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Wr0ngPass"}`)
	_, b, _ = c.do("POST", "/content-manager/collection-types/api::article.article", `{"data":{"title":"Audited"}}`)
	id := dataOf(b)["documentId"].(string)
	c.do("PUT", "/content-manager/collection-types/api::article.article/"+id, `{"data":{"title":"Audited 2","views":3}}`)
	c.do("GET", "/content-manager/collection-types/api::article.article", "")
	c.do("POST", "/webhooks", `{"name":"Hook","url":"https://hooks.example.com/secret/path?token=abc","headers":{"Authorization":"Bearer s3cr3t"},"events":["entry.create"]}`)
	c.do("PUT", "/users/me", `{"currentPassword":"Passw0rd","password":"N3wSecretPass"}`)

	_, b, _ = c.do("GET", "/audit-logs?pageSize=50", "")
	var actions []string
	byAction := map[string]map[string]any{}
	for _, e := range b["data"].([]any) {
		m := e.(map[string]any)
		actions = append(actions, m["action"].(string))
		byAction[m["action"].(string)] = m
	}
	for _, want := range []string{"user.create", "admin.auth.error", "entry.create", "entry.update", "webhook.create", "user.password.update"} {
		if byAction[want] == nil {
			t.Errorf("missing %s in %v", want, actions)
		}
	}
	for _, a := range actions {
		if strings.HasPrefix(a, "entry.read") || a == "entry.find" {
			t.Errorf("a read is audited: %s", a)
		}
	}
	if byAction["entry.create"]["user"].(map[string]any)["email"] != "ada@example.com" || byAction["admin.auth.error"]["user"] != nil {
		t.Fatalf("users: %v %v", byAction["entry.create"], byAction["admin.auth.error"])
	}
	hook := strconv.Itoa(int(byAction["webhook.create"]["id"].(float64)))
	_, b, _ = c.do("GET", "/audit-logs/"+hook, "")
	payload, _ := json.Marshal(dataOf(b)["payload"])
	if strings.Contains(string(payload), "s3cr3t") || strings.Contains(string(payload), "secret/path") || !strings.Contains(string(payload), "https://hooks.example.com") {
		t.Fatalf("webhook payload: %s", payload)
	}
	upd := strconv.Itoa(int(byAction["entry.update"]["id"].(float64)))
	_, b, _ = c.do("GET", "/audit-logs/"+upd, "")
	if f := dataOf(b)["payload"].(map[string]any)["fields"].([]any); len(f) != 2 {
		t.Fatalf("changed fields: %v", f)
	}
	// No password, plain or hashed, anywhere in the log.
	req, _ := http.NewRequest("GET", ts.URL+"/admin/api/v1/audit-logs/export.csv?action=entry.create", nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	csvBody, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	lines := strings.Split(strings.TrimSpace(string(csvBody)), "\n")
	if res.Header.Get("Content-Type") != "text/csv; charset=utf-8" || len(lines) != 2 || !strings.HasPrefix(lines[0], "date,action,origin") {
		t.Fatalf("csv export: %s", csvBody)
	}
	all := ""
	_, b, _ = c.do("GET", "/audit-logs?pageSize=100", "")
	for _, e := range b["data"].([]any) {
		id := strconv.Itoa(int(e.(map[string]any)["id"].(float64)))
		_, d, _ := c.do("GET", "/audit-logs/"+id, "")
		raw, _ := json.Marshal(d)
		all += string(raw)
	}
	if strings.Contains(all, "N3wSecretPass") || strings.Contains(all, "Passw0rd") || strings.Contains(all, "$2a$") {
		t.Fatal("a password is in the audit log")
	}
	if !strings.Contains(all, "audit-log.export") {
		t.Fatal("the export is audited")
	}
}

func TestAdminReleases(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	hits := make(chan map[string]any, 10)
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		hits <- body
	}))
	defer target.Close()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	c.do("POST", "/webhooks", `{"name":"Build","url":"`+target.URL+`","events":["releases.publish"]}`)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Ready"}}`)
	ready := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("POST", base, `{"data":{"views":3}}`)
	untitled := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("POST", base, `{"data":{"title":"Live"}}`)
	live := dataOf(b)["documentId"].(string)
	c.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+live+`"]}`)

	if s, _, _ := c.do("POST", "/releases", `{"name":" "}`); s != 400 {
		t.Fatalf("a release needs a name: %d", s)
	}
	s, b, _ := c.do("POST", "/releases", `{"name":"Launch"}`)
	if s != 201 || dataOf(b)["status"] != "empty" {
		t.Fatalf("create: %d %v", s, b)
	}
	rel := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	add := func(typ, doc string) (int, map[string]any) {
		s, b, _ := c.do("POST", rel+"/actions", `{"type":"`+typ+`","uid":"api::article.article","documentId":"`+doc+`"}`)
		return s, b
	}
	for _, a := range [][2]string{{"publish", ready}, {"publish", untitled}, {"unpublish", live}} {
		if s, b := add(a[0], a[1]); s != 201 {
			t.Fatalf("add %v: %d %v", a, s, b)
		}
	}
	if s, b := add("publish", ready); s != 400 || !strings.Contains(errMessage(b), "already") {
		t.Fatalf("a document goes once in a release: %d %v", s, b)
	}
	_, b, _ = c.do("POST", "/content-manager/collection-types/api::tag.tag", `{"data":{"name":"x"}}`)
	if tag, ok := dataOf(b)["documentId"].(string); ok {
		if s, _, _ := c.do("POST", rel+"/actions", `{"type":"publish","uid":"api::tag.tag","documentId":"`+tag+`"}`); s != 400 {
			t.Fatalf("only draft and publish types: %d", s)
		}
	}
	_, b, _ = c.do("GET", rel, "")
	d := dataOf(b)
	states := map[string]string{}
	for _, a := range d["actions"].([]any) {
		a := a.(map[string]any)
		states[a["documentId"].(string)] = a["state"].(string)
	}
	if d["status"] != "blocked" || states[ready] != "ready" || states[untitled] != "not-ready" || states[live] != "ready" {
		t.Fatalf("assessment: %v %v", d["status"], states)
	}
	if s, b, _ := c.do("POST", rel+"/publish", ""); s != 400 {
		t.Fatalf("a blocked release does not publish: %d %v", s, b)
	}
	_, b, _ = c.do("GET", base+"?status=published", "")
	if n := len(b["data"].([]any)); n != 1 {
		t.Fatalf("nothing changed: %d published", n)
	}
	_, b, _ = c.do("GET", "/releases/document/api::article.article/"+ready, "")
	if l := b["data"].([]any); len(l) != 1 || l[0].(map[string]any)["name"] != "Launch" {
		t.Fatalf("releases of a document: %v", l)
	}

	c.do("PUT", base+"/"+untitled, `{"data":{"title":"Titled"}}`)
	s, b, _ = c.do("POST", rel+"/publish", "")
	if s != 200 || dataOf(b)["status"] != "done" || dataOf(b)["releasedAt"] == nil {
		t.Fatalf("publish: %d %v", s, b)
	}
	_, b, _ = c.do("GET", base+"?status=published&sort=title", "")
	titles := []string{}
	for _, e := range b["data"].([]any) {
		titles = append(titles, e.(map[string]any)["title"].(string))
	}
	if strings.Join(titles, ",") != "Ready,Titled" {
		t.Fatalf("published together, Live unpublished: %v", titles)
	}
	select {
	case h := <-hits:
		if r, _ := h["release"].(map[string]any); h["event"] != "releases.publish" || r["name"] != "Launch" {
			t.Fatalf("webhook: %#v", h)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no releases.publish webhook")
	}
	if s, _ := add("publish", live); s != 409 {
		t.Fatalf("a published release is read-only: %d", s)
	}
	if _, b, _ = c.do("GET", "/releases?status=done", ""); len(b["data"].([]any)) != 1 {
		t.Fatalf("done list: %v", b)
	}

	_, b, _ = c.do("POST", base, `{"data":{"title":"Later"}}`)
	later := dataOf(b)["documentId"].(string)
	if s, _, _ := c.do("PUT", "/releases/settings", `{"defaultTimezone":"Mars/Olympus"}`); s != 400 {
		t.Fatalf("settings validation: %d", s)
	}
	c.do("PUT", "/releases/settings", `{"defaultTimezone":"Asia/Tokyo"}`)
	_, b, _ = c.do("POST", "/releases", `{"name":"Tokyo","scheduledAt":"2099-01-01T00:00:00Z"}`)
	if tz := dataOf(b)["timezone"]; tz != "Asia/Tokyo" {
		t.Fatalf("the default timezone applies: %v", tz)
	}
	c.do("DELETE", "/releases/"+strconv.Itoa(int(dataOf(b)["id"].(float64))), "")
	if _, b, _ = c.do("GET", "/releases/settings", ""); dataOf(b)["defaultTimezone"] != "Asia/Tokyo" {
		t.Fatalf("settings: %v", b)
	}
	if s, _, _ := c.do("POST", "/releases", `{"name":"Bad","scheduledAt":"tomorrow"}`); s != 400 {
		t.Fatalf("schedule validation: %d", s)
	}
	_, b, _ = c.do("POST", "/releases", `{"name":"Scheduled","scheduledAt":"2099-01-01T09:00:00+02:00","timezone":"Europe/Paris"}`)
	sched := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	if at := dataOf(b)["scheduledAt"]; at != "2099-01-01T07:00:00Z" {
		t.Fatalf("stored in UTC: %v", at)
	}
	c.do("POST", sched+"/actions", `{"type":"publish","uid":"api::article.article","documentId":"`+later+`"}`)
	c.do("PUT", sched, `{"scheduledAt":"2001-01-01T00:00:00Z","timezone":"Europe/Paris"}`)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, b, _ = c.do("GET", sched, "")
		if dataOf(b)["status"] == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the scheduler publishes a due release: %v", dataOf(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
	_, b, _ = c.do("POST", base, `{"data":{"views":1}}`)
	blocked := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("POST", "/releases", `{"name":"Doomed"}`)
	doomed := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	c.do("POST", doomed+"/actions", `{"type":"publish","uid":"api::article.article","documentId":"`+blocked+`"}`)
	c.do("PUT", doomed, `{"scheduledAt":"2001-01-01T00:00:00Z"}`)
	deadline = time.Now().Add(5 * time.Second)
	for {
		_, b, _ = c.do("GET", doomed, "")
		if dataOf(b)["status"] == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a blocked scheduled release fails: %v", dataOf(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if e, _ := dataOf(b)["error"].(string); !strings.Contains(e, blocked) {
		t.Fatalf("the failure names the entry: %v", dataOf(b))
	}
	c.do("DELETE", base+"/"+blocked, "")
	if _, b, _ = c.do("GET", doomed, ""); dataOf(b)["count"].(float64) != 0 {
		t.Fatalf("a deleted document leaves the pending releases: %v", dataOf(b))
	}
	_, b, _ = c.do("GET", "/audit-logs?action=release.trigger&origin=scheduler", "")
	if l, _ := b["data"].([]any); len(l) != 2 || l[0].(map[string]any)["user"] != nil {
		t.Fatalf("the scheduler is audited, without a user: %v", b)
	}
	_, b, _ = c.do("GET", "/audit-logs/"+strconv.Itoa(int(b["data"].([]any)[0].(map[string]any)["id"].(float64))), "")
	if p, _ := dataOf(b)["payload"].(map[string]any); p["status"] != "failed" {
		t.Fatalf("the last trigger failed: %v", dataOf(b))
	}
	if s, _, _ := c.do("DELETE", sched, ""); s != 200 {
		t.Fatalf("delete: %d", s)
	}
	if s, _, _ := c.do("GET", sched, ""); s != 404 {
		t.Fatalf("deleted: %d", s)
	}
}

func TestAdminReleaseIsAtomic(t *testing.T) {
	var failOn atomic.Value
	failOn.Store("")
	calls := map[string]int{}
	var mu sync.Mutex
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(app *grapi.App, _ string) {
		app.UseDocuments(func(c *document.Context, next document.Next) (any, error) {
			if c.Action == document.Publish {
				id, _ := c.Params["documentId"].(string)
				mu.Lock()
				calls[id]++
				n := calls[id]
				mu.Unlock()
				// The first call is the trial publication of the assessment.
				if id == failOn.Load() && n == 2 {
					return nil, errors.New("disk on fire")
				}
			}
			return next()
		})
	})
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", "/releases", `{"name":"Atomic"}`)
	rel := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	var ids []string
	for _, title := range []string{"One", "Two", "Three"} {
		_, b, _ = c.do("POST", base, `{"data":{"title":"`+title+`"}}`)
		ids = append(ids, dataOf(b)["documentId"].(string))
	}
	failOn.Store(ids[2])
	for _, id := range ids {
		c.do("POST", rel+"/actions", `{"type":"publish","uid":"api::article.article","documentId":"`+id+`"}`)
	}
	mu.Lock()
	clear(calls)
	mu.Unlock()
	if s, b, _ := c.do("POST", rel+"/publish", ""); s != 500 {
		t.Fatalf("the third action fails: %v", b)
	}
	_, b, _ = c.do("GET", base+"?status=published", "")
	if n := len(b["data"].([]any)); n != 0 {
		t.Fatalf("nothing is published: %d", n)
	}
	if _, b, _ = c.do("GET", rel, ""); dataOf(b)["releasedAt"] != nil {
		t.Fatalf("the release is not done: %v", dataOf(b))
	}
}

func TestAdminReleaseLocalePermissions(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "i18n-dp")
	defer stop()
	super := newAdminClient(t, ts.URL)
	_, b, _ := super.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	super.token = dataOf(b)["token"].(string)
	super.do("POST", "/i18n/locales", `{"code":"fr"}`)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = super.do("POST", base, `{"data":{"title":"Hello"}}`)
	id := dataOf(b)["documentId"].(string)
	if s, b, _ := super.do("PUT", base+"/"+id+"?locale=fr", `{"data":{"title":"Bonjour"}}`); s != 200 {
		t.Fatalf("fr version: %d %v", s, b)
	}
	_, b, _ = super.do("POST", "/roles", `{"name":"English editor"}`)
	role := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	perms := []string{}
	for _, a := range []string{"read", "update", "publish"} {
		perms = append(perms, `{"action":"plugin::content-manager.explorer.`+a+`","subject":"api::article.article","properties":{"locales":["en"]},"conditions":[]}`)
	}
	for _, a := range []string{"read", "create", "publish", "actions.create"} {
		perms = append(perms, `{"action":"admin::releases.`+a+`","subject":null,"properties":{},"conditions":[]}`)
	}
	if s, b, _ := super.do("PUT", "/roles/"+role+"/permissions", `{"permissions":[`+strings.Join(perms, ",")+`]}`); s != 200 {
		t.Fatalf("permissions: %d %v", s, b)
	}
	_, b, _ = super.do("POST", "/users", `{"firstname":"En","email":"en@example.com","roles":[`+role+`]}`)
	tok := dataOf(b)["registrationToken"].(string)
	en := newAdminClient(t, ts.URL)
	_, b, _ = en.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"En","password":"Engl1shPass"}}`)
	en.token = dataOf(b)["token"].(string)

	_, b, _ = en.do("POST", "/releases", `{"name":"Mixed"}`)
	rel := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	add := func(c *adminClient, locale string) int {
		s, _, _ := c.do("POST", rel+"/actions", `{"type":"publish","uid":"api::article.article","documentId":"`+id+`","locale":"`+locale+`"}`)
		return s
	}
	if s := add(en, "en"); s != 201 {
		t.Fatalf("en entry: %d", s)
	}
	if s := add(en, "fr"); s != 403 {
		t.Fatalf("fr is not publishable by this role: %d", s)
	}
	if s := add(super, "fr"); s != 201 {
		t.Fatalf("super admin adds fr: %d", s)
	}
	if s, _, _ := en.do("POST", rel+"/publish", ""); s != 403 {
		t.Fatalf("the release holds a locale the role cannot publish: %d", s)
	}
	if s, b, _ := super.do("POST", rel+"/publish", ""); s != 200 {
		t.Fatalf("super admin publishes: %d %v", s, b)
	}
}

func TestAdminReleaseForgetsDeletedDocuments(t *testing.T) {
	var app *grapi.App
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(a *grapi.App, _ string) { app = a })
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("POST", "/content-manager/collection-types/api::article.article", `{"data":{"title":"Gone"}}`)
	id := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("POST", "/releases", `{"name":"R"}`)
	rel := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	c.do("POST", rel+"/actions", `{"type":"publish","uid":"api::article.article","documentId":"`+id+`"}`)
	if _, err := app.Documents().Delete(context.Background(), "api::article.article", id, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, b, _ = c.do("GET", rel, ""); dataOf(b)["count"].(float64) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("a document deleted by the Document Service leaves the release: %v", dataOf(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestAdminReleaseAcrossTypesAndLocales(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "i18n-dp")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	c.do("POST", "/i18n/locales", `{"code":"fr"}`)
	const articles = "/content-manager/collection-types/api::article.article"
	const writers = "/content-manager/collection-types/api::writer.writer"
	_, b, _ = c.do("POST", articles, `{"data":{"title":"Hello"}}`)
	article := dataOf(b)["documentId"].(string)
	c.do("PUT", articles+"/"+article+"?locale=fr", `{"data":{"title":"Bonjour"}}`)
	_, b, _ = c.do("POST", writers, `{"data":{"name":"Ada"}}`)
	writer := dataOf(b)["documentId"].(string)
	c.do("POST", writers+"/actions/bulk-publish", `{"documentIds":["`+writer+`"]}`)

	_, b, _ = c.do("POST", "/releases", `{"name":"Mixed"}`)
	rel := "/releases/" + strconv.Itoa(int(dataOf(b)["id"].(float64)))
	for _, a := range []string{
		`{"type":"publish","uid":"api::article.article","documentId":"` + article + `","locale":"en"}`,
		`{"type":"publish","uid":"api::article.article","documentId":"` + article + `","locale":"fr"}`,
		`{"type":"unpublish","uid":"api::writer.writer","documentId":"` + writer + `"}`,
	} {
		if s, b, _ := c.do("POST", rel+"/actions", a); s != 201 {
			t.Fatalf("add %s: %d %v", a, s, b)
		}
	}
	if s, b, _ := c.do("POST", rel+"/publish", ""); s != 200 {
		t.Fatalf("publish: %d %v", s, b)
	}
	for _, locale := range []string{"en", "fr"} {
		_, b, _ = c.do("GET", articles+"?status=published&locale="+locale, "")
		if n := len(b["data"].([]any)); n != 1 {
			t.Fatalf("%s article published: %d", locale, n)
		}
	}
	_, b, _ = c.do("GET", writers+"?status=published", "")
	if n := len(b["data"].([]any)); n != 0 {
		t.Fatalf("writer unpublished: %d", n)
	}
}

func TestAdminHistoryDiff(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "components")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::page.page"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Home","seo":{"metaTitle":"Home"},"links":[{"label":"A","url":"/a"},{"label":"B","url":"/b"}],
		"body":[{"__component":"blocks.hero","headline":"Hi"},{"__component":"blocks.gallery","caption":"G","slides":3}]}}`)
	id := dataOf(b)["documentId"].(string)
	c.do("PUT", base+"/"+id, `{"data":{"title":"Home","seo":{"metaTitle":"Welcome"},"links":[{"label":"A","url":"/a"},{"label":"B","url":"/bee"}],
		"body":[{"__component":"blocks.hero","headline":"Hi"},{"__component":"blocks.rich-text"}]}}`)
	_, b, _ = c.do("GET", "/content-manager/history/api::page.page/"+id, "")
	versions := b["data"].([]any)
	first := strconv.Itoa(int(versions[1].(map[string]any)["id"].(float64)))
	second := strconv.Itoa(int(versions[0].(map[string]any)["id"].(float64)))
	paths := func(b map[string]any) []string {
		out := []string{}
		for _, ch := range dataOf(b)["changes"].([]any) {
			out = append(out, ch.(map[string]any)["path"].(string))
		}
		sort.Strings(out)
		return out
	}
	_, b, _ = c.do("GET", "/content-manager/history/versions/"+first+"/diff", "")
	if got := strings.Join(paths(b), ","); got != "body.1,links.1.url,seo.metaTitle" {
		t.Fatalf("diff against the draft: %s %v", got, b)
	}
	_, b, _ = c.do("GET", "/content-manager/history/versions/"+first+"/diff?against="+second, "")
	if got := strings.Join(paths(b), ","); got != "body.1,links.1.url,seo.metaTitle" {
		t.Fatalf("diff between versions: %s", got)
	}
	_, b, _ = c.do("GET", "/content-manager/history/versions/"+second+"/diff", "")
	if got := paths(b); len(got) != 0 {
		t.Fatalf("the last version is the draft: %v", got)
	}
}

func TestAdminHistoryDanglingRelations(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "relations")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	create := func(uid, data string) string {
		_, b, _ := c.do("POST", "/content-manager/collection-types/"+uid, `{"data":`+data+`}`)
		return dataOf(b)["documentId"].(string)
	}
	keep, gone := create("api::tag.tag", `{"name":"keep"}`), create("api::tag.tag", `{"name":"gone"}`)
	post := create("api::post.post", `{"title":"Hello","tags":{"set":[{"documentId":"`+keep+`"},{"documentId":"`+gone+`"}]}}`)
	c.do("PUT", "/content-manager/collection-types/api::post.post/"+post, `{"data":{"title":"Changed","tags":{"set":[]}}}`)
	c.do("DELETE", "/content-manager/collection-types/api::tag.tag/"+gone, "")
	_, b, _ = c.do("GET", "/content-manager/history/api::post.post/"+post, "")
	versions := b["data"].([]any)
	first := strconv.Itoa(int(versions[len(versions)-1].(map[string]any)["id"].(float64)))
	_, b, _ = c.do("GET", "/content-manager/history/versions/"+first, "")
	dangling, _ := dataOf(b)["danglingReferences"].([]any)
	if len(dangling) != 1 || dangling[0].(map[string]any)["documentId"] != gone || dangling[0].(map[string]any)["path"] != "tags" {
		t.Fatalf("the deleted tag is reported: %v", dataOf(b))
	}
	if s, b, _ := c.do("POST", "/content-manager/history/versions/"+first+"/restore", ""); s != 200 {
		t.Fatalf("restore: %d %v", s, b)
	}
	_, b, _ = c.do("GET", "/content-manager/collection-types/api::post.post/"+post, "")
	d := dataOf(b)
	tags, _ := d["tags"].([]any)
	if d["title"] != "Hello" || len(tags) != 1 || tags[0].(map[string]any)["documentId"] != keep {
		t.Fatalf("restored without the deleted tag: %v", d)
	}
}

func TestAdminHistoryRules(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"First"}}`)
	id := dataOf(b)["documentId"].(string)
	count := func() int {
		_, b, _ := c.do("GET", "/content-manager/history/api::article.article/"+id, "")
		return len(b["data"].([]any))
	}

	_, b, _ = c.do("POST", "/api-tokens", `{"name":"Writer","type":"full-access"}`)
	key := dataOf(b)["accessKey"].(string)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/articles/"+id, strings.NewReader(`{"data":{"title":"From REST"}}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 || count() != 1 {
		t.Fatalf("a REST update records no version: %d, %d versions", res.StatusCode, count())
	}

	_, b, _ = c.do("POST", "/roles", `{"name":"Reader"}`)
	role := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	c.do("PUT", "/roles/"+role+"/permissions", `{"permissions":[{"action":"plugin::content-manager.explorer.read","subject":"api::article.article","properties":{},"conditions":[]}]}`)
	_, b, _ = c.do("POST", "/users", `{"firstname":"Rea","email":"rea@example.com","roles":[`+role+`]}`)
	tok := dataOf(b)["registrationToken"].(string)
	reader := newAdminClient(t, ts.URL)
	_, b, _ = reader.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"Rea","password":"Read3rPass"}}`)
	reader.token = dataOf(b)["token"].(string)
	_, b, _ = reader.do("GET", "/content-manager/history/api::article.article/"+id, "")
	versions, _ := b["data"].([]any)
	if len(versions) != 1 {
		t.Fatalf("a reader lists the versions: %v", b)
	}
	vid := strconv.Itoa(int(versions[0].(map[string]any)["id"].(float64)))
	if s, _, _ := reader.do("POST", "/content-manager/history/versions/"+vid+"/restore", ""); s != 403 {
		t.Fatalf("restoring needs update: %d", s)
	}
}

func TestAdminHistoryRestoresSharedFields(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "i18n-dp")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	c.do("POST", "/i18n/locales", `{"code":"fr"}`)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Hello","views":1}}`)
	id := dataOf(b)["documentId"].(string)
	c.do("PUT", base+"/"+id+"?locale=fr", `{"data":{"title":"Bonjour"}}`)
	c.do("PUT", base+"/"+id, `{"data":{"title":"Hello","views":5}}`)
	_, b, _ = c.do("GET", "/content-manager/history/api::article.article/"+id+"?locale=fr", "")
	versions := b["data"].([]any)
	if len(versions) != 1 {
		t.Fatalf("one fr version: %v", versions)
	}
	vid := strconv.Itoa(int(versions[0].(map[string]any)["id"].(float64)))
	if s, b, _ := c.do("POST", "/content-manager/history/versions/"+vid+"/restore", ""); s != 200 {
		t.Fatalf("restore fr: %d %v", s, b)
	}
	_, b, _ = c.do("GET", base+"/"+id+"?locale=en", "")
	if d := dataOf(b); d["views"].(float64) != 1 || d["title"] != "Hello" {
		t.Fatalf("the shared views follow the fr version, the en title stays: %v", d)
	}
}

func TestAdminHistoryRetention(t *testing.T) {
	var app *grapi.App
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(a *grapi.App, dir string) {
		app = a
		if err := os.WriteFile(filepath.Join(dir, "config", "admin.yaml"), []byte("history:\n  retentionDays: 1\n  maxVersionsPerDocument: 2\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		a.OnBootstrap(func(ctx context.Context) error {
			_, err := a.Documents().E.DB.Exec(ctx, `insert into grapi_history_versions (uid, document_id, status, data, schema_fields, created_at)
				values ('api::article.article', 'old', 'draft', '{}', '{}', '2000-01-01T00:00:00Z')`)
			return err
		})
	})
	defer stop()
	count := func(doc string) int {
		var n int
		_ = app.Documents().E.DB.QueryRow(context.Background(), `select count(*) from grapi_history_versions where document_id = '`+doc+`'`).Scan(&n)
		return n
	}
	deadline := time.Now().Add(5 * time.Second)
	for count("old") != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the scheduler purges the versions past the retention")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"v1"}}`)
	id := dataOf(b)["documentId"].(string)
	c.do("PUT", base+"/"+id, `{"data":{"title":"v2"}}`)
	c.do("PUT", base+"/"+id, `{"data":{"title":"v3"}}`)
	if n := count(id); n != 2 {
		t.Fatalf("maxVersionsPerDocument keeps 2: %d", n)
	}
}

func TestAdminHistoryAfterFieldRemoval(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "basic", false)
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Hello","summary":"Old summary"}}`)
	id := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("GET", "/content-manager/history/api::article.article/"+id, "")
	vid := strconv.Itoa(int(b["data"].([]any)[0].(map[string]any)["id"].(float64)))
	stop()

	file := filepath.Join(dir, "src/api/article/content-types/article/schema.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	trimmed := regexp.MustCompile(`\s*"summary": \{[^}]*\},`).ReplaceAll(raw, nil)
	if err := os.WriteFile(file, trimmed, 0o600); err != nil {
		t.Fatal(err)
	}
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(srv.Handler)
	defer func() { ts2.Close(); _ = srv.Close() }()
	c2 := newAdminClient(t, ts2.URL)
	_, b, _ = c2.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd"}`)
	c2.token = dataOf(b)["token"].(string)
	_, b, _ = c2.do("GET", "/content-manager/history/versions/"+vid, "")
	if u, _ := dataOf(b)["unknownFields"].(map[string]any); u["summary"] != "Old summary" {
		t.Fatalf("the removed field is unknown: %v", dataOf(b))
	}
	if s, b, _ := c2.do("POST", "/content-manager/history/versions/"+vid+"/restore", ""); s != 200 {
		t.Fatalf("restore ignores it: %d %v", s, b)
	}
}

func TestAdminHistoryOrigins(t *testing.T) {
	var app *grapi.App
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(a *grapi.App, dir string) {
		app = a
		if err := os.WriteFile(filepath.Join(dir, "config", "admin.yaml"), []byte("history:\n  origins: [admin-panel, admin-token]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"First"}}`)
	id := dataOf(b)["documentId"].(string)
	count := func() int {
		_, b, _ := c.do("GET", "/content-manager/history/api::article.article/"+id, "")
		return len(b["data"].([]any))
	}
	if n := count(); n != 1 {
		t.Fatalf("the admin records once: %d", n)
	}
	_, b, _ = c.do("POST", "/api-tokens", `{"name":"Writer","type":"full-access"}`)
	key := dataOf(b)["accessKey"].(string)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/articles/"+id, strings.NewReader(`{"data":{"title":"From REST"}}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if n := count(); res.StatusCode != 200 || n != 2 {
		t.Fatalf("an API token write records a version: %d, %d", res.StatusCode, n)
	}
	if _, err := app.Documents().Update(context.Background(), "api::article.article", id, document.Params{"data": map[string]any{"title": "From code"}}); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 2 {
		t.Fatalf("document-service is not listed: %d", n)
	}
}

func TestAdminAuditDetails(t *testing.T) {
	var app *grapi.App
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(a *grapi.App, _ string) {
		app = a
		a.OnBootstrap(func(ctx context.Context) error {
			_, err := a.Documents().E.DB.Exec(ctx, `insert into grapi_audit_logs (action, date, origin, payload) values ('admin.logout', '2000-01-01T00:00:00Z', 'admin-panel', '{}')`)
			return err
		})
	})
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		_ = app.Documents().E.DB.QueryRow(context.Background(), `select count(*) from grapi_audit_logs where date < '2001'`).Scan(&n)
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the scheduler purges the entries past the retention")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ = c.do("POST", base, `{"data":{"title":"Before","views":1,"internalNote":"secret"}}`)
	id := dataOf(b)["documentId"].(string)
	c.do("PUT", base+"/"+id, `{"data":{"title":"After","views":1,"internalNote":"still secret"}}`)
	c.do("POST", base, `{"data":{"title":"Other"}}`)

	_, b, _ = c.do("GET", "/audit-logs?action=entry.update&uid=api::article.article&documentId="+id, "")
	list := b["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("filtered by document: %v", b)
	}
	_, b, _ = c.do("GET", "/audit-logs/"+strconv.Itoa(int(list[0].(map[string]any)["id"].(float64))), "")
	p := dataOf(b)["payload"].(map[string]any)
	changes, _ := p["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("only the changed field: %v", p)
	}
	ch := changes[0].(map[string]any)
	if ch["path"] != "title" || ch["before"] != "Before" || ch["after"] != "After" {
		t.Fatalf("before and after: %v", ch)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("a private field never reaches the log: %s", raw)
	}

	filters := "?action=entry.create&action=entry.update&user=1&from=2001-01-01T00:00:00Z&to=2999-01-01T00:00:00Z"
	_, b, _ = c.do("GET", "/audit-logs"+filters+"&pageSize=100", "")
	listed := len(b["data"].([]any))
	req, _ := http.NewRequest("GET", ts.URL+"/admin/api/v1/audit-logs/export.csv"+filters, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(res.Body).ReadAll()
	_ = res.Body.Close()
	if err != nil || listed != 3 || len(records)-1 != listed {
		t.Fatalf("the list and the CSV agree: %d listed, %d exported, %v", listed, len(records)-1, err)
	}

	_, b, _ = c.do("POST", "/roles", `{"name":"Auditor"}`)
	role := strconv.Itoa(int(dataOf(b)["id"].(float64)))
	c.do("PUT", "/roles/"+role+"/permissions", `{"permissions":[{"action":"admin::audit-logs.read","subject":null,"properties":{},"conditions":[]}]}`)
	_, b, _ = c.do("POST", "/users", `{"firstname":"Au","email":"auditor@example.com","roles":[`+role+`]}`)
	tok := dataOf(b)["registrationToken"].(string)
	auditor := newAdminClient(t, ts.URL)
	_, b, _ = auditor.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"Au","password":"Aud1torPass"}}`)
	auditor.token = dataOf(b)["token"].(string)
	if s, _, _ := auditor.do("GET", "/audit-logs", ""); s != 200 {
		t.Fatalf("read: %d", s)
	}
	if s, _, _ := auditor.do("GET", "/audit-logs/export.csv", ""); s != 403 {
		t.Fatalf("export needs its permission: %d", s)
	}
}
