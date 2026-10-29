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
