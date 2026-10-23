package grapi

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// reloadGate keeps requests and reloads apart: a reload starts once the
// requests in flight are done, and the requests that arrive meanwhile wait
// for the new version, so that none sees a schema half synced.
type reloadGate struct {
	mu        sync.Mutex
	reloading chan struct{}
	inflight  sync.WaitGroup
	wait      time.Duration
}

// enter admits a request, waiting for a reload in progress; false when the
// reload outlasts the wait.
func (g *reloadGate) enter(ctx context.Context) bool {
	for {
		g.mu.Lock()
		done := g.reloading
		if done == nil {
			g.inflight.Add(1)
			g.mu.Unlock()
			return true
		}
		g.mu.Unlock()
		timer := time.NewTimer(g.wait)
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			return false
		case <-ctx.Done():
			timer.Stop()
			return false
		}
	}
}

func (g *reloadGate) leave() { g.inflight.Done() }

// begin holds the new requests back and waits for those in flight.
func (g *reloadGate) begin() {
	g.mu.Lock()
	g.reloading = make(chan struct{})
	g.mu.Unlock()
	g.inflight.Wait()
}

func (g *reloadGate) end() {
	g.mu.Lock()
	close(g.reloading)
	g.reloading = nil
	g.mu.Unlock()
}

func (g *reloadGate) handler(next func() http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.enter(r.Context()) {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"data":null,"error":{"status":503,"name":"ServiceUnavailableError","message":"The server is reloading","details":{}}}`))
			return
		}
		defer g.leave()
		next().ServeHTTP(w, r)
	})
}
