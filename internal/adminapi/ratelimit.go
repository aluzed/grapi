package adminapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

var errRateLimit = &Error{Status: http.StatusTooManyRequests, Name: "RateLimitError", Message: "Too many requests, please try again later."}

// rateLimited is the admin rateLimit middleware: key
// <email|unknownEmail>:<path>:<ip>, 5 requests per 5 minutes by default.
func (s *Server) rateLimited(h handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		cfg := s.cfg.RateLimit
		if cfg.Disabled || cfg.Max <= 0 || cfg.Interval <= 0 {
			return h(w, r)
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return err
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var b struct {
			Email any `json:"email"`
		}
		_ = json.Unmarshal(body, &b)
		email := "unknownEmail"
		if e, ok := b.Email.(string); ok {
			email = normalizeEmail(e)
		}
		p := strings.TrimSuffix(strings.ToLower(path.Clean(r.URL.Path)), "/")
		if !s.limiter.Allow(w, email+":"+p+":"+s.cfg.Proxy.IP(r), cfg, time.Now()) {
			return errRateLimit
		}
		return h(w, r)
	}
}
