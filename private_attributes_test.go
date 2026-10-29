package grapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

// api.responses.privateAttributes and a model's options.privateAttributes
// hide attributes like private: true does.
func TestStoredPrivateAttributes(t *testing.T) {
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(_ *grapi.App, dir string) {
		p := filepath.Join(dir, "src", "api", "article", "content-types", "article", "schema.json")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), `"options": { "draftAndPublish": true }`, `"options": { "draftAndPublish": true, "privateAttributes": ["summary"] }`, 1))
		if err := os.WriteFile(p, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config", "api.yaml"), []byte("responses:\n  privateAttributes: [contact]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("POST", "/api-tokens", `{"name":"All","type":"full-access","lifespan":30}`)
	key := dataOf(b)["accessKey"].(string)
	api := func(method, path, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out
	}
	if s, b := api("POST", "/api/articles?status=published", `{"data":{"title":"Hello","summary":"hidden","contact":"a@example.com"}}`); s != 201 {
		t.Fatalf("create: %d %v", s, b)
	}
	_, b = api("GET", "/api/articles", "")
	art := b["data"].([]any)[0].(map[string]any)
	if art["title"] != "Hello" || art["summary"] != nil || art["contact"] != nil {
		t.Fatalf("stored private attributes leak: %v", art)
	}
	for _, f := range []string{"summary", "contact"} {
		s, b := api("GET", "/api/articles?filters["+f+"][$eq]=x", "")
		if s != 400 || b["error"].(map[string]any)["message"] != "Invalid key "+f {
			t.Fatalf("filter on %s: %d %v", f, s, b)
		}
	}
}
