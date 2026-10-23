package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // keygrip, koa's cookie signature, is HMAC-SHA1
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/session"
	"github.com/aluzed/grapi/internal/db/query"
)

// UPSessions is plugins.users-permissions.config.sessions, used when
// jwtManagement is "refresh".
type UPSessions struct {
	AccessTokenLifespan      time.Duration
	MaxRefreshTokenLifespan  time.Duration
	IdleRefreshTokenLifespan time.Duration
	MaxSessionLifespan       time.Duration
	IdleSessionLifespan      time.Duration
	// HTTPOnly sends the refresh token in a cookie instead of the body.
	HTTPOnly bool
	Cookie   UPCookie
}

// UPCookie is sessions.cookie; Secure defaults to Production.
type UPCookie struct {
	Name, SameSite, Path, Domain string
	Secure                       *bool
	MaxAge                       time.Duration
}

func newUPSessions(e *query.Engine, cfg UsersConfig) *session.Manager {
	c := cfg.Sessions
	or := func(d, v time.Duration) time.Duration {
		if d > 0 {
			return d
		}
		return v
	}
	day := 24 * time.Hour
	return session.New(session.DB{E: e}, "users-permissions", session.Config{
		Secret: cfg.JWTSecret, Algorithm: cfg.JWTAlgorithm,
		AccessTokenLifespan:      or(c.AccessTokenLifespan, 10*time.Minute),
		MaxRefreshTokenLifespan:  or(c.MaxRefreshTokenLifespan, 30*day),
		IdleRefreshTokenLifespan: or(c.IdleRefreshTokenLifespan, 14*day),
		MaxSessionLifespan:       or(c.MaxSessionLifespan, day),
		IdleSessionLifespan:      or(c.IdleSessionLifespan, 2*time.Hour),
	})
}

func (s *Server) upCookieName() string {
	if n := s.cfg.Users.Sessions.Cookie.Name; n != "" {
		return n
	}
	return "strapi_up_refresh"
}

func (s *Server) refreshInCookie(r *http.Request) bool {
	return s.cfg.Users.Sessions.HTTPOnly || r.Header.Get("x-strapi-refresh-cookie") == "httpOnly"
}

// setRefreshCookie is buildRefreshCookieOptions; an empty value clears it.
func (s *Server) setRefreshCookie(w http.ResponseWriter, value string) {
	c := s.cfg.Users.Sessions.Cookie
	secure := s.cfg.Users.Production
	if c.Secure != nil {
		secure = *c.Secure
	}
	//nolint:gosec // Secure follows sessions.cookie.secure, else production, like Strapi: plain HTTP in development.
	ck := &http.Cookie{Name: s.upCookieName(), Value: value, HttpOnly: true, Secure: secure, Path: "/", Domain: c.Domain, SameSite: http.SameSiteLaxMode}
	if c.Path != "" {
		ck.Path = c.Path
	}
	switch strings.ToLower(c.SameSite) {
	case "strict":
		ck.SameSite = http.SameSiteStrictMode
	case "none":
		ck.SameSite = http.SameSiteNoneMode
	}
	switch {
	case value == "":
		ck.Expires = time.Unix(0, 0)
	case c.MaxAge > 0:
		ck.MaxAge = int(c.MaxAge / time.Second)
		ck.Expires = time.Now().Add(c.MaxAge)
	}
	http.SetCookie(w, ck)
	if keys := s.cfg.Users.AppKeys; len(keys) > 0 {
		sig := *ck //nolint:gosec // the same attributes as the cookie it signs
		sig.Name, sig.Value = ck.Name+".sig", keygripSign(keys[0], ck.Name+"="+value)
		http.SetCookie(w, &sig)
	}
}

