package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Captured from Strapi 5 with server.url https://bo.example.com.
func TestRootRedirectsToAdmin(t *testing.T) {
	s := &Server{cfg: Config{RootRedirect: "https://bo.example.com/admin"}, log: slog.Default()}
	for _, tc := range []struct{ method, accept, contentType, body string }{
		{http.MethodGet, "", "text/html; charset=utf-8", "Redirecting to https://bo.example.com/admin."},
		{http.MethodGet, "*/*", "text/html; charset=utf-8", "Redirecting to https://bo.example.com/admin."},
		{http.MethodGet, "text/html", "text/html; charset=utf-8", "Redirecting to https://bo.example.com/admin."},
		{http.MethodGet, "application/json", "text/plain; charset=utf-8", "Redirecting to https://bo.example.com/admin."},
		{http.MethodGet, "text/html;q=0, application/json", "text/plain; charset=utf-8", "Redirecting to https://bo.example.com/admin."},
		{http.MethodHead, "", "text/html; charset=utf-8", ""},
	} {
		r := httptest.NewRequest(tc.method, "/", nil)
		if tc.accept != "" {
			r.Header.Set("Accept", tc.accept)
		}
		w := httptest.NewRecorder()
		s.route(w, r)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "https://bo.example.com/admin" {
			t.Errorf("%s Accept %q: %d Location %q", tc.method, tc.accept, w.Code, w.Header().Get("Location"))
		}
		if got := w.Header().Get("Content-Type"); got != tc.contentType {
			t.Errorf("%s Accept %q: Content-Type %q, want %q", tc.method, tc.accept, got, tc.contentType)
		}
		if w.Body.String() != tc.body {
			t.Errorf("%s Accept %q: body %q, want %q", tc.method, tc.accept, w.Body.String(), tc.body)
		}
	}
}

func TestRootPostIsMethodNotAllowed(t *testing.T) {
	s := &Server{cfg: Config{RootRedirect: "/admin"}, log: slog.Default()}
	w := httptest.NewRecorder()
	s.route(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / = %d, want 405", w.Code)
	}
}
