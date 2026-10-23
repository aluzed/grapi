package grapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

func adminLogin(t *testing.T, base string) *adminClient {
	t.Helper()
	c := newAdminClient(t, base)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	return c
}

func TestAdminBulkPublishFifty(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	const base = "/content-manager/collection-types/api::article.article"
	var ids, invalid []string
	for i := range 50 {
		body := fmt.Sprintf(`{"data":{"title":"Post %d"}}`, i)
		if i%17 == 5 {
			body = `{"data":{"views":1}}` // no title: the draft saves, publishing fails
		}
		s, b, _ := c.do("POST", base, body)
		if s != 201 {
			t.Fatalf("create %d: %d %v", i, s, b)
		}
		id := dataOf(b)["documentId"].(string)
		ids = append(ids, id)
		if i%17 == 5 {
			invalid = append(invalid, id)
		}
	}
	s, b, _ := c.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+strings.Join(ids, `","`)+`"]}`)
	res := dataOf(b)
	errs, _ := res["errors"].([]any)
	if s != 200 || res["count"] != float64(47) || len(errs) != 3 {
		t.Fatalf("bulk publish of 50 with 3 invalid: %d %v", s, b)
	}
	for i, e := range errs {
		m := e.(map[string]any)
		if m["documentId"] != invalid[i] || !strings.Contains(fmt.Sprint(m["message"]), "title") {
			t.Fatalf("error %d names the document and the reason: %v", i, m)
		}
	}
	_, b, _ = c.do("GET", base+"?status=published&pageSize=100", "")
	if n := len(b["data"].([]any)); n != 47 {
		t.Fatalf("published: %d", n)
	}
}

// The Content Manager list answers what the Document Service does for the
// same filters, sort, search, locale and status.
func TestAdminListMatchesDocumentService(t *testing.T) {
	var app *grapi.App
	ts, _, stop, _ := newAdminAppWith(t, "i18n-dp", false, func(a *grapi.App, _ string) { app = a })
	defer stop()
	c := adminLogin(t, ts.URL)
	c.do("POST", "/i18n/locales", `{"code":"fr"}`)
	const base = "/content-manager/collection-types/api::article.article"
	for i, title := range []string{"Go basics", "Go generics", "Rust", "Go tooling", "Zig"} {
		s, b, _ := c.do("POST", base, fmt.Sprintf(`{"data":{"title":"%s en","views":%d}}`, title, i*10))
		if s != 201 {
			t.Fatalf("create: %d %v", s, b)
		}
		id := dataOf(b)["documentId"].(string)
		c.do("PUT", base+"/"+id+"?locale=fr", fmt.Sprintf(`{"data":{"title":"%s fr"}}`, title))
		if i%2 == 0 {
			if s, b, _ := c.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+id+`"],"locale":"fr"}`); s != 200 || dataOf(b)["count"] != float64(1) {
				t.Fatalf("publish fr: %d %v", s, b)
			}
		}
	}
	filters := `{"views":{"$gte":0}}`
	q := url.Values{"locale": {"fr"}, "_q": {"go"}, "sort": {"views:desc"}, "filters": {filters}}
	for _, status := range []string{"draft", "published"} {
		q.Set("status", status)
		_, b, _ := c.do("GET", base+"?"+q.Encode(), "")
		var admin []string
		for _, r := range b["data"].([]any) {
			admin = append(admin, r.(map[string]any)["documentId"].(string))
		}
		rows, err := app.Documents().FindMany(context.Background(), "api::article.article", document.Params{
			"locale": "fr", "status": status, "_q": "go", "sort": "views:desc",
			"filters": map[string]any{"views": map[string]any{"$gte": 0}},
		})
		if err != nil {
			t.Fatal(err)
		}
		var docs []string
		for _, r := range rows {
			id, _ := r.Get("documentId")
			docs = append(docs, id.(string))
		}
		if strings.Join(admin, ",") != strings.Join(docs, ",") || len(docs) == 0 {
			t.Fatalf("%s: admin %v, document service %v", status, admin, docs)
		}
	}
}

