package grapi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReloadGate(t *testing.T) {
	g := &reloadGate{wait: 200 * time.Millisecond}
	var version atomic.Int32
	version.Store(1)
	release := make(chan struct{})
	h := g.handler(func() http.Handler {
		v := version.Load()
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/slow" {
				<-release
			}
			// The version a request ends with is the one it started with.
			if version.Load() != v {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	})
	do := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}

	slow := make(chan int)
	go func() { slow <- do("/slow").Code }()
	time.Sleep(20 * time.Millisecond)
	begun := make(chan struct{})
	go func() {
		g.begin()
		close(begun)
	}()
	select {
	case <-begun:
		t.Fatal("the reload started with a request in flight")
	case <-time.After(50 * time.Millisecond):
	}
	waiting := make(chan int)
	go func() { waiting <- do("/").Code }()
	close(release)
	if code := <-slow; code != 200 {
		t.Fatalf("the request in flight finishes on its version: %d", code)
	}
	<-begun
	version.Store(2)
	time.Sleep(20 * time.Millisecond)
	g.end()
	if code := <-waiting; code != 200 {
		t.Fatalf("a request arriving during the reload is served by the new version: %d", code)
	}

	g.begin()
	rec := do("/")
	g.end()
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("a reload longer than the wait answers 503 with Retry-After: %d %v", rec.Code, rec.Header())
	}
	if do("/").Code != 200 {
		t.Fatal("served again after the reload")
	}
}
