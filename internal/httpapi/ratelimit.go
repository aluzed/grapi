package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/ratelimit"
)

// RateLimit is users-permissions' ratelimit config (koa2-ratelimit).
type RateLimit = ratelimit.Config

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
	key := prefixKey(r.URL.Path, s.cfg.Proxy.IP(r), body)
	if s.limiter.Allow(w, key, cfg, time.Now()) {
		return false
	}
	writeError(w, errRateLimit, s.log)
	return true
}
