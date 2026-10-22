package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimit is users-permissions' ratelimit config (koa2-ratelimit).
type RateLimit struct {
	Disabled bool
	Interval time.Duration
	Max      int
}

type window struct {
	count int
	reset time.Time
}

type limiter struct {
	mu   sync.Mutex
	hits map[string]*window
	last time.Time
}

// Routes whose body has no email identifier: keying them on body.email
// would let a caller get a fresh bucket per request.
var routesWithoutIdentifier = []string{"/auth/local", "/auth/reset-password", "/auth/change-password"}

func rateLimitPath(p string) string {
	n := strings.ToLower(path.Clean(p))
	n = strings.TrimRight(n, "/")
	if n == "" {
		return "/"
	}
	return n
}

// prefixKey is buildPrefixKey: <email|noIdentifier>:<path>:<ip>.
func prefixKey(p, ip string, body []byte) string {
	rp := rateLimitPath(p)
	usesEmail := !strings.Contains(rp, "/connect/")
	for _, r := range routesWithoutIdentifier {
		if strings.HasSuffix(rp, r) {
			usesEmail = false
		}
	}
	if !usesEmail {
		return "noIdentifier:" + rp + ":" + ip
	}
	var b struct {
		Email any `json:"email"`
	}
	_ = json.Unmarshal(body, &b)
	email, _ := b.Email.(string)
	if email == "" {
		return "unknownIdentifier:" + rp + ":" + ip
	}
	return strings.ToLower(email) + ":" + rp + ":" + ip
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allow counts a hit and sets the X-RateLimit headers; false past max.
func (l *limiter) allow(w http.ResponseWriter, key string, cfg RateLimit, now time.Time) bool {
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
	remaining := cfg.Max - win.count
	if remaining < 0 {
		remaining = 0
	}
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

var errRateLimit = newError(http.StatusTooManyRequests, "RateLimitError", "Too many requests, please try again later.")

// rateLimited runs the users-permissions rateLimit route middleware.
func (s *Server) rateLimited(w http.ResponseWriter, r *http.Request) bool {
	cfg := s.cfg.Users.RateLimit
	if cfg.Disabled {
		return false
	}
	if cfg.Interval == 0 {
		cfg.Interval = time.Minute
	}
	if cfg.Max == 0 {
		cfg.Max = 10
	}
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(r.Body, maxBodyRead))
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	key := prefixKey(r.URL.Path, clientIP(r, s.cfg.TrustProxy), body)
	if s.limiter.allow(w, key, cfg, time.Now()) {
		return false
	}
	writeError(w, errRateLimit, s.log)
	return true
}
