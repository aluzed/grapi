package grapi_test

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func newRateLimitServer(t *testing.T, server string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "server.yaml"), []byte(server), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(func() { ts.Close(); _ = srv.Close() })
	return ts
}

// loginStatuses sends 11 failed logins, each with its own identifier and
// the X-Forwarded-For of forwarded(i), and returns their statuses.
func loginStatuses(t *testing.T, ts *httptest.Server, forwarded func(i int) string) []int {
	t.Helper()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	var out []int
	for i := range 11 {
		body := fmt.Sprintf(`{"identifier":"user%d@example.com","password":"nope"}`, i)
		s, b := c.do("POST", "/api/auth/local", "", body, "X-Forwarded-For", forwarded(i))
		if s == 429 && errorMessage(b) != "Too many requests, please try again later." {
			t.Fatalf("429 body: %v", b)
		}
		out = append(out, s)
	}
	return out
}

func TestLoginRateLimitIgnoresIdentifier(t *testing.T) {
	// A boolean server.proxy trusts nothing, as in Strapi 5 (proxy.koa only).
	for _, server := range []string{"proxy: false\n", "proxy: true\n"} {
		ts := newRateLimitServer(t, server)
		got := loginStatuses(t, ts, func(i int) string { return fmt.Sprintf("10.0.0.%d", i) })
		for i, s := range got[:10] {
			if s != 400 {
				t.Fatalf("%s attempt %d: %d", server, i+1, s)
			}
		}
		if got[10] != 429 {
			t.Fatalf("%s a new identifier and a spoofed X-Forwarded-For do not reset the limit: %v", server, got)
		}
	}
}

func TestLoginRateLimitUsesProxyClientIP(t *testing.T) {
	ts := newRateLimitServer(t, "proxy:\n  koa: true\n")
	got := loginStatuses(t, ts, func(i int) string { return fmt.Sprintf("10.0.0.%d", i) })
	if strings.Contains(fmt.Sprint(got), "429") {
		t.Fatalf("behind a trusted proxy each client IP has its own counter: %v", got)
	}
	got = loginStatuses(t, ts, func(int) string { return "10.0.1.1" })
	if got[10] != 429 {
		t.Fatalf("one client IP behind the proxy is limited: %v", got)
	}
	// maxIpsCount 1 keeps the address the last proxy saw: a forged first
	// entry does not escape the limit.
	ts = newRateLimitServer(t, "proxy:\n  koa: true\n  maxIpsCount: 1\n")
	got = loginStatuses(t, ts, func(i int) string { return fmt.Sprintf("10.9.9.%d, 10.0.2.2", i) })
	if got[10] != 429 {
		t.Fatalf("maxIpsCount: %v", got)
	}
}
