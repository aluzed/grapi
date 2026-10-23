package httpapi

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseBytes(t *testing.T) {
	for in, want := range map[any]int64{"56kb": 57344, "1mb": 1 << 20, "1.5MB": 3 << 19, "100": 100, 2048: 2048} {
		if got, ok := parseBytes(in); !ok || got != want {
			t.Errorf("%v: %d %v", in, got, ok)
		}
	}
}

func TestBodyLimits(t *testing.T) {
	m, err := ParseMiddlewares([]any{map[string]any{"name": "strapi::body", "config": map[string]any{"jsonLimit": "10b", "textLimit": 4}}})
	if err != nil {
		t.Fatal(err)
	}
	h := m.wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	call := func(ct, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/x", strings.NewReader(body))
		req.Header.Set("Content-Type", ct)
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := call("application/json", `{"a":"12"}`); c != 204 {
		t.Errorf("10 bytes of JSON: %d", c)
	}
	if c := call("application/json; charset=utf-8", `{"a":"123"}`); c != 413 {
		t.Errorf("11 bytes of JSON: %d", c)
	}
	if c := call("text/plain", "hello"); c != 413 {
		t.Errorf("text over 4 bytes: %d", c)
	}
	if c := call("multipart/form-data; boundary=x", strings.Repeat("x", 100)); c != 204 {
		t.Errorf("multipart is left to the upload: %d", c)
	}
}

func TestMaxFileSizeStreams(t *testing.T) {
	m, err := ParseMiddlewares([]any{map[string]any{"name": "strapi::body", "config": map[string]any{"formidable": map[string]any{"maxFileSize": 1000}}}})
	if err != nil || m.Body.MaxFileSize != 1000 {
		t.Fatalf("formidable.maxFileSize: %+v %v", m.Body, err)
	}
	if DefaultBodyLimits.MaxFileSize != 200<<20 {
		t.Fatal("formidable's default is 200 MiB")
	}
	// 600 bytes twice: the limit counts the file data of the whole request.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, name := range []string{"a.bin", "b.bin"} {
		f, _ := mw.CreateFormFile("files", name)
		_, _ = f.Write(bytes.Repeat([]byte{1}, 600))
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/upload", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	dir := t.TempDir()
	if _, err := readMultipart(withMaxFileSize(r, 1000), dir); !errors.Is(err, errFileTooBig) {
		t.Fatalf("over the limit: %v", err)
	}
}
