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
	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/plugins/email"
)

func seconds(c *config.Config, key string) time.Duration {
	n, _ := c.Int(key)
	return time.Duration(n) * time.Second
}

// adminConfig reads admin.*; a missing admin.auth.secret is generated in
// .env under ADMIN_JWT_SECRET.
func adminConfig(c *config.Config, dir string, mail *email.Service, trustProxy bool, log *slog.Logger) (adminapi.Config, error) {
	cfg := adminapi.Config{TrustProxy: trustProxy, Email: mail}
	cfg.Path, _ = c.String("admin.url")
	if strings.HasPrefix(cfg.Path, "http") || cfg.Path == "" {
		cfg.Path = "/admin"
	}
	_, cfg.AdminURL = absoluteURLs(c)
	secret, _ := c.String("admin.auth.secret")
	if secret == "" {
		secret = c.Env["ADMIN_JWT_SECRET"]
	}
	if secret == "" {
		var err error
		if secret, err = generateSecret(c, dir, "ADMIN_JWT_SECRET"); err != nil {
			return cfg, err
		}
		log.Info("the admin JWT secret has been generated in the .env file under ADMIN_JWT_SECRET")
	}
	sp := "admin.auth.sessions."
	cfg.Sessions.Secret = secret
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
func withAdmin(adm *adminapi.Server, api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if adm.Handles(r.URL.Path) {
			adm.ServeHTTP(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}
