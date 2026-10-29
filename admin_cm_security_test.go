package grapi_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

// newAuthoredProject is the basic fixture with an article author, a
// relation to users-permissions users.
func newAuthoredProject(t *testing.T) (string, *adminClient, func()) {
	t.Helper()
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(_ *grapi.App, dir string) {
		p := filepath.Join(dir, "src", "api", "article", "content-types", "article", "schema.json")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), `"attributes": {`, `"attributes": {
    "author": { "type": "relation", "relation": "manyToOne", "target": "plugin::users-permissions.user" },`, 1))
		if err := os.WriteFile(p, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	})
	return ts.URL, adminLogin(t, ts.URL), stop
}

// newReader invites an admin whose role reads only the given article fields.
func newReader(t *testing.T, base string, c *adminClient, fields string) *adminClient {
	t.Helper()
	_, b, _ := c.do("POST", "/roles", `{"name":"Readers"}`)
	role := fmt.Sprint(int(dataOf(b)["id"].(float64)))
	perms := `{"permissions":[{"action":"plugin::content-manager.explorer.read","subject":"api::article.article","properties":{"fields":` + fields + `},"conditions":[]}]}`
	if s, b, _ := c.do("PUT", "/roles/"+role+"/permissions", perms); s != 200 {
		t.Fatalf("permissions: %d %v", s, b)
	}
	_, b, _ = c.do("POST", "/users", `{"firstname":"Rea","email":"rea@example.com","roles":[`+role+`]}`)
	r := newAdminClient(t, base)
	_, b, _ = r.do("POST", "/auth/register", `{"registrationToken":"`+dataOf(b)["registrationToken"].(string)+`","userInfo":{"firstname":"Rea","password":"Rea1derPass"}}`)
	r.token = dataOf(b)["token"].(string)
	return r
}

func TestAdminCMHidesRelatedSecrets(t *testing.T) {
	base, c, stop := newAuthoredProject(t)
	defer stop()
	const articles = "/content-manager/collection-types/api::article.article"
	s, b, _ := c.do("POST", "/content-manager/collection-types/plugin::users-permissions.user", `{"data":{"username":"bob","email":"bob@example.com","password":"Secret123","confirmed":true}}`)
	if s != 201 {
		t.Fatalf("user: %d %v", s, b)
	}
	_, b, _ = c.do("POST", articles, fmt.Sprintf(`{"data":{"title":"Hello","views":42,"author":%v}}`, dataOf(b)["id"]))
	id := dataOf(b)["documentId"].(string)
	r := newReader(t, base, c, `["title","author"]`)

	_, b, _ = c.do("GET", articles+"/"+id, "")
	author, _ := dataOf(b)["author"].(map[string]any)
	if author["username"] != "bob" || author["password"] != nil {
		t.Fatalf("super admin sees the author without its password hash: %v", author)
	}
	s, b, _ = r.do("GET", articles+"/"+id, "")
	raw, _ := json.Marshal(b)
	if s != 200 || strings.Contains(string(raw), "bob") || strings.Contains(string(raw), "$2") || dataOf(b)["views"] != nil {
		t.Fatalf("a reader without access to users sees none of the author's fields: %d %s", s, raw)
	}
	if a, _ := dataOf(b)["author"].(map[string]any); a["documentId"] == nil {
		t.Fatalf("the relation still names the author: %s", raw)
	}
	s, b, _ = r.do("GET", articles, "")
	raw, _ = json.Marshal(b)
	if s != 200 || strings.Contains(string(raw), "bob") {
		t.Fatalf("list: %d %s", s, raw)
	}
}

