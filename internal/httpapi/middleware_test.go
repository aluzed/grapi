package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Captured from Strapi 5.59 with the default config/middlewares.
const strapiCSP = "connect-src 'self' https:;img-src 'self' data: blob: https://market-assets.strapi.io;media-src 'self' data: blob:;default-src 'self';base-uri 'self';font-src 'self' https: data:;form-action 'self';frame-ancestors 'self';object-src 'none';script-src 'self';script-src-attr 'none';style-src 'self' https: 'unsafe-inline'"

func serve(m Middlewares, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	m.wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(w, r)
	return w
}

func TestDefaultHeadersMatchStrapi(t *testing.T) {
	w := serve(DefaultMiddlewares(), httptest.NewRequest(http.MethodGet, "/api/articles", nil))
	want := map[string]string{
		"Content-Security-Policy":           strapiCSP,
		"Referrer-Policy":                   "no-referrer",
		"Strict-Transport-Security":         "max-age=31536000; includeSubDomains",
		"X-Content-Type-Options":            "nosniff",
		"X-DNS-Prefetch-Control":            "off",
		"X-Download-Options":                "noopen",
		"X-Frame-Options":                   "SAMEORIGIN",
		"X-Permitted-Cross-Domain-Policies": "none",
		"Vary":                              "Origin",
		"Access-Control-Allow-Credentials":  "true",
	}
	for k, v := range want {
		got := w.Header().Get(k)
		if raw, ok := w.Header()[k]; ok {
			got = raw[0]
		}
		if got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if v, ok := w.Header()["Access-Control-Allow-Origin"]; !ok || v[0] != "" {
		t.Errorf("Access-Control-Allow-Origin = %v, Strapi sends it empty without Origin", v)
	}
	if w.Header().Get("X-Powered-By") != "" {
		t.Error("X-Powered-By must not be sent")
	}
}

func TestCORS(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "/api/articles", nil)
	r.Header.Set("Origin", "https://example.com")
	r.Header.Set("Access-Control-Request-Method", "POST")
	w := serve(DefaultMiddlewares(), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", w.Code)
	}
	for k, v := range map[string]string{
		"Access-Control-Allow-Origin":  "https://example.com",
		"Access-Control-Max-Age":       "31536000",
		"Access-Control-Allow-Methods": "GET,POST,PUT,PATCH,DELETE,HEAD,OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type,Authorization,Origin,Accept",
	} {
		if got := w.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	m, err := ParseMiddlewares([]any{"strapi::security", map[string]any{"name": "strapi::cors", "config": map[string]any{"origin": []any{"https://ok.example"}}}})
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/articles", nil)
	r.Header.Set("Origin", "https://evil.example")
	w = serve(m, r)
	if _, ok := w.Header()["Access-Control-Allow-Origin"]; ok || w.Header().Get("Vary") != "Origin" {
		t.Errorf("refused origin got headers %v", w.Header())
	}
	m, _ = ParseMiddlewares([]any{map[string]any{"name": "strapi::security", "config": map[string]any{
		"contentSecurityPolicy": map[string]any{"directives": map[string]any{"img-src": []any{"'self'", "data:", "cdn.example"}}},
	}}})
	w = serve(m, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w.Header().Get("Content-Security-Policy"); got[:len("connect-src 'self' https:;img-src 'self' data: cdn.example;")] != "connect-src 'self' https:;img-src 'self' data: cdn.example;" {
		t.Errorf("CSP = %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Vary") != "" {
		t.Error("cors left out of the config must not run")
	}
}

func TestCheckRequiredMiddlewares(t *testing.T) {
	all := []any{"strapi::logger", "strapi::errors", "strapi::security", map[string]any{"name": "strapi::cors", "config": map[string]any{}}, "strapi::poweredBy", "strapi::query", "strapi::body", "strapi::session", "strapi::favicon", "strapi::public"}
	if err := CheckRequiredMiddlewares(all); err != nil {
		t.Fatal(err)
	}
	err := CheckRequiredMiddlewares([]any{"strapi::errors", "strapi::query", "strapi::body", "strapi::public", "strapi::favicon"})
	want := `Missing required middlewares in configuration. Add the following middlewares: "strapi::security, strapi::cors".`
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}
