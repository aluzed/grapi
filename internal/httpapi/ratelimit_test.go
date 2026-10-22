package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrefixKey(t *testing.T) {
	cases := map[string]string{
		"/api/auth/local":               "noIdentifier:/api/auth/local:1.2.3.4",
		"/api/auth/local/":              "noIdentifier:/api/auth/local:1.2.3.4",
		"/api/Auth/Change-Password":     "noIdentifier:/api/auth/change-password:1.2.3.4",
		"/api/auth/forgot-password":     "a@b.c:/api/auth/forgot-password:1.2.3.4",
		"/api/auth/local/register":      "a@b.c:/api/auth/local/register:1.2.3.4",
		"/api/connect/github/callback/": "noIdentifier:/api/connect/github/callback:1.2.3.4",
	}
	for p, want := range cases {
		if got := prefixKey(p, "1.2.3.4", []byte(`{"email":"A@b.c"}`)); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
	}
	if got := prefixKey("/api/auth/forgot-password", "ip", []byte(`{}`)); got != "unknownIdentifier:/api/auth/forgot-password:ip" {
		t.Errorf("no email: %s", got)
	}
}

func TestLimiter(t *testing.T) {
	var l limiter
	cfg := RateLimit{Interval: time.Minute, Max: 2}
	now := time.Now()
	for i := range 3 {
		w := httptest.NewRecorder()
		ok := l.allow(w, "k", cfg, now)
		if ok != (i < 2) {
			t.Fatalf("hit %d allowed=%v", i+1, ok)
		}
		if i == 2 && w.Header().Get("Retry-After") == "" {
			t.Fatal("Retry-After missing")
		}
	}
	if !l.allow(httptest.NewRecorder(), "k", cfg, now.Add(61*time.Second)) {
		t.Fatal("a new window must start after the interval")
	}
}
