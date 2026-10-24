package grapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestPreview(t *testing.T) {
	conf := `preview:
  enabled: true
  allowedOrigins: ["https://front.test"]
  handlers:
    api::post.post: "https://front.test/blog/{title}?status={status}&token={previewToken}"
`
	ts, _, stop, _ := newAdminAppWith(t, "relations", false, func(app *grapi.App, dir string) {
		if err := os.WriteFile(filepath.Join(dir, "config", "admin.yaml"), []byte(conf), 0o600); err != nil {
			t.Fatal(err)
		}
		app.PreviewHandler("api::writer.writer", func(context.Context, grapi.PreviewRequest) (string, error) { return "", nil })
		app.PreviewHandler("api::category.category", func(_ context.Context, r grapi.PreviewRequest) (string, error) {
			return "https://elsewhere.test/c/" + r.DocumentID, nil
		})
	})
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	create := func(uid, data string) string {
		t.Helper()
		_, b, _ := c.do("POST", "/content-manager/collection-types/"+uid, `{"data":`+data+`}`)
		return dataOf(b)["documentId"].(string)
	}
	post, other := create("api::post.post", `{"title":"Hello World"}`), create("api::post.post", `{"title":"Other"}`)

	s, b, _ := c.do("GET", "/preview/url/api::post.post?documentId="+post+"&status=draft", "")
	raw, _ := dataOf(b)["url"].(string)
	u, err := url.Parse(raw)
	if s != 200 || err != nil || u.Path != "/blog/Hello World" || !strings.Contains(raw, "Hello%20World") || u.Query().Get("status") != "draft" || u.Query().Get("token") == "" {
		t.Fatalf("preview url: %d %v", s, b)
	}
	token := u.Query().Get("token")

	api := func(path, bearer string) (int, map[string]any) {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if s, b := api("/api/posts/"+post+"?status=draft", token); s != 200 || dataOf(b)["title"] != "Hello World" {
		t.Fatalf("the token reads its draft: %d %v", s, b)
	}
	if s, b := api("/api/posts/"+post+"?status=draft&previewToken="+url.QueryEscape(token), ""); s != 200 {
		t.Fatalf("or in the query string: %d %v", s, b)
	}
	if s, b := api("/api/posts?status=draft", token); s != 200 || len(b["data"].([]any)) != 1 {
		t.Fatalf("a list holds that document only: %d %v", s, b)
	}
	if s, _ := api("/api/posts/"+other+"?status=draft", token); s != 403 {
		t.Fatalf("another document: %d", s)
	}
	if s, _ := api("/api/tags", token); s != 403 {
		t.Fatalf("another type: %d", s)
	}

	if s, _, _ := c.do("GET", "/preview/url/api::tag.tag?documentId=x", ""); s != 204 {
		t.Fatalf("a type without handler: %d", s)
	}
	writer := create("api::writer.writer", `{"name":"Ada"}`)
	if s, _, _ := c.do("GET", "/preview/url/api::writer.writer?documentId="+writer, ""); s != 204 {
		t.Fatalf("a Go handler that answers \"\": %d", s)
	}
	cat := create("api::category.category", `{"name":"News"}`)
	if s, b, _ := c.do("GET", "/preview/url/api::category.category?documentId="+cat, ""); s != 400 || !strings.Contains(previewErr(b), "allowedOrigins") {
		t.Fatalf("an origin out of the list: %d %v", s, b)
	}
	res, err := http.Get(ts.URL + "/admin/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'self' https://front.test") {
		t.Fatalf("frame-src: %s", csp)
	}
}

func previewErr(b map[string]any) string {
	e, _ := b["error"].(map[string]any)
	m, _ := e["message"].(string)
	return m
}
