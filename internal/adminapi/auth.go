package adminapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/session"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/plugins/email"
)

// RefreshCookie holds the refresh token, for the auth routes only.
const RefreshCookie = "grapi_admin_refresh"

func (s *Server) authRoutes() {
	s.handle("GET /auth/init", s.initInfo)
	s.handle("POST /auth/register-admin", s.registerAdmin)
	s.handle("POST /auth/login", s.login)
	s.handle("POST /auth/access-token", s.accessToken)
	s.handle("POST /auth/logout", s.logout)
	s.handle("POST /auth/forgot-password", s.forgotPassword)
	s.handle("POST /auth/reset-password", s.resetPassword)
	s.handle("GET /auth/registration-info", s.registrationInfo)
	s.handle("POST /auth/register", s.register)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	scheme, tok, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(tok)
}

func (s *Server) authenticate(r *http.Request) (*User, error) {
	tok := bearer(r)
	if tok == "" {
		return nil, errUnauthorized
	}
	userID, sessionID, err := s.sessions.ValidateAccess(r.Context(), tok)
	if errors.Is(err, session.ErrInvalidToken) {
		return nil, errUnauthorized
	}
	if err != nil {
		return nil, err
	}
	u, err := s.findUser(r.Context(), map[string]any{"id": userID})
	if err != nil {
		return nil, err
	}
	if u == nil || !u.IsActive || u.Blocked {
		return nil, errUnauthorized
	}
	u.sessionID = sessionID
	return u, nil
}

func (s *Server) secure(r *http.Request) bool {
	if s.cfg.Cookie.Secure != nil {
		return *s.cfg.Cookie.Secure
	}
	if r.TLS != nil {
		return true
	}
	return s.cfg.Proxy.Koa && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) cookie(r *http.Request, value string, expires time.Time) *http.Cookie {
	c := &http.Cookie{Name: RefreshCookie, //nolint:gosec // Secure follows admin.auth.cookie.secure and the request scheme
		Value: value, Path: s.api("/auth"), Domain: s.cfg.Cookie.Domain, HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode}
	switch strings.ToLower(s.cfg.Cookie.SameSite) {
	case "strict":
		c.SameSite = http.SameSiteStrictMode
	case "none":
		c.SameSite = http.SameSiteNoneMode
	}
	if !expires.IsZero() {
		c.Expires = expires
		c.MaxAge = int(time.Until(expires).Seconds())
	}
	return c
}

// setRefresh writes the refresh cookie: persistent for a remembered session,
// a browser-session cookie otherwise.
func (s *Server) setRefresh(w http.ResponseWriter, r *http.Request, tok string, sess *session.Session) {
	var expires time.Time
	if sess.Type == session.TypeRefresh {
		expires = sess.ExpiresAt
	}
	http.SetCookie(w, s.cookie(r, tok, expires))
}

func (s *Server) clearRefresh(w http.ResponseWriter, r *http.Request) {
	c := s.cookie(r, "", time.Time{}) //nolint:gosec // same attributes as the cookie it clears
	c.MaxAge = -1
	http.SetCookie(w, c)
}

func (s *Server) initInfo(w http.ResponseWriter, r *http.Request) error {
	n, err := s.countUsers(r.Context())
	if err != nil {
		return err
	}
	return data(w, map[string]any{"hasAdmin": n > 0})
}

type loginBody struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
	DeviceID   string `json:"deviceId"`
}

// startSession logs u in and answers the access token and the user.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *User, remember bool, deviceID string) error {
	typ := session.TypeSession
	if remember {
		typ = session.TypeRefresh
	}
	meta := map[string]any{"userAgent": r.UserAgent(), "loginAt": s.now().UTC().Format(time.RFC3339)}
	refresh, sess, err := s.sessions.Login(r.Context(), fmt.Sprint(u.ID), deviceID, typ, meta)
	if err != nil {
		return err
	}
	access, err := s.sessions.AccessToken(r.Context(), refresh)
	if err != nil {
		return err
	}
	s.setRefresh(w, r, refresh, sess)
	return data(w, map[string]any{"token": access, "user": u})
}

