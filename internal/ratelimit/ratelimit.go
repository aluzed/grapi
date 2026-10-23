// Package ratelimit is koa2-ratelimit's fixed window, in memory, as
// Strapi's users-permissions and admin rateLimit middlewares use it.
package ratelimit

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Config is a rateLimit config: Max hits per Interval and key.
type Config struct {
	Disabled bool
	Interval time.Duration
	Max      int
}

type window struct {
	count int
	reset time.Time
}

// Limiter counts hits per key.
type Limiter struct {
	mu   sync.Mutex
	hits map[string]*window
	last time.Time
}

// Allow counts a hit on key and sets the X-RateLimit headers; false past
// cfg.Max, with Retry-After set.
func (l *Limiter) Allow(w http.ResponseWriter, key string, cfg Config, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string]*window{}
	}
	if now.Sub(l.last) > cfg.Interval {
		for k, v := range l.hits {
			if now.After(v.reset) {
				delete(l.hits, k)
			}
		}
		l.last = now
	}
	win := l.hits[key]
	if win == nil || now.After(win.reset) {
		win = &window{reset: now.Add(cfg.Interval)}
		l.hits[key] = win
	}
	win.count++
	remaining := max(cfg.Max-win.count, 0)
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(cfg.Max))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(win.reset.Unix(), 10))
	if win.count > cfg.Max {
		h.Set("Retry-After", strconv.Itoa(int(win.reset.Sub(now).Seconds()+0.999)))
		return false
	}
	return true
}
