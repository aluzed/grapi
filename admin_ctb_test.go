package grapi_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Removing one side of a bidirectional relation removes the other, from
// either side.
func TestAdminContentTypeBuilderDropsInverse(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "i18n-dp", true)
	defer stop()
	c := adminLogin(t, ts.URL)
	_, b, _ := c.do("GET", "/content-type-builder/schema", "")
	types := map[string]map[string]any{}
	for _, x := range dataOf(b)["contentTypes"].([]any) {
		m := x.(map[string]any)
		types[m["uid"].(string)] = m
	}
	without := func(uid, field string) map[string]any {
		ct := types[uid]
		var kept []any
		for _, a := range ct["attributes"].([]any) {
			if a.(map[string]any)["name"] != field {
				kept = append(kept, a)
			}
		}
		ct["attributes"] = kept
		return map[string]any{"action": "update", "uid": uid, "data": ct}
	}
	batch, _ := json.Marshal(map[string]any{"contentTypes": []any{without("api::article.article", "category"), without("api::writer.writer", "articles")}})
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", string(batch)); s != 200 {
		t.Fatalf("update schema: %d %v", s, b)
	}
	attrs := func(name string) map[string]any {
		raw, err := os.ReadFile(filepath.Join(dir, "src", "api", name, "content-types", name, "schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var s map[string]any
		_ = json.Unmarshal(raw, &s)
		return s["attributes"].(map[string]any)
	}
	if _, ok := attrs("category")["articles"]; ok {
		t.Error("category.articles stays after article.category was removed")
	}
	if _, ok := attrs("article")["writer"]; ok {
		t.Error("article.writer stays after writer.articles was removed")
	}
	if _, ok := attrs("article")["title"]; !ok {
		t.Error("the other fields stay")
	}
}
