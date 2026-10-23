package document_test

import (
	"context"
	"testing"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
)

func titleOf(t *testing.T, row query.Row, attr string) any {
	t.Helper()
	rel, _ := row.Get(attr)
	r, ok := rel.(query.Row)
	if !ok {
		return nil
	}
	title, _ := r.Get("title")
	return title
}

// Discarding a draft recreates it: the links other drafts held on the old
// one follow the new one.
func TestDiscardDraftKeepsIncomingLinks(t *testing.T) {
	s := service(t, "relations")
	ctx := context.Background()
	docID := func(r query.Row) string { d, _ := r.Get("documentId"); return d.(string) }
	create := func(uid string, data map[string]any) string {
		t.Helper()
		row, err := s.Create(ctx, uid, document.Params{"data": data})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Publish(ctx, uid, docID(row), document.Params{}); err != nil {
			t.Fatal(err)
		}
		return docID(row)
	}
	a := create("api::post.post", map[string]any{"title": "A1"})
	b := create("api::post.post", map[string]any{"title": "B1", "parent": a})
	c := create("api::category.category", map[string]any{"name": "Hub", "highlight": a})
	if _, err := s.Update(ctx, "api::post.post", a, document.Params{"data": map[string]any{"title": "A2"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DiscardDraft(ctx, "api::post.post", a, document.Params{}); err != nil {
		t.Fatal(err)
	}
	draft := document.Params{"status": "draft", "populate": map[string]any{"highlight": true, "parent": true}}
	cat, err := s.FindOne(ctx, "api::category.category", c, draft)
	if err != nil {
		t.Fatal(err)
	}
	if got := titleOf(t, cat, "highlight"); got != "A1" {
		t.Fatalf("the draft category highlights the new draft: %v", got)
	}
	post, err := s.FindOne(ctx, "api::post.post", b, draft)
	if err != nil {
		t.Fatal(err)
	}
	if got := titleOf(t, post, "parent"); got != "A1" {
		t.Fatalf("the draft child keeps its parent: %v", got)
	}
	published := document.Params{"status": "published", "populate": map[string]any{"highlight": true}}
	if cat, _ = s.FindOne(ctx, "api::category.category", c, published); titleOf(t, cat, "highlight") != "A1" {
		t.Fatalf("the published link is untouched: %v", titleOf(t, cat, "highlight"))
	}
}

func TestNoDraftAndPublishActions(t *testing.T) {
	s := service(t, "relations")
	ctx := context.Background()
	row, err := s.Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "retro"}})
	if err != nil {
		t.Fatal(err)
	}
	if at, _ := row.Get("publishedAt"); at == nil {
		t.Fatal("publishedAt is always set without draft and publish")
	}
	doc, _ := row.Get("documentId")
	for name, action := range map[string]func(context.Context, string, string, document.Params) ([]query.Row, error){
		"publish": s.Publish, "unpublish": s.Unpublish, "discardDraft": s.DiscardDraft,
	} {
		if _, err := action(ctx, "api::tag.tag", doc.(string), document.Params{}); err == nil {
			t.Errorf("%s answers an error without draft and publish", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