type registerAdminBody struct {
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

func (b *registerAdminBody) validate() []FieldError {
	var errs []FieldError
	b.Email = normalizeEmail(b.Email)
	if strings.TrimSpace(b.Firstname) == "" {
		errs = append(errs, FieldError{Path: []string{"firstname"}, Message: "firstname is a required field"})
	}
	errs = append(errs, checkEmail("email", b.Email)...)
	return append(errs, checkPassword("password", b.Password)...)
}

func (s *Server) registerAdmin(w http.ResponseWriter, r *http.Request) error {
	var b registerAdminBody
	if err := decode(r, &b); err != nil {
		return err
	}
	if errs := b.validate(); len(errs) > 0 {
		return validation(errs)
	}
	ctx := r.Context()
	n, err := s.countUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return appError("You cannot register a new super admin")
	}
	super, err := s.e.FindOne(ctx, roleUID, query.Params{Where: map[string]any{"code": SuperAdminCode}})
	if err != nil {
		return err
	}
	if super == nil {
		return appError("Cannot register the first admin because the super admin role does not exist")
	}
	hash, err := hashPassword(b.Password)
	if err != nil {
		return err
	}
	row, err := s.e.Create(ctx, userUID, query.Data{
		"firstname": b.Firstname, "lastname": nullable(b.Lastname), "email": b.Email, "password": hash,
		"isActive": true, "blocked": false, "roles": []any{get(super, "id")},
	}, query.Params{})
	if err != nil {
		return err
	}
	u, err := s.findUser(ctx, map[string]any{"id": get(row, "id")})
	if err != nil {
		return err
	}
	s.audit(r, u, "user.create", map[string]any{"id": u.ID, "email": u.Email, "role": SuperAdminCode})
	return s.startSession(w, r, u, false, "")
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var b loginBody
	if err := decode(r, &b); err != nil {
		return err
	}
	if b.Email == "" || b.Password == "" {
		return validation([]FieldError{{Path: []string{"email"}, Message: "email and password are required"}})
	}
	u, err := s.findUser(r.Context(), map[string]any{"email": normalizeEmail(b.Email)})
	if err != nil {
		return err
	}
	invalid := &Error{Status: http.StatusBadRequest, Name: "ApplicationError", Message: "Invalid credentials"}
	if u == nil || !passwordMatches(b.Password, u.password) {
		s.audit(r, nil, "admin.auth.error", map[string]any{"email": normalizeEmail(b.Email)})
		return invalid
	}
	if !u.IsActive || u.Blocked {
		s.audit(r, u, "admin.auth.error", map[string]any{"email": u.Email, "reason": "inactive"})
		return appError("User not active")
	}
	s.audit(r, u, "admin.auth.success", nil)
	return s.startSession(w, r, u, b.RememberMe, b.DeviceID)
}

