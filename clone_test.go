package grapi_test

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

func cloneApp(t *testing.T, fixture string) *grapi.App {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", fixture, "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return app
}

func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func TestDocumentClone(t *testing.T) {
	app := cloneApp(t, "relations")
	ctx := context.Background()
	docs := app.Documents()
	id := func(e grapi.Entry) string { v, _ := e.Get("documentId"); return v.(string) }
	writer, _ := docs.Create(ctx, "api::writer.writer", document.Params{"data": map[string]any{"name": "Ada"}})
	tag, _ := docs.Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "go"}})
	post, err := docs.Create(ctx, "api::post.post", document.Params{"data": map[string]any{
		"title": "Original", "writer": id(writer), "tags": []any{id(tag)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docs.Publish(ctx, "api::post.post", id(post), document.Params{}); err != nil {
		t.Fatal(err)
	}
	res, err := docs.Clone(ctx, "api::post.post", id(post), document.Params{"data": map[string]any{"title": "Copy"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.DocumentID == "" || res.DocumentID == id(post) {
		t.Fatalf("the draft only, as a new document: %+v", res)
	}
	got, _ := docs.FindOne(ctx, "api::post.post", res.DocumentID, document.Params{"populate": []any{"writer", "tags"}})
	g := asJSON(t, got)
	if g["title"] != "Copy" || g["publishedAt"] != nil || g["writer"].(map[string]any)["name"] != "Ada" || len(g["tags"].([]any)) != 1 {
		t.Fatalf("clone: %v", g)
	}
	orig, _ := docs.FindOne(ctx, "api::post.post", id(post), document.Params{})
	if title, _ := orig.Get("title"); title != "Original" {
		t.Fatalf("the original is untouched: %v", title)
	}
	if res, _ := docs.Clone(ctx, "api::post.post", "nope", document.Params{}); res == nil || len(res.Entries) != 0 || res.DocumentID != "" {
		t.Fatalf("an unknown document clones nothing: %+v", res)
	}
}

func TestDocumentCloneComponents(t *testing.T) {
	app := cloneApp(t, "components")
	ctx := context.Background()
	docs := app.Documents()
	page, err := docs.Create(ctx, "api::page.page", document.Params{"data": map[string]any{
		"title": "Home",
		"seo":   map[string]any{"metaTitle": "Home"},
		"links": []any{map[string]any{"label": "a"}, map[string]any{"label": "b"}},
		"body":  []any{map[string]any{"__component": "blocks.hero", "headline": "Hi", "cta": map[string]any{"label": "Go"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := page.Get("documentId")
	// lodash's merge goes into arrays by index: the first link changes, the second stays.
	res, err := docs.Clone(ctx, "api::page.page", pid.(string), document.Params{"data": map[string]any{"links": []any{map[string]any{"label": "A"}}}})
	if err != nil {
		t.Fatal(err)
	}
	all := document.Params{"populate": map[string]any{"seo": true, "links": true, "body": map[string]any{"on": map[string]any{"blocks.hero": map[string]any{"populate": map[string]any{"cta": true}}}}}}
	c, _ := docs.FindOne(ctx, "api::page.page", res.DocumentID, all)
	o, _ := docs.FindOne(ctx, "api::page.page", pid.(string), all)
	cj, oj := asJSON(t, c), asJSON(t, o)
	labels := func(m map[string]any) []any {
		var out []any
		for _, l := range m["links"].([]any) {
			out = append(out, l.(map[string]any)["label"])
		}
		return out
	}
	if l := labels(cj); len(l) != 2 || l[0] != "A" || l[1] != "b" {
		t.Fatalf("merged links: %v", l)
	}
	if l := labels(oj); l[0] != "a" {
		t.Fatalf("the original's links: %v", l)
	}
	if cj["seo"].(map[string]any)["id"] == oj["seo"].(map[string]any)["id"] {
		t.Fatalf("components are copied, not shared: %v %v", cj["seo"], oj["seo"])
	}
	hero := cj["body"].([]any)[0].(map[string]any)
	if hero["headline"] != "Hi" || hero["cta"].(map[string]any)["label"] != "Go" {
		t.Fatalf("dynamic zone: %v", hero)
	}
}