func TestAdminCMFiltersOnlyWhatTheUserReads(t *testing.T) {
	base, c, stop := newAuthoredProject(t)
	defer stop()
	const articles = "/content-manager/collection-types/api::article.article"
	_, b, _ := c.do("POST", "/content-manager/collection-types/plugin::users-permissions.user", `{"data":{"username":"bob","email":"bob@example.com","password":"Secret123","confirmed":true}}`)
	c.do("POST", articles, fmt.Sprintf(`{"data":{"title":"Hello","views":42,"author":%v}}`, dataOf(b)["id"]))
	r := newReader(t, base, c, `["title","author"]`)
	anon := newAdminClient(t, base)
	if s, _, _ := anon.do("POST", "/auth/forgot-password", `{"email":"ada@example.com"}`); s != 200 {
		t.Fatalf("forgot-password: %d", s)
	}
	total := func(f string) any {
		t.Helper()
		s, b, _ := r.do("GET", articles+"?filters="+url.QueryEscape(f), "")
		if s != 200 {
			t.Fatalf("filters %s: %d %v", f, s, b)
		}
		return b["meta"].(map[string]any)["pagination"].(map[string]any)["total"]
	}
	for _, f := range []string{
		`{"views":{"$eq":41}}`,
		`{"createdBy":{"resetPasswordToken":{"$null":true}}}`,
		`{"createdBy":{"password":{"$null":true}}}`,
		`{"$or":[{"createdBy":{"resetPasswordToken":{"$startsWith":"zz"}}}]}`,
		`{"author":{"password":{"$null":true}}}`,
		`{"author":{"username":{"$eq":"nobody"}}}`,
		`{"author":{"resetPasswordToken":{"$notNull":true}}}`,
	} {
		if n := total(f); n != float64(1) {
			t.Errorf("%s narrows the list (total %v): the field must be dropped", f, n)
		}
	}
	if n := total(`{"title":{"$eq":"Nope"}}`); n != float64(0) {
		t.Errorf("a readable field still filters: %v", n)
	}
	if n := total(`{"createdBy":{"firstname":{"$eq":"Nope"}}}`); n != float64(0) {
		t.Errorf("an allowed admin field still filters: %v", n)
	}
	for _, sort := range []string{"views:asc", "createdBy.resetPasswordToken:asc", "author.password:desc"} {
		if s, b, _ := r.do("GET", articles+"?sort="+url.QueryEscape(sort), ""); s != 200 {
			t.Errorf("sort %s: %d %v", sort, s, b)
		}
	}
}

func TestAdminRelationPickerFollowsReadPermissions(t *testing.T) {
	base, c, stop := newAuthoredProject(t)
	defer stop()
	c.do("POST", "/content-manager/collection-types/plugin::users-permissions.user", `{"data":{"username":"bob","email":"bob@example.com","password":"Secret123","confirmed":true}}`)
	r := newReader(t, base, c, `["title","author"]`)
	const picker = "/content-manager/relations/api::article.article/author"
	s, b, _ := r.do("GET", picker, "")
	raw, _ := json.Marshal(b)
	if s != 200 || strings.Contains(string(raw), "bob") || b["meta"].(map[string]any)["mainField"] != "documentId" {
		t.Fatalf("a reader without access to users gets their document ids only: %d %s", s, raw)
	}
	if s, b, _ := r.do("GET", picker+"?_q="+url.QueryEscape("bob@example"), ""); s != 200 || len(b["data"].([]any)) != 0 {
		t.Fatalf("the search must not match fields the reader cannot see: %d %v", s, b)
	}
	if _, b, _ := c.do("GET", picker+"?_q=bo", ""); len(b["data"].([]any)) != 1 || b["data"].([]any)[0].(map[string]any)["label"] != "bob" {
		t.Fatalf("super admin searches the main field: %v", b)
	}
}

func TestAdminRelationPickerInComponentChecksTheField(t *testing.T) {
	ts, _, stop, _ := newAdminApp(t, "components", false)
	defer stop()
	c := adminLogin(t, ts.URL)
	c.do("POST", "/content-manager/collection-types/api::author.author", `{"data":{"name":"Zed"}}`)
	reader := func(fields string) *adminClient {
		_, b, _ := c.do("POST", "/roles", `{"name":"R`+fmt.Sprint(len(fields))+`"}`)
		role := fmt.Sprint(int(dataOf(b)["id"].(float64)))
		c.do("PUT", "/roles/"+role+"/permissions", `{"permissions":[
			{"action":"plugin::content-manager.explorer.read","subject":"api::page.page","properties":{"fields":`+fields+`},"conditions":[]},
			{"action":"plugin::content-manager.explorer.read","subject":"api::author.author","properties":{"fields":["name"]},"conditions":[]}]}`)
		email := fmt.Sprintf("r%d@example.com", len(fields))
		_, b, _ = c.do("POST", "/users", `{"firstname":"R","email":"`+email+`","roles":[`+role+`]}`)
		r := newAdminClient(t, ts.URL)
		_, b, _ = r.do("POST", "/auth/register", `{"registrationToken":"`+dataOf(b)["registrationToken"].(string)+`","userInfo":{"firstname":"R","password":"Rea1derPass"}}`)
		r.token = dataOf(b)["token"].(string)
		return r
	}
	const picker = "/content-manager/relations/api::page.page/author?component=shared.link"
	if s, _, _ := reader(`["title"]`).do("GET", picker, ""); s != 404 {
		t.Fatalf("links unreadable: %d", s)
	}
	if s, b, _ := reader(`["title","links"]`).do("GET", picker, ""); s != 200 || len(b["data"].([]any)) != 1 {
		t.Fatalf("links readable: %d %v", s, b)
	}
}

