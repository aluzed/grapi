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
	"strconv"
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
	return newAdminFixture(t, "basic")
}

func newAdminFixture(t *testing.T, fixture string) (*httptest.Server, *captureMail, func()) {
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
