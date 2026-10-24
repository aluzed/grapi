package grapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// strapi::security covers the admin too: its pages and its API cannot be
// framed by another site.
func TestAdminSecurityHeaders(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "basic")
	defer stop()
	for _, p := range []string{"/admin/", "/admin/auth/login", "/admin/api/v1/init"} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		csp := res.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-ancestors 'self'") || !strings.Contains(csp, "object-src 'none'") || res.Header.Get("X-Frame-Options") != "SAMEORIGIN" {
			t.Fatalf("%s: %v", p, res.Header)
		}
	}
}