func TestAdminCTBRefusesPathsOutOfTheProject(t *testing.T) {
	ts, _, stop, _ := newAdminApp(t, "basic", true)
	defer stop()
	c := adminLogin(t, ts.URL)
	victim := t.TempDir()
	if err := os.WriteFile(filepath.Join(victim, "keep.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	up := strings.Repeat("../", 30) + strings.TrimPrefix(victim, "/")
	for _, body := range []string{
		`{"contentTypes":[{"action":"delete","uid":"api::a./` + up + `","data":{"info":{}}}],"components":[]}`,
		`{"contentTypes":[{"action":"delete","uid":"api::../../..` + victim + `.x","data":{"info":{}}}],"components":[]}`,
		`{"contentTypes":[],"components":[{"action":"delete","uid":"x./` + up + `/keep","data":{}}]}`,
	} {
		s, b, _ := c.do("POST", "/content-type-builder/update-schema", body)
		if s != 400 {
			t.Errorf("%s: %d %v", body, s, b)
		}
		if _, err := os.Stat(filepath.Join(victim, "keep.json")); err != nil {
			t.Fatalf("a file out of the project was removed: %v", err)
		}
	}
}

func TestAdminOwnFilesOnlyReadsOwnFileByID(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	s, up := c.upload("/upload", "red.png", pngBytes(t))
	if s != 201 {
		t.Fatalf("upload: %d %v", s, up)
	}
	id := fmt.Sprint(int(up["list"].([]any)[0].(map[string]any)["id"].(float64)))
	_, b, _ := c.do("POST", "/roles", `{"name":"Authors"}`)
	role := fmt.Sprint(int(dataOf(b)["id"].(float64)))
	c.do("PUT", "/roles/"+role+"/permissions", `{"permissions":[{"action":"plugin::upload.read","subject":null,"conditions":["admin::is-creator"]}]}`)
	_, b, _ = c.do("POST", "/users", `{"firstname":"Au","email":"au@example.com","roles":[`+role+`]}`)
	r := newAdminClient(t, ts.URL)
	_, b, _ = r.do("POST", "/auth/register", `{"registrationToken":"`+dataOf(b)["registrationToken"].(string)+`","userInfo":{"firstname":"Au","password":"Auth0rPass"}}`)
	r.token = dataOf(b)["token"].(string)
	if _, b, _ := r.do("GET", "/upload/files", ""); len(b["data"].([]any)) != 0 {
		t.Fatalf("the list hides the others' files: %v", b)
	}
	for _, p := range []string{"/upload/files/" + id, "/upload/files/" + id + "/related"} {
		if s, b, _ := r.do("GET", p, ""); s != 404 {
			t.Errorf("%s of another admin's file: %d %v", p, s, b)
		}
	}
	if s, _, _ := c.do("GET", "/upload/files/"+id, ""); s != 200 {
		t.Fatalf("its creator reads it: %d", s)
	}
}

func TestAdminForgotPasswordIsRateLimited(t *testing.T) {
	ts, mail, stop := newAdminProject(t)
	defer stop()
	adminLogin(t, ts.URL)
	anon := newAdminClient(t, ts.URL)
	for i := range 5 {
		if s, b, _ := anon.do("POST", "/auth/forgot-password", `{"email":"ada@example.com"}`); s != 200 {
			t.Fatalf("request %d: %d %v", i+1, s, b)
		}
	}
	s, b, _ := anon.do("POST", "/auth/forgot-password", `{"email":"Ada@example.com"}`)
	if s != 429 || b["error"].(map[string]any)["name"] != "RateLimitError" {
		t.Fatalf("sixth request in 5 minutes: %d %v", s, b)
	}
	if len(mail.sent) != 5 {
		t.Fatalf("mails sent: %d", len(mail.sent))
	}
	if s, _, _ := anon.do("POST", "/auth/forgot-password", `{"email":"other@example.com"}`); s != 200 {
		t.Fatalf("another email has its own bucket: %d", s)
	}
}
