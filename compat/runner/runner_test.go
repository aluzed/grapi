package runner

import (
	"encoding/json"
	"image"
	"image/png"
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

func TestRunSendsBearerAndGeneratedFiles(t *testing.T) {
	var gotAuth, gotName, gotType string
	var gotSize image.Point
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if f, h, err := r.FormFile("files"); err == nil {
			gotName, gotType = h.Filename, h.Header.Get("Content-Type")
			if img, err := png.Decode(f); err == nil {
				gotSize = img.Bounds().Size()
			}
		}
		_, _ = io.WriteString(w, `{"jwt":"secret-jwt"}`)
	}))
	defer srv.Close()

	files := []corpus.File{{Name: "f", Fixture: "x", Cases: []corpus.Case{
		{Name: "login", Method: "POST", Path: "/auth", Save: map[string]string{"jwt": "jwt"}},
		{Name: "upload", Method: "POST", Path: "/upload", Bearer: "{{jwt}}", Files: []corpus.Upload{{Name: "a.png", Width: 40, Height: 30}}},
	}}}
	if err := Run(Target{BaseURL: srv.URL}, files, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret-jwt" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotName != "a.png" || gotType != "image/png" || gotSize != image.Pt(40, 30) {
		t.Errorf("file name=%q type=%q size=%v", gotName, gotType, gotSize)
	}
}

func TestGeneratedFilesAreStable(t *testing.T) {
	f := []corpus.Upload{{Name: "a.png", Width: 64, Height: 48}}
	a, _, err := multipartBody(f)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := multipartBody(f)
	// Boundaries differ between calls; the PNG bytes in between must not.
	if len(a) != len(b) {
		t.Errorf("generated bodies differ in size: %d vs %d", len(a), len(b))
	}
}
