package httpapi

import (
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
