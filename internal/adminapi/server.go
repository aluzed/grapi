// Package adminapi is grapi's admin API, /admin/api/v1: its own versioned
// contract (docs/admin-api.md), on the admin tables of a Strapi database.
package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/auth/session"
	"github.com/aluzed/grapi/internal/clientip"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/plugins/email"
	"github.com/aluzed/grapi/internal/ratelimit"
	"github.com/aluzed/grapi/internal/webhooks"
	"github.com/aluzed/grapi/schema"
)

// Version is the path segment of this API.
const Version = "v1"

// Cookie is admin.auth.cookie.
type Cookie struct {
	// Secure defaults to true behind HTTPS (or a proxy saying so).
	Secure   *bool
	SameSite string
	Domain   string
}

// Config of the admin.
type Config struct {
	// Path is admin.path, "/admin" by default.
	Path     string
	Sessions session.Config
	Cookie   Cookie
	// RateLimit is admin.rateLimit, on login and register-admin.
	RateLimit ratelimit.Config
	// Proxy is server.proxy: the client IP, and X-Forwarded-Proto telling
	// HTTPS when trusted.
	Proxy clientip.Proxy
	// AdminURL is admin.absoluteUrl, used in the e-mails.
	AdminURL string
	Email    *email.Service
	// EmailProvider is plugins.email.config.provider (sendmail by default).
	EmailProvider string
	// ForgotPasswordFrom is admin.forgotPassword.from.
	ForgotPasswordFrom string
	// ResetPasswordTTL is admin.forgotPassword.expiresIn, 1 hour by default.
	ResetPasswordTTL time.Duration
	// APITokenSalt is admin.apiToken.salt, EncryptionKey
	// admin.secrets.encryptionKey (stored keys can then be shown again).
	APITokenSalt, EncryptionKey string
	// ContentActions lists the actions a custom API token can be given.
	ContentActions func() []string
	// Routes lists the content API routes, for the users-permissions editor.
	Routes func() []httpapi.RouteInfo
	// Webhooks stores and sends the webhooks; nil leaves their routes out.
	Webhooks *webhooks.Runner
	// Media is the upload pipeline; nil leaves the media routes out.
	Media MediaService
	// UI is the built SPA; nil serves the API only.
	UI fs.FS
	// AuditEnabled records the audit log (admin.auditLogs.enabled, true by
	// default); AuditRetentionDays purges older rows (0: keep).
	AuditEnabled       bool
	AuditRetentionDays int
	// HistoryEnabled records the content history (admin.history.enabled,
	// true by default); HistoryRetentionDays purges older versions (0: keep).
	HistoryEnabled       bool
	HistoryRetentionDays int
	// HistoryMaxVersions keeps that many versions per document and locale
	// (0: no limit).
	HistoryMaxVersions int
	// AuditExportMaxRows caps a CSV export (1 000 000 by default).
	AuditExportMaxRows int
	// UploadFromURLMaxSize, UploadFromURLTimeout bound a download of
	// upload-from-urls (200 MB, 60 s by default); UploadFromURLPrivate
	// lets it reach private addresses.
	UploadFromURLMaxSize int64
	UploadFromURLTimeout time.Duration
	UploadFromURLPrivate bool
	// HistoryOrigins are the origins whose writes record a version
	// (admin-panel only when empty).
	HistoryOrigins []string
	// Develop is `grapi develop`: the Content-Type Builder writes the
	// schemas of ProjectDir, and the reload picks them up.
	Develop    bool
	ProjectDir string
	// Version is grapi's version, Environment the project's (NODE_ENV).
	Version     string
	Environment string
	// Preview is admin.preview.
	Preview Preview
}

// Server serves the admin.
type Server struct {
	cfg        Config
	limiter    ratelimit.Limiter
	docs       *document.Service
	store      store.Store
	e          *query.Engine
	set        *schema.Set
	log        *slog.Logger
	sessions   *session.Manager
	actions    *permissions.Actions
	conditions *permissions.Conditions
	mux        *http.ServeMux
	now        func() time.Time
	wake       chan struct{}
	// historyPurgedAt is only touched by the scheduler goroutine.
	historyPurgedAt  time.Time
	auditPurgedAt    time.Time
	sessionsPurgedAt time.Time
}

// New builds the admin server on the Document Service of the content API.
func New(cfg Config, docs *document.Service, log *slog.Logger) *Server {
	e, set := docs.E, docs.Set
	if cfg.Path == "" {
		cfg.Path = "/admin"
	}
	cfg.Path = "/" + strings.Trim(cfg.Path, "/")
	if cfg.ResetPasswordTTL <= 0 {
		cfg.ResetPasswordTTL = time.Hour
	}
	if cfg.AuditExportMaxRows <= 0 {
		cfg.AuditExportMaxRows = 1_000_000
	}
	if cfg.UploadFromURLMaxSize <= 0 {
		cfg.UploadFromURLMaxSize = 200 << 20
	}
	if cfg.UploadFromURLTimeout <= 0 {
		cfg.UploadFromURLTimeout = time.Minute
	}
	if log == nil {
		log = slog.Default()
	}
	s := &Server{cfg: cfg, docs: docs, store: store.Store{DB: e.DB}, e: e, set: set, log: log, actions: &permissions.Actions{}, conditions: permissions.NewConditions(), now: time.Now, wake: make(chan struct{}, 1)}
	s.sessions = session.New(session.DB{E: e}, "admin", cfg.Sessions)
	s.registerActions()
	s.routes()
	return s
}

