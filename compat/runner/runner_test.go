package runner

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi/compat/corpus"
)

func TestEncodeQuery(t *testing.T) {
	got := encodeQuery("filters[title][$eq]=Hello World&sort=a:asc&flag")
	want := "filters[title][$eq]=Hello+World&sort=a%3Aasc&flag"
	if got != want {
		t.Errorf("encodeQuery = %q, want %q", got, want)
	}
}

func TestRunSavesAndExpandsVariables(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"data":{"id":5,"documentId":"doc-a"}}`)
			return
		}
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"data":{"id":5,"documentId":"doc-a"}}`)
	}))
	defer srv.Close()

	files := []corpus.File{{Name: "f", Fixture: "x", Cases: []corpus.Case{
		{Name: "create", Method: "POST", Path: "/api/a", Save: map[string]string{"d": "data.documentId", "i": "data.id"}},
		{Name: "get", Method: "PUT", Path: "/api/a/{{d}}", Token: "full", Body: map[string]any{"data": map[string]any{"ref": "{{i}}"}}},
		{Name: "missing", Method: "GET", Path: "/api/a/{{nope}}"},
	}}}
	out := t.TempDir()
	if err := Run(Target{BaseURL: srv.URL, Tokens: map[string]string{"full": "k"}}, files, out); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/a/doc-a" || gotAuth != "Bearer k" {
		t.Errorf("path=%q auth=%q", gotPath, gotAuth)
	}
	if ref := gotBody["data"].(map[string]any)["ref"]; ref != float64(5) {
		t.Errorf("numeric placeholder sent as %#v, want 5", ref)
	}
	raw, err := os.ReadFile(filepath.Join(out, "f", "003-missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res Result
	_ = json.Unmarshal(raw, &res)
	if res.Error == "" {
		t.Error("undefined variable must be reported as an error")
	}
}