// keygripSign is keygrip's sign, koa's cookie signature: HMAC-SHA1 in
// base64url without padding.
func keygripSign(key, data string) string {
	m := hmac.New(sha1.New, []byte(key))
	m.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func deviceIDOf(body map[string]any) string {
	d, _ := body["deviceId"].(string)
	return d
}

func sessionMetadata(r *http.Request) map[string]any {
	m := map[string]any{"loginAt": time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
	if name := session.DeviceName(r.UserAgent()); name != "" {
		m["deviceName"] = name
	}
	return m
}

// authIssue says how an auth answer gets its tokens in refresh mode.
type authIssue int

const (
	// issueLogin is a login: the refresh token may go in a cookie.
	issueLogin authIssue = iota
	// issueRegister always answers the refresh token in the body.
	issueRegister
	// issueReissue ends every session first (password change or reset).
	issueReissue
	// issueToken answers an access token only (email confirmation).
	issueToken
)

// refreshTokens opens a session and returns its refresh and access tokens.
func (s *Server) refreshTokens(ctx context.Context, userID, deviceID string, meta map[string]any) (string, string, error) {
	refresh, _, err := s.upSessions.Login(ctx, userID, deviceID, session.TypeRefresh, meta)
	if err != nil {
		return "", "", err
	}
	access, err := s.upSessions.AccessToken(ctx, refresh)
	if err != nil {
		return "", "", appError("Invalid credentials")
	}
	return refresh, access, nil
}

func (s *Server) sendRefreshAuth(w http.ResponseWriter, r *http.Request, user query.Row, auth *Auth, body map[string]any, how authIssue) error {
	ctx := r.Context()
	id, _ := user.Get("id")
	userID := fmt.Sprint(id)
	out := map[string]any{"user": s.sanitizeOutput(user, s.schemaOf(userUID), auth)}
	switch how {
	case issueToken:
		_, access, err := s.refreshTokens(ctx, userID, "", nil)
		if err != nil {
			return err
		}
		out["jwt"] = access
	case issueReissue:
		if err := s.upSessions.Logout(ctx, userID, ""); err != nil {
			return err
		}
		refresh, access, err := s.refreshTokens(ctx, userID, deviceIDOf(body), nil)
		if err != nil {
			return err
		}
		out["jwt"], out["refreshToken"] = access, refresh
	default:
		refresh, access, err := s.refreshTokens(ctx, userID, deviceIDOf(body), sessionMetadata(r))
		if err != nil {
			return err
		}
		out["jwt"] = access
		if how == issueLogin && s.refreshInCookie(r) {
			s.setRefreshCookie(w, refresh)
		} else {
			out["refreshToken"] = refresh
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) refreshTokenOf(r *http.Request, body map[string]any) string {
	if c, err := r.Cookie(s.upCookieName()); err == nil && c.Value != "" {
		return c.Value
	}
	t, _ := body["refreshToken"].(string)
	return t
}

func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	if s.upSessions == nil {
		return errNotFound
	}
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	token := s.refreshTokenOf(r, body)
	if token == "" {
		return newError(http.StatusBadRequest, "BadRequestError", "Missing refresh token")
	}
	invalid := newError(http.StatusUnauthorized, "UnauthorizedError", "Invalid refresh token")
	ctx := r.Context()
	next, _, err := s.upSessions.Rotate(ctx, token)
	if err != nil {
		return invalid
	}
	access, err := s.upSessions.AccessToken(ctx, next)
	if err != nil {
		return invalid
	}
	if s.refreshInCookie(r) {
		s.setRefreshCookie(w, next)
		writeJSON(w, http.StatusOK, map[string]any{"jwt": access})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"jwt": access, "refreshToken": next})
	return nil
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	if s.upSessions == nil {
		return errNotFound
	}
	if rc.auth.User == nil {
		return newError(http.StatusUnauthorized, "UnauthorizedError", "Missing authentication")
	}
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	userID := fmt.Sprint(rc.auth.userID())
	scope, _ := body["scope"].(string)
	current := rc.auth.SessionID
	if token := s.refreshTokenOf(r, body); token != "" {
		if _, sid, err := s.upSessions.ParseRefresh(token); err == nil {
			current = sid
		}
	}
	switch {
	case scope == "all":
		err = s.upSessions.Logout(ctx, userID, "")
	case deviceIDOf(body) != "":
		err = s.upSessions.Logout(ctx, userID, deviceIDOf(body))
	case current != "":
		_, err = s.upSessions.Revoke(ctx, userID, current)
	default:
		err = s.upSessions.Logout(ctx, userID, "")
	}
	// As in Strapi, a failed revocation is logged and the logout answers ok.
	if err != nil {
		s.log.Error("users-permissions logout failed", "error", err)
	}
	if s.refreshInCookie(r) {
		s.setRefreshCookie(w, "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

func (s *Server) authSessions(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	if s.upSessions == nil {
		return errNotFound
	}
	if rc.auth.User == nil {
		return newError(http.StatusUnauthorized, "UnauthorizedError", "Missing authentication")
	}
	list, err := s.upSessions.List(r.Context(), fmt.Sprint(rc.auth.userID()))
	if err != nil {
		return err
	}
	type entry struct {
		ID           string `json:"id"`
		DeviceID     string `json:"deviceId"`
		DeviceName   string `json:"deviceName,omitempty"`
		Current      bool   `json:"current"`
		LoginAt      string `json:"loginAt,omitempty"`
		LastActiveAt string `json:"lastActiveAt,omitempty"`
	}
	out := make([]entry, 0, len(list))
	for _, x := range list {
		e := entry{ID: x.SessionID, DeviceID: x.DeviceID, Current: rc.auth.SessionID != "" && x.SessionID == rc.auth.SessionID}
		e.DeviceName, _ = x.Metadata["deviceName"].(string)
		e.LoginAt, _ = x.Metadata["loginAt"].(string)
		if !x.CreatedAt.IsZero() {
			e.LastActiveAt = x.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}
		return out[i].LastActiveAt > out[j].LastActiveAt
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
	return nil
}

func (s *Server) authRevokeSession(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	if s.upSessions == nil {
		return errNotFound
	}
	if rc.auth.User == nil {
		return newError(http.StatusUnauthorized, "UnauthorizedError", "Missing authentication")
	}
	ok, err := s.upSessions.Revoke(r.Context(), fmt.Sprint(rc.auth.userID()), rc.params["sessionId"])
	if err != nil {
		return err
	}
	if !ok {
		return newError(http.StatusNotFound, "NotFoundError", "Session not found")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{}})
	return nil
}
