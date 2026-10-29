package grapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

// Strapi expands a nested "*" to the target's readable populatable
// attributes, then its document service rejects them: answers captured from
// Strapi 5 on dev-freelance (posts with a cover and an author whose user
// schema has a picture).
func TestNestedWildcardPopulate(t *testing.T) {
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(_ *grapi.App, dir string) {
		p := filepath.Join(dir, "src", "api", "article", "content-types", "article", "schema.json")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), `"attributes": {`, `"attributes": {
    "cover": { "type": "media", "multiple": false, "allowedTypes": ["images"] },
    "author": { "type": "relation", "relation": "manyToOne", "target": "plugin::users-permissions.user" },`, 1))
		if err := os.WriteFile(p, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		user, err := os.ReadFile(filepath.Join("internal", "builtin", "schemas", "users-permissions", "content-types", "user", "schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var s map[string]any
		if err := json.Unmarshal(user, &s); err != nil {
			t.Fatal(err)
		}
		s["attributes"].(map[string]any)["picture"] = map[string]any{"type": "media", "multiple": false, "allowedTypes": []any{"images"}}
		ext := filepath.Join(dir, "src", "extensions", "users-permissions", "content-types", "user")
		if err := os.MkdirAll(ext, 0o750); err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(ext, "schema.json"), out, 0o600); err != nil {
			t.Fatal(err)
		}
	})
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("GET", "/users-permissions/roles", "")
	var publicID string
	for _, r := range b["data"].([]any) {
		if m := r.(map[string]any); m["type"] == "public" {
			publicID = strconv.Itoa(int(m["id"].(float64)))
		}
	}
	if s, b, _ := c.do("PUT", "/users-permissions/roles/"+publicID, `{"permissions":["api::article.article.find","plugin::users-permissions.user.find","plugin::upload.content-api.find"]}`); s != 200 {
		t.Fatalf("grant: %d %v", s, b)
	}
	get := func(q string) (int, map[string]any) {
		res, err := http.Get(ts.URL + "/api/articles?" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out
	}
	for q, want := range map[string]string{
		"populate[cover]=*":                     `{"key":"related","path":"cover.related"}|Invalid key related at cover.related`,
		"populate[author]=*":                    `{"key":"picture","path":"author.picture"}|Invalid key picture at author.picture`,
		"populate[author][populate][picture]=*": `{"key":"related","path":"author.picture.related"}|Invalid key related at author.picture.related`,
	} {
		details, msg, _ := strings.Cut(want, "|")
		s, b := get(q)
		e, _ := b["error"].(map[string]any)
		got, _ := json.Marshal(e["details"])
		if s != 400 || e["name"] != "ValidationError" || e["message"] != msg || string(got) != details {
			t.Errorf("%s: %d %v", q, s, b)
		}
	}
	for _, q := range []string{"populate=*", "populate[author]=true", "populate[author][populate]=*", "populate[cover]=true"} {
		if s, b := get(q); s != 200 {
			t.Errorf("%s: %d %v", q, s, b)
		}
	}
}
