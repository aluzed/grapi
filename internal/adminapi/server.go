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
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/plugins/email"
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
	// TrustProxy is server.proxy: X-Forwarded-Proto tells HTTPS.
	TrustProxy bool
	// AdminURL is admin.absoluteUrl, used in the e-mails.
	AdminURL string
	Email    *email.Service
	// ForgotPasswordFrom is admin.forgotPassword.from.
	ForgotPasswordFrom string
	// ResetPasswordTTL is admin.forgotPassword.expiresIn, 1 hour by default.
	ResetPasswordTTL time.Duration
	// UI is the built SPA; nil serves the API only.
	UI fs.FS
	// Develop enables the Content-Type Builder entry.
	Develop bool
	// Version is grapi's version, Environment the project's (NODE_ENV).
	Version     string
	Environment string
}

// Server serves the admin.
type Server struct {
	cfg        Config
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
	if log == nil {
		log = slog.Default()
	}
	s := &Server{cfg: cfg, docs: docs, store: store.Store{DB: e.DB}, e: e, set: set, log: log, actions: &permissions.Actions{}, conditions: permissions.NewConditions(), now: time.Now}
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
	s.mux.HandleFunc(s.cfg.Path+"/api/", func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, errNotFound)
	})
	s.mux.HandleFunc(s.cfg.Path+"/", s.serveUI)
	s.mux.HandleFunc(s.cfg.Path, s.serveUI)
}

// Bootstrap seeds the default roles of an empty admin.
func (s *Server) Bootstrap(ctx context.Context) error { return s.seedRoles(ctx) }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

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
