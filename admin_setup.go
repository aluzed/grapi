package grapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aluzed/grapi/admin"
	"github.com/aluzed/grapi/internal/adminapi"
	"github.com/aluzed/grapi/internal/auth/users"
	"github.com/aluzed/grapi/internal/clientip"
	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/plugins/email"
	"github.com/aluzed/grapi/internal/ratelimit"
)

func seconds(c *config.Config, key string) time.Duration {
	n, _ := c.Int(key)
	return time.Duration(n) * time.Second
}

// adminConfig reads admin.*; a missing admin.auth.secret is generated in
// .env under ADMIN_JWT_SECRET.
func adminConfig(c *config.Config, dir string, mail *email.Service, proxy clientip.Proxy, log *slog.Logger) (adminapi.Config, error) {
	cfg := adminapi.Config{Proxy: proxy, Email: mail}
	cfg.RateLimit = rateLimitConfig(c, "admin.rateLimit", ratelimit.Config{Interval: 5 * time.Minute, Max: 5})
	cfg.Path, _ = c.String("admin.url")
	if strings.HasPrefix(cfg.Path, "http") || cfg.Path == "" {
		cfg.Path = "/admin"
	}
	_, cfg.AdminURL = absoluteURLs(c)
	secret := c.StringOrEnv("admin.auth.secret", "ADMIN_JWT_SECRET")
	if secret == "" {
		var err error
		if secret, err = generateSecret(c, dir, "ADMIN_JWT_SECRET"); err != nil {
			return cfg, err
		}
		log.Info("the admin JWT secret has been generated in the .env file under ADMIN_JWT_SECRET")
	}
	sp := "admin.auth.sessions."
	cfg.Sessions.Secret = secret
	cfg.Preview = previewConfig(c, secret)
	cfg.Sessions.AccessTokenLifespan = seconds(c, sp+"accessTokenLifespan")
	cfg.Sessions.MaxRefreshTokenLifespan = seconds(c, sp+"maxRefreshTokenLifespan")
	cfg.Sessions.IdleRefreshTokenLifespan = seconds(c, sp+"idleRefreshTokenLifespan")
	cfg.Sessions.MaxSessionLifespan = seconds(c, sp+"maxSessionLifespan")
	cfg.Sessions.IdleSessionLifespan = seconds(c, sp+"idleSessionLifespan")
	if b, ok := c.Bool("admin.auth.cookie.secure"); ok {
		cfg.Cookie.Secure = &b
	}
	cfg.Cookie.SameSite, _ = c.String("admin.auth.cookie.sameSite")
	cfg.Cookie.Domain, _ = c.String("admin.auth.cookie.domain")
	cfg.ForgotPasswordFrom, _ = c.String("admin.forgotPassword.from")
	cfg.HistoryEnabled, cfg.HistoryRetentionDays = true, 90
	cfg.AuditEnabled, cfg.AuditRetentionDays = true, 90
	if b, ok := c.Bool("admin.auditLogs.enabled"); ok {
		cfg.AuditEnabled = b
	}
	if n, ok := c.Int("admin.auditLogs.retentionDays"); ok {
		cfg.AuditRetentionDays = n
	}
	if n, ok := c.Int("admin.auditLogs.exportMaxRows"); ok {
		cfg.AuditExportMaxRows = n
	}
	if b, ok := c.Bool("plugins.upload.config.uploadFromUrls.allowPrivateNetworks"); ok {
		cfg.UploadFromURLPrivate = b
	}
	if n, ok := c.Int("plugins.upload.config.uploadFromUrls.maxSize"); ok {
		cfg.UploadFromURLMaxSize = int64(n)
	}
	if b, ok := c.Bool("admin.history.enabled"); ok {
		cfg.HistoryEnabled = b
	}
	if n, ok := c.Int("admin.history.retentionDays"); ok {
		cfg.HistoryRetentionDays = n
	}
	if n, ok := c.Int("admin.history.maxVersionsPerDocument"); ok {
		cfg.HistoryMaxVersions = n
	}
	if list, ok := c.Get("admin.history.origins").([]any); ok {
		for _, o := range list {
			if o, ok := o.(string); ok {
				cfg.HistoryOrigins = append(cfg.HistoryOrigins, o)
			}
		}
	}
	if v := c.Get("admin.forgotPassword.expiresIn"); v != nil {
		d, err := durationOf(v)
		if err != nil {
			return cfg, fmt.Errorf("admin.forgotPassword.expiresIn: %w", err)
		}
		cfg.ResetPasswordTTL = d
	}
	if serve, ok := c.Bool("admin.serveAdminPanel"); !ok || serve {
		cfg.UI = admin.FS
	}
	return cfg, nil
}

func durationOf(v any) (time.Duration, error) {
	switch x := v.(type) {
	case int:
		return time.Duration(x) * time.Second, nil
	case float64:
		return time.Duration(x * float64(time.Second)), nil
	case string:
		return users.ParseDuration(x)
	}
	return 0, fmt.Errorf("invalid duration %v", v)
}

// withAdmin routes the admin paths to the admin server.
// previewConfig reads admin.preview: enabled, allowedOrigins, and the URL
// templates by content type (handlers).
func previewConfig(c *config.Config, secret string) adminapi.Preview {
	p := adminapi.Preview{Secret: secret, Handlers: map[string]string{}}
	p.Enabled, _ = c.Get("admin.preview.enabled").(bool)
	if list, ok := c.Get("admin.preview.allowedOrigins").([]any); ok {
		for _, o := range list {
			if s, ok := o.(string); ok && s != "" {
				p.AllowedOrigins = append(p.AllowedOrigins, strings.TrimRight(s, "/"))
			}
		}
	}
	if m, ok := c.Get("admin.preview.handlers").(map[string]any); ok {
		for uid, v := range m {
			if s, ok := v.(string); ok {
				p.Handlers[uid] = s
			}
		}
	}
	return p
}

// withFrameSrc lets the admin frame the preview origins: frame-src is set
// in the CSP that strapi::security wrote.
func withFrameSrc(h http.Header, sources []string) {
	csp := h.Get("Content-Security-Policy")
	if csp == "" || len(sources) == 0 {
		return
	}
	var parts []string
	for _, d := range strings.Split(csp, ";") {
		if !strings.HasPrefix(strings.TrimSpace(d), "frame-src ") {
			parts = append(parts, d)
		}
	}
	parts = append(parts, "frame-src "+strings.Join(sources, " "))
	h.Set("Content-Security-Policy", strings.Join(parts, ";"))
}

func withAdmin(adm *adminapi.Server, api *httpapi.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if adm.Handles(r.URL.Path) {
			api.SecurityHeaders(w.Header())
			withFrameSrc(w.Header(), adm.FrameSources())
			adm.ServeHTTP(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}