// An update limited to title and summary leaves the other fields alone,
// whatever the request sends (sanitizeUpdateInput drops them).
func TestAdminFieldLevelUpdate(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	const base = "/content-manager/collection-types/api::article.article"
	_, b, _ := c.do("POST", base, `{"data":{"title":"Start","summary":"s","views":5}}`)
	id := dataOf(b)["documentId"].(string)
	_, b, _ = c.do("POST", "/roles", `{"name":"Editors of titles"}`)
	role := fmt.Sprint(int(dataOf(b)["id"].(float64)))
	perms := `{"permissions":[
		{"action":"plugin::content-manager.explorer.read","subject":"api::article.article","properties":{"fields":["title","summary","views"]},"conditions":[]},
		{"action":"plugin::content-manager.explorer.update","subject":"api::article.article","properties":{"fields":["title","summary"]},"conditions":[]}]}`
	if s, b, _ := c.do("PUT", "/roles/"+role+"/permissions", perms); s != 200 {
		t.Fatalf("permissions: %d %v", s, b)
	}
	_, b, _ = c.do("POST", "/users", `{"firstname":"Ed","email":"ed@example.com","roles":[`+role+`]}`)
	tok := dataOf(b)["registrationToken"].(string)
	ed := newAdminClient(t, ts.URL)
	_, b, _ = ed.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"Ed","password":"Ed1torPass"}}`)
	ed.token = dataOf(b)["token"].(string)
	_, b, _ = ed.do("GET", "/content-manager/models", "")
	raw, _ := json.Marshal(b)
	if !strings.Contains(string(raw), `"updateFields":["title","summary"]`) {
		t.Fatalf("the edit view learns which fields it may change: %s", raw)
	}
	if s, b, _ := ed.do("PUT", base+"/"+id, `{"data":{"title":"Edited","views":999}}`); s != 200 {
		t.Fatalf("update: %d %v", s, b)
	}
	_, b, _ = c.do("GET", base+"/"+id, "")
	d := dataOf(b)
	if d["title"] != "Edited" || d["views"] != float64(5) {
		t.Fatalf("title changes, views does not: %v", d)
	}
}

// A role without delete and with some fields unreadable: the list neither
// offers deletion nor shows those columns or values.
func TestAdminListPermissions(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	const base = "/content-manager/collection-types/api::article.article"
	c.do("POST", base, `{"data":{"title":"Visible","summary":"s","views":5}}`)
	_, b, _ := c.do("POST", "/roles", `{"name":"Readers"}`)
	role := fmt.Sprint(int(dataOf(b)["id"].(float64)))
	perms := `{"permissions":[{"action":"plugin::content-manager.explorer.read","subject":"api::article.article","properties":{"fields":["title"]},"conditions":[]}]}`
	if s, b, _ := c.do("PUT", "/roles/"+role+"/permissions", perms); s != 200 {
		t.Fatalf("permissions: %d %v", s, b)
	}
	_, b, _ = c.do("POST", "/users", `{"firstname":"Rea","email":"rea@example.com","roles":[`+role+`]}`)
	tok := dataOf(b)["registrationToken"].(string)
	r := newAdminClient(t, ts.URL)
	_, b, _ = r.do("POST", "/auth/register", `{"registrationToken":"`+tok+`","userInfo":{"firstname":"Rea","password":"Rea1derPass"}}`)
	r.token = dataOf(b)["token"].(string)
	_, b, _ = r.do("GET", "/content-manager/models", "")
	var model map[string]any
	for _, m := range b["data"].([]any) {
		if m := m.(map[string]any); m["uid"] == "api::article.article" {
			model = m
		}
	}
	if model == nil {
		t.Fatalf("the article model is readable: %v", b)
	}
	if model["permissions"].(map[string]any)["delete"] != false {
		t.Fatalf("no delete permission: %v", model["permissions"])
	}
	for _, col := range model["listColumns"].([]any) {
		if col == "summary" || col == "views" {
			t.Fatalf("unreadable column %v listed: %v", col, model["listColumns"])
		}
	}
	_, b, _ = r.do("GET", base, "")
	raw, _ := json.Marshal(b)
	if !strings.Contains(string(raw), `"Visible"`) || strings.Contains(string(raw), `"views"`) || strings.Contains(string(raw), `"summary"`) {
		t.Fatalf("only readable values: %s", raw)
	}
}

func TestAdminIncompleteDraft(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	const base = "/content-manager/collection-types/api::article.article"
	s, b, _ := c.do("POST", base, `{"data":{"summary":"no title yet"}}`)
	if s != 201 {
		t.Fatalf("a draft saves without its required title: %d %v", s, b)
	}
	id := dataOf(b)["documentId"].(string)
	s, b, _ = c.do("POST", base+"/actions/bulk-publish", `{"documentIds":["`+id+`"]}`)
	raw, _ := json.Marshal(b)
	if s != 200 || !strings.Contains(string(raw), `"path":["title"]`) {
		t.Fatalf("publishing names the title: %d %s", s, raw)
	}
}

// The edit view sends a relation's order as a set; the new order is what the
// content API returns.
func TestAdminRelationReorder(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "relations")
	defer stop()
	c := adminLogin(t, ts.URL)
	var tags []string
	for _, name := range []string{"a", "b", "c"} {
		_, b, _ := c.do("POST", "/content-manager/collection-types/api::tag.tag", `{"data":{"name":"`+name+`"}}`)
		tags = append(tags, dataOf(b)["documentId"].(string))
	}
	set := func(order ...int) string {
		var parts []string
		for _, i := range order {
			parts = append(parts, `{"documentId":"`+tags[i]+`"}`)
		}
		return `{"set":[` + strings.Join(parts, ",") + `]}`
	}
	const base = "/content-manager/collection-types/api::post.post"
	_, b, _ := c.do("POST", base, `{"data":{"title":"Order","tags":`+set(0, 1, 2)+`}}`)
	post := dataOf(b)["documentId"].(string)
	if s, b, _ := c.do("PUT", base+"/"+post, `{"data":{"tags":`+set(2, 0, 1)+`}}`); s != 200 {
		t.Fatalf("reorder: %d %v", s, b)
	}
	_, b, _ = c.do("POST", "/api-tokens", `{"name":"Read","type":"read-only","lifespan":null}`)
	key := dataOf(b)["accessKey"].(string)
	req, _ := http.NewRequest("GET", ts.URL+"/api/posts/"+post+"?status=draft&populate=tags", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	var names []string
	for _, tag := range dataOf(out)["tags"].([]any) {
		names = append(names, tag.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "c,a,b" {
		t.Fatalf("the content API returns the new order: %v", names)
	}
}
