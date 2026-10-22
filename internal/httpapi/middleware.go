package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Middlewares is config/middlewares: the global middlewares, by name, with
// their config. grapi implements strapi::security and strapi::cors; the
// others are built in (errors, query, body, public) or not needed.
type Middlewares struct {
	// Security and CORS are nil when the project leaves the middleware out.
	Security *SecurityConfig
	CORS     *CORSConfig
	// Body is strapi::body, which is always on (its limits).
	Body *BodyLimits
}

// SecurityConfig is the part of strapi::security (koa-helmet) grapi reads.
type SecurityConfig struct {
	// Directives override or add Content-Security-Policy directives.
	Directives map[string][]string
	// Disabled lists the headers turned off (contentSecurityPolicy: false...).
	Disabled map[string]bool
}

// CORSConfig is strapi::cors.
type CORSConfig struct {
	Origin      []string
	Expose      []string
	MaxAge      int
	Credentials bool
	Methods     []string
	Headers     []string
}

// DefaultMiddlewares is the stack of a generated Strapi project.
func DefaultMiddlewares() Middlewares {
	return Middlewares{Security: &SecurityConfig{}, CORS: DefaultCORS()}
}

// DefaultCORS is strapi::cors without config.
func DefaultCORS() *CORSConfig {
	return &CORSConfig{
		Origin:      []string{"*"},
		MaxAge:      31536000,
		Credentials: true,
		Methods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		Headers:     []string{"Content-Type", "Authorization", "Origin", "Accept"},
	}
}

var cspDefaults = []struct {
	name   string
	values []string
}{
	{"connect-src", []string{"'self'", "https:"}},
	{"img-src", []string{"'self'", "data:", "blob:", "https://market-assets.strapi.io"}},
	{"media-src", []string{"'self'", "data:", "blob:"}},
	{"default-src", []string{"'self'"}},
	{"base-uri", []string{"'self'"}},
	{"font-src", []string{"'self'", "https:", "data:"}},
	{"form-action", []string{"'self'"}},
	{"frame-ancestors", []string{"'self'"}},
	{"object-src", []string{"'none'"}},
	{"script-src", []string{"'self'"}},
	{"script-src-attr", []string{"'none'"}},
	{"style-src", []string{"'self'", "https:", "'unsafe-inline'"}},
}

func (c *SecurityConfig) csp() string {
	seen := map[string]bool{}
	var parts []string
	for _, d := range cspDefaults {
		values := d.values
		if v, ok := c.Directives[d.name]; ok {
			values = v
		}
		seen[d.name] = true
		if values != nil {
			parts = append(parts, d.name+" "+strings.Join(values, " "))
		}
	}
	var extra []string
	for name := range c.Directives {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		if v := c.Directives[name]; v != nil {
			parts = append(parts, strings.TrimSpace(name+" "+strings.Join(v, " ")))
		}
	}
	return strings.Join(parts, ";")
}

