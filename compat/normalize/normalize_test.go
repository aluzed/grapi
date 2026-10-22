package normalize

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValueAliasesAcrossResponses(t *testing.T) {
	n := New()
	a := n.Value(decode(t, `{"data":{"id":7,"documentId":"abc","createdAt":"2026-01-01T00:00:00.000Z","publishedAt":null,"title":"x"}}`))
	b := n.Value(decode(t, `{"data":[{"id":9,"documentId":"zzz"},{"id":7,"documentId":"abc"}]}`))

	wantA := decode(t, `{"data":{"id":"#id1","documentId":"#doc1","createdAt":"<ts>","publishedAt":null,"title":"x"}}`)
	wantB := decode(t, `{"data":[{"id":"#id2","documentId":"#doc2"},{"id":"#id1","documentId":"#doc1"}]}`)
	if !reflect.DeepEqual(a, wantA) {
		t.Errorf("first response = %v, want %v", a, wantA)
	}
	if !reflect.DeepEqual(b, wantB) {
		t.Errorf("second response = %v, want %v", b, wantB)
	}
}

func TestValueMasksSecretsAndUploads(t *testing.T) {
	got := New().Value(decode(t, `{"jwt":"eyJ","url":"/uploads/a_123.png","link":"/uploads/keep"}`))
	want := decode(t, `{"jwt":"<secret>","url":"<upload-url>","link":"/uploads/keep"}`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
