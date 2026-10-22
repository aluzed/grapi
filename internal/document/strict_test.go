package document_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/aluzed/grapi/internal/document"
)

const strictWriter = `{"kind":"collectionType","collectionName":"writers","info":{"singularName":"writer","pluralName":"writers","displayName":"Writer"},
"options":{"draftAndPublish":false},"attributes":{"name":{"type":"string"}}}`

const strictBook = `{"kind":"collectionType","collectionName":"books","info":{"singularName":"book","pluralName":"books","displayName":"Book"},
"options":{"draftAndPublish":true},"attributes":{"title":{"type":"string"},
"writer":{"type":"relation","relation":"manyToOne","target":"api::writer.writer","required":true},
"cover":{"type":"media","multiple":false,"required":true,"allowedTypes":["images"]}}}`

func TestStrictRelations(t *testing.T) {
	s := serviceOf(t, fstest.MapFS{
		"src/api/writer/content-types/writer/schema.json": {Data: []byte(strictWriter)},
		"src/api/book/content-types/book/schema.json":     {Data: []byte(strictBook)},
	})
	ctx := context.Background()
	w, err := s.Create(ctx, "api::writer.writer", document.Params{"data": map[string]any{"name": "Ada"}})
	if err != nil {
		t.Fatal(err)
	}
	wid, _ := w.Get("documentId")
	published := document.Params{"status": "published", "data": map[string]any{"title": "x"}}
	if _, err := s.Create(ctx, "api::book.book", published); err != nil {
		t.Fatalf("without strictRelations required relations are not enforced: %v", err)
	}
	s.StrictRelations = true
	_, err = s.Create(ctx, "api::book.book", document.Params{"status": "published", "data": map[string]any{"title": "x", "writer": wid}})
	var ve *document.ValidationError
	if !errors.As(err, &ve) || ve.Msg != "cover must be defined." {
		t.Fatalf("missing cover: %v", err)
	}
	_, err = s.Create(ctx, "api::book.book", document.Params{"status": "published", "data": map[string]any{"title": "x", "writer": map[string]any{"connect": []any{}}}})
	if !errors.As(err, &ve) || ve.Msg != "2 errors occurred" {
		t.Fatalf("empty connect and no cover: %v", err)
	}
	draft, err := s.Create(ctx, "api::book.book", document.Params{"data": map[string]any{"title": "draft"}})
	if err != nil {
		t.Fatalf("drafts are not checked: %v", err)
	}
	id, _ := draft.Get("documentId")
	if _, err := s.Update(ctx, "api::book.book", id.(string), document.Params{"data": map[string]any{"title": "kept"}}); err != nil {
		t.Fatalf("an update leaving the relations out keeps them: %v", err)
	}
}