// headers are koa-helmet's, in the order Strapi 5.59 sends them.
func (c *SecurityConfig) headers() [][2]string {
	all := [][2]string{
		{"Content-Security-Policy", c.csp()},
		{"Referrer-Policy", "no-referrer"},
		{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-DNS-Prefetch-Control", "off"},
		{"X-Download-Options", "noopen"},
		{"X-Frame-Options", "SAMEORIGIN"},
		{"X-Permitted-Cross-Domain-Policies", "none"},
	}
	out := all[:0]
	for _, h := range all {
		if !c.Disabled[h[0]] {
			out = append(out, h)
		}
	}
	return out
}

// matchOrigin is strapi::cors' matchOrigin.
func (c *CORSConfig) matchOrigin(requestOrigin string) string {
	if requestOrigin == "" {
		return "*"
	}
	for _, o := range c.Origin {
		if o == "*" {
			return requestOrigin
		}
	}
	for _, o := range c.Origin {
		if o == requestOrigin {
			return requestOrigin
		}
	}
	return ""
}

// apply is @koa/cors; it returns true when it answered a preflight.
func (c *CORSConfig) apply(w http.ResponseWriter, r *http.Request) bool {
	h := w.Header()
	h.Add("Vary", "Origin")
	origin := c.matchOrigin(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	if c.Credentials && origin == "*" {
		origin = r.Header.Get("Origin")
	}
	if r.Method != http.MethodOptions {
		h.Set("Access-Control-Allow-Origin", origin)
		if c.Credentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}
		if len(c.Expose) > 0 {
			h.Set("Access-Control-Expose-Headers", strings.Join(c.Expose, ","))
		}
		return false
	}
	if r.Header.Get("Access-Control-Request-Method") == "" {
		return false
	}
	h.Set("Access-Control-Allow-Origin", origin)
	if c.Credentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
	if c.MaxAge > 0 {
		h.Set("Access-Control-Max-Age", strconv.Itoa(c.MaxAge))
	}
	if len(c.Methods) > 0 {
		h.Set("Access-Control-Allow-Methods", strings.Join(c.Methods, ","))
	}
	allow := strings.Join(c.Headers, ",")
	if allow == "" {
		allow = r.Header.Get("Access-Control-Request-Headers")
	}
	if allow != "" {
		h.Set("Access-Control-Allow-Headers", allow)
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

// wrap wraps next with the security and CORS middlewares.
func (m Middlewares) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.Security != nil {
			for _, h := range m.Security.headers() {
				w.Header()[h[0]] = []string{h[1]}
			}
		}
		if m.CORS != nil && m.CORS.apply(w, r) {
			return
		}
		limits := DefaultBodyLimits
		if m.Body != nil {
			limits = *m.Body
		}
		if limits.limitBody(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ParseMiddlewares reads config/middlewares: names, or {name, config}.
func ParseMiddlewares(list []any) (Middlewares, error) {
	var m Middlewares
	for i, entry := range list {
		name, cfg := "", map[string]any{}
		switch e := entry.(type) {
		case string:
			name = e
		case map[string]any:
			name, _ = e["name"].(string)
			if c, ok := e["config"].(map[string]any); ok {
				cfg = c
			}
		default:
			return m, fmt.Errorf("middlewares[%d]: expected a name or {name, config}", i)
		}
		switch name {
		case "strapi::security":
			m.Security = parseSecurity(cfg)
		case "strapi::cors":
			m.CORS = parseCORS(cfg)
		case "strapi::body":
			m.Body = parseBodyLimits(cfg)
		}
	}
	return m, nil
}

func toStrings(v any) []string {
	switch x := v.(type) {
	case string:
		var out []string
		for _, p := range strings.Split(x, ",") {
			out = append(out, strings.TrimSpace(p))
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	}
	return nil
}

func parseSecurity(cfg map[string]any) *SecurityConfig {
	sc := &SecurityConfig{Directives: map[string][]string{}, Disabled: map[string]bool{}}
	switch csp := cfg["contentSecurityPolicy"].(type) {
	case bool:
		if !csp {
			sc.Disabled["Content-Security-Policy"] = true
		}
	case map[string]any:
		if d, ok := csp["directives"].(map[string]any); ok {
			for name, v := range d {
				if v == nil {
					sc.Directives[name] = nil
					continue
				}
				sc.Directives[name] = toStrings(v)
			}
		}
	}
	for key, header := range map[string]string{
		"referrerPolicy": "Referrer-Policy", "hsts": "Strict-Transport-Security", "xContentTypeOptions": "X-Content-Type-Options",
		"xDnsPrefetchControl": "X-DNS-Prefetch-Control", "xDownloadOptions": "X-Download-Options", "xFrameOptions": "X-Frame-Options",
		"frameguard": "X-Frame-Options", "xPermittedCrossDomainPolicies": "X-Permitted-Cross-Domain-Policies",
	} {
		if v, ok := cfg[key].(bool); ok && !v {
			sc.Disabled[header] = true
		}
	}
	return sc
}

func parseCORS(cfg map[string]any) *CORSConfig {
	c := DefaultCORS()
	if v, ok := cfg["origin"]; ok {
		c.Origin = toStrings(v)
	}
	if v, ok := cfg["expose"]; ok {
		c.Expose = toStrings(v)
	}
	if v, ok := cfg["maxAge"].(int); ok {
		c.MaxAge = v
	}
	if v, ok := cfg["credentials"].(bool); ok {
		c.Credentials = v
	}
	if v, ok := cfg["methods"]; ok {
		c.Methods = toStrings(v)
	}
	if v, ok := cfg["headers"]; ok {
		c.Headers = toStrings(v)
	}
	return c
}