func (s *Server) refreshCookie(r *http.Request) string {
	c, err := r.Cookie(RefreshCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// accessToken rotates the refresh cookie and returns a new access token.
func (s *Server) accessToken(w http.ResponseWriter, r *http.Request) error {
	refresh := s.refreshCookie(r)
	if refresh == "" {
		return errUnauthorized
	}
	next, sess, err := s.sessions.Rotate(r.Context(), refresh)
	if errors.Is(err, session.ErrInvalidToken) {
		s.clearRefresh(w, r)
		return errUnauthorized
	}
	if err != nil {
		return err
	}
	u, err := s.findUser(r.Context(), map[string]any{"id": sess.UserID})
	if err != nil {
		return err
	}
	if u == nil || !u.IsActive || u.Blocked {
		_ = s.sessions.Logout(r.Context(), sess.UserID, sess.DeviceID)
		s.clearRefresh(w, r)
		return errUnauthorized
	}
	access, err := s.sessions.AccessToken(r.Context(), next)
	if err != nil {
		return err
	}
	s.setRefresh(w, r, next, sess)
	return data(w, map[string]any{"token": access})
}

// logout ends the session of the refresh cookie (its device family).
func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if refresh := s.refreshCookie(r); refresh != "" {
		if sess, err := s.sessionOf(r.Context(), refresh); err == nil && sess != nil {
			if err := s.sessions.Logout(r.Context(), sess.UserID, sess.DeviceID); err != nil {
				return err
			}
			if u, err := s.findUser(r.Context(), map[string]any{"id": sess.UserID}); err == nil && u != nil {
				s.audit(r, u, "admin.logout", nil)
			}
		}
	}
	s.clearRefresh(w, r)
	return data(w, map[string]any{})
}

func (s *Server) sessionOf(ctx context.Context, refresh string) (*session.Session, error) {
	_, sid, err := s.sessions.ParseRefresh(refresh)
	if err != nil {
		return nil, err
	}
	return s.sessions.Get(ctx, sid)
}

func token() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) error {
	var b struct {
		Email string `json:"email"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	if errs := checkEmail("email", normalizeEmail(b.Email)); len(errs) > 0 {
		return validation(errs)
	}
	ctx := r.Context()
	u, err := s.findUser(ctx, map[string]any{"email": normalizeEmail(b.Email), "isActive": true})
	if err != nil {
		return err
	}
	if u != nil {
		tok := token()
		if _, err := s.e.Update(ctx, userUID, map[string]any{"id": u.ID}, query.Data{"resetPasswordToken": tok, "resetPasswordTokenExpiresAt": s.now().Add(s.cfg.ResetPasswordTTL).UTC()}, query.Params{}); err != nil {
			return err
		}
		s.audit(r, u, "user.password-reset.create", map[string]any{"id": u.ID})
		link := strings.TrimRight(s.cfg.AdminURL, "/") + "/auth/reset-password?code=" + tok
		s.mail(ctx, u.Email, "Reset password", "We heard that you lost your password. Sorry about that!\n\nBut don't worry! You can use the following link to reset your password:\n"+link+"\n\nThanks.")
	}
	return data(w, map[string]any{})
}

// mail logs a failure without telling the caller, so that the forgot
// password answer never reveals whether the account exists.
func (s *Server) mail(ctx context.Context, to, subject, text string) {
	if s.cfg.Email == nil {
		s.log.Warn("admin e-mail not sent: no e-mail provider is configured", "subject", subject)
		return
	}
	if err := s.cfg.Email.Send(ctx, email.Message{To: to, From: s.cfg.ForgotPasswordFrom, Subject: subject, Text: text}); err != nil {
		s.log.Error("admin e-mail", "error", err)
	}
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) error {
	var b struct {
		ResetPasswordToken string `json:"resetPasswordToken"`
		Password           string `json:"password"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	if errs := checkPassword("password", b.Password); len(errs) > 0 {
		return validation(errs)
	}
	if b.ResetPasswordToken == "" {
		return appError("Invalid reset password token")
	}
	ctx := r.Context()
	row, err := s.e.FindOne(ctx, userUID, query.Params{Where: map[string]any{"resetPasswordToken": b.ResetPasswordToken, "isActive": true}})
	if err != nil {
		return err
	}
	if row == nil {
		return appError("Invalid reset password token")
	}
	id := get(row, "id")
	expires := session.Time(get(row, "resetPasswordTokenExpiresAt"))
	if expires.IsZero() || !expires.After(s.now()) {
		_, _ = s.e.Update(ctx, userUID, map[string]any{"id": id}, query.Data{"resetPasswordToken": nil, "resetPasswordTokenExpiresAt": nil}, query.Params{})
		return appError("This reset password token has expired")
	}
	hash, err := hashPassword(b.Password)
	if err != nil {
		return err
	}
	if _, err := s.e.Update(ctx, userUID, map[string]any{"id": id}, query.Data{"password": hash, "resetPasswordToken": nil, "resetPasswordTokenExpiresAt": nil}, query.Params{}); err != nil {
		return err
	}
	if err := s.sessions.Logout(ctx, fmt.Sprint(id), ""); err != nil {
		return err
	}
	u, err := s.findUser(ctx, map[string]any{"id": id})
	if err != nil {
		return err
	}
	s.audit(r, u, "user.password-reset.confirm", map[string]any{"id": u.ID})
	return s.startSession(w, r, u, false, "")
}

func (s *Server) registrationInfo(w http.ResponseWriter, r *http.Request) error {
	tok := r.URL.Query().Get("registrationToken")
	if tok == "" {
		return appError("Invalid registrationToken")
	}
	u, err := s.findUser(r.Context(), map[string]any{"registrationToken": tok})
	if err != nil {
		return err
	}
	if u == nil {
		return appError("Invalid registrationToken")
	}
	return data(w, map[string]any{"email": u.Email, "firstname": u.Firstname, "lastname": u.Lastname})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	var b struct {
		RegistrationToken string `json:"registrationToken"`
		UserInfo          struct {
			Firstname string `json:"firstname"`
			Lastname  string `json:"lastname"`
			Password  string `json:"password"`
		} `json:"userInfo"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	var errs []FieldError
	if strings.TrimSpace(b.UserInfo.Firstname) == "" {
		errs = append(errs, FieldError{Path: []string{"userInfo", "firstname"}, Message: "firstname is a required field"})
	}
	errs = append(errs, checkPassword("password", b.UserInfo.Password)...)
	if len(errs) > 0 {
		return validation(errs)
	}
	ctx := r.Context()
	u, err := s.findUser(ctx, map[string]any{"registrationToken": b.RegistrationToken})
	if err != nil {
		return err
	}
	if b.RegistrationToken == "" || u == nil {
		return appError("Invalid registrationToken")
	}
	hash, err := hashPassword(b.UserInfo.Password)
	if err != nil {
		return err
	}
	if _, err := s.e.Update(ctx, userUID, map[string]any{"id": u.ID}, query.Data{
		"firstname": b.UserInfo.Firstname, "lastname": nullable(b.UserInfo.Lastname),
		"password": hash, "registrationToken": nil, "isActive": true,
	}, query.Params{}); err != nil {
		return err
	}
	if u, err = s.findUser(ctx, map[string]any{"id": u.ID}); err != nil {
		return err
	}
	s.audit(r, u, "user.invite.accept", map[string]any{"id": u.ID})
	return s.startSession(w, r, u, false, "")
}
