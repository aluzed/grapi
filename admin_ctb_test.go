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

// A TypeScript project gets the .ts core files, and existing ones are kept.
func TestAdminContentTypeBuilderCoreFilesTS(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "basic", true)
	defer stop()
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(dir, "src", "api", "book", "services", "book.ts")
	if err := os.MkdirAll(filepath.Dir(custom), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("// mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := adminLogin(t, ts.URL)
	batch := `{"contentTypes":[{"action":"create","data":{"kind":"collectionType","info":{"displayName":"Book","singularName":"book","pluralName":"books"},"options":{},"attributes":[{"name":"title","type":"string"}]}}]}`
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", batch); s != 200 {
		t.Fatalf("create: %d %v", s, b)
	}
	router, err := os.ReadFile(filepath.Join(dir, "src", "api", "book", "routes", "book.ts"))
	if err != nil || string(router) != "/**\n * book router\n */\n\nimport { factories } from '@strapi/strapi';\n\nexport default factories.createCoreRouter('api::book.book');\n" {
		t.Fatalf("router: %q %v", router, err)
	}
	if kept, _ := os.ReadFile(custom); string(kept) != "// mine\n" {
		t.Fatalf("an existing file is kept: %q", kept)
	}
}
