package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aluzed/grapi/internal/clientip"
	"github.com/aluzed/grapi/schema"
)

// The auth mutations replay the request through the REST handlers: they must
// keep the caller's IP, or every client shares the proxy's rate limit bucket.
func TestGraphQLRESTKeepsClientIP(t *testing.T) {
	s := &Server{cfg: Config{Proxy: clientip.Proxy{Koa: true, IPHeader: "X-Real-Ip"}}, set: &schema.Set{}}
	r := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Real-Ip", "203.0.113.7")
	var ip string
	g := &gqlResolver{s: s, r: r, auth: &Auth{}}
	_, err := g.rest(func(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
		ip = s.cfg.Proxy.IP(r)
		w.WriteHeader(http.StatusOK)
		return nil
	}, http.MethodPost, map[string]any{}, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ip != "203.0.113.7" {
		t.Fatalf("client IP = %q, want the X-Real-Ip of the GraphQL request", ip)
	}
}
