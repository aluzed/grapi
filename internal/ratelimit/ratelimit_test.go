package ratelimit

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	var l Limiter
	cfg := Config{Interval: time.Minute, Max: 2}
	now := time.Now()
	for i := range 3 {
		w := httptest.NewRecorder()
		ok := l.Allow(w, "k", cfg, now)
		if ok != (i < 2) {
			t.Fatalf("hit %d allowed=%v", i+1, ok)
		}
		if i == 2 && w.Header().Get("Retry-After") == "" {
			t.Fatal("Retry-After missing")
		}
	}
	if !l.Allow(httptest.NewRecorder(), "k", cfg, now.Add(61*time.Second)) {
		t.Fatal("a new window must start after the interval")
	}
}