// Path is the mount path of the admin.
func (s *Server) Path() string { return s.cfg.Path }

// Actions is the action registry.
func (s *Server) Actions() *permissions.Actions { return s.actions }

// Conditions is the condition registry.
func (s *Server) Conditions() *permissions.Conditions { return s.conditions }

// Handles reports whether path belongs to the admin.
func (s *Server) Handles(path string) bool {
	return path == s.cfg.Path || strings.HasPrefix(path, s.cfg.Path+"/")
}

func (s *Server) api(p string) string { return s.cfg.Path + "/api/" + Version + p }

type handler func(w http.ResponseWriter, r *http.Request) error

type authedHandler func(w http.ResponseWriter, r *http.Request, u *User) error

func (s *Server) handle(pattern string, h handler) {
	method, p, _ := strings.Cut(pattern, " ")
	s.mux.HandleFunc(method+" "+s.api(p), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if err := h(w, r); err != nil {
			s.writeError(w, err)
		}
	})
}

func (s *Server) authed(pattern string, h authedHandler) {
	s.handle(pattern, func(w http.ResponseWriter, r *http.Request) error {
		u, err := s.authenticate(r)
		if err != nil {
			return err
		}
		return h(w, r, u)
	})
}

func (s *Server) routes() {
	s.mux = http.NewServeMux()
	s.authRoutes()
	s.meRoutes()
	s.adminRoutes()
	s.roleRoutes()
	s.contentManagerRoutes()
	s.contentManagerEditRoutes()
	s.relationRoutes()
	s.singleTypeRoutes()
	s.componentRoutes()
	s.mediaRoutes()
	s.mediaBulkRoutes()
	s.uidRoutes()
	s.homepageRoutes()
	s.i18nRoutes()
	s.tokenRoutes()
	s.webhookRoutes()
	s.usersPermissionsRoutes()
	s.emailRoutes()
	s.ctbRoutes()
	s.viewConfigRoutes()
	s.historyRoutes()
	s.auditRoutes()
	s.releaseRoutes()
	s.mediaSettingsRoutes()
	s.previewRoutes()
	s.projectRoutes()
	s.mux.HandleFunc(s.cfg.Path+"/api/", func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, errNotFound)
	})
	s.mux.HandleFunc(s.cfg.Path+"/", s.serveUI)
	s.mux.HandleFunc(s.cfg.Path, s.serveUI)
}

// Bootstrap seeds the default roles of an empty admin and creates the
// history table.
func (s *Server) Bootstrap(ctx context.Context) error {
	if err := s.ensureHistoryTable(ctx); err != nil {
		return err
	}
	if err := s.ensureAuditTable(ctx); err != nil {
		return err
	}
	if err := s.ensureReleaseTables(ctx); err != nil {
		return err
	}
	return s.seedRoles(ctx)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer httpapi.Recover(w, r, s.log)
	s.mux.ServeHTTP(w, r.WithContext(document.WithOrigin(r.Context(), "admin-panel")))
}

// Error is an API error.
type Error struct {
	Status  int
	Name    string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Message }

var (
	errNotFound     = &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Not Found"}
	errUnauthorized = &Error{Status: http.StatusUnauthorized, Name: "UnauthorizedError", Message: "Missing or invalid credentials"}
)

func appError(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Name: "ApplicationError", Message: msg}
}

// FieldError is one entry of a validation error's details.
type FieldError struct {
	Path    []string `json:"path"`
	Message string   `json:"message"`
}

func required(path ...string) FieldError {
	return FieldError{Path: path, Message: path[len(path)-1] + " is a required field"}
}

func validation(errs []FieldError) *Error {
	msg := errs[0].Message
	if len(errs) > 1 {
		msg = "Validation failed"
	}
	return &Error{Status: http.StatusBadRequest, Name: "ValidationError", Message: msg, Details: map[string]any{"errors": errs}}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func data(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusOK, map[string]any{"data": v})
	return nil
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	var e *Error
	if !errors.As(err, &e) {
		s.log.Error("admin api", "error", err)
		e = &Error{Status: http.StatusInternalServerError, Name: "InternalServerError", Message: "Internal Server Error"}
	}
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	writeJSON(w, e.Status, map[string]any{"data": nil, "error": map[string]any{"status": e.Status, "name": e.Name, "message": e.Message, "details": details}})
}

const maxBody = 1 << 20

func decode(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte("{}")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return &Error{Status: http.StatusBadRequest, Name: "ValidationError", Message: "Invalid JSON body"}
	}
	return nil
}
