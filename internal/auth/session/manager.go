// Package session is the session manager of the admin and of
// users-permissions in refresh mode, on the strapi_sessions table: a
// refresh token names a session row, an access token is short-lived and
// names the session it came from.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/aluzed/grapi/internal/auth/users"
)

// Token types: a refresh session survives the browser ("remember me"), a
// session one does not.
const (
	TypeRefresh = "refresh"
	TypeSession = "session"
)

// Statuses of a session row.
const (
	StatusActive  = "active"
	StatusRotated = "rotated"
)

// ErrInvalidToken is every refresh or access token rejection.
var ErrInvalidToken = errors.New("invalid token")

// Session is one strapi_sessions row.
type Session struct {
	UserID            string
	SessionID         string
	ChildID           string
	DeviceID          string
	Origin            string
	Type              string
	Status            string
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
	CreatedAt         time.Time
	Metadata          map[string]any
}

// Store persists the sessions.
type Store interface {
	Create(ctx context.Context, s *Session) error
	// Get returns nil when there is no such session.
	Get(ctx context.Context, sessionID string) (*Session, error)
	MarkRotated(ctx context.Context, sessionID, childID string) error
	Delete(ctx context.Context, sessionID string) error
	// ListActive is newest first.
	ListActive(ctx context.Context, origin, userID string) ([]*Session, error)
	// DeleteDevice removes the sessions of userID on deviceID, all of them
	// when deviceID is empty.
	DeleteDevice(ctx context.Context, origin, userID, deviceID string) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

// Config of one origin; zero durations take Strapi's defaults.
type Config struct {
	Secret    string
	Algorithm string
	// AccessTokenLifespan defaults to 30 minutes.
	AccessTokenLifespan time.Duration
	// MaxRefreshTokenLifespan (30 days) and IdleRefreshTokenLifespan (14
	// days) apply to remembered sessions, MaxSessionLifespan (1 day) and
	// IdleSessionLifespan (2 hours) to the others.
	MaxRefreshTokenLifespan  time.Duration
	IdleRefreshTokenLifespan time.Duration
	MaxSessionLifespan       time.Duration
	IdleSessionLifespan      time.Duration
}

func (c *Config) defaults() {
	set := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	day := 24 * time.Hour
	set(&c.AccessTokenLifespan, 30*time.Minute)
	set(&c.MaxRefreshTokenLifespan, 30*day)
	set(&c.IdleRefreshTokenLifespan, 14*day)
	set(&c.MaxSessionLifespan, day)
	set(&c.IdleSessionLifespan, 2*time.Hour)
}

// Manager manages the sessions of one origin ("admin", "users-permissions").
type Manager struct {
	store  Store
	origin string
	cfg    Config
	// Now is the clock, time.Now by default.
	Now     func() time.Time
	cleanup int
}

// New returns the manager of origin.
func New(store Store, origin string, cfg Config) *Manager {
	cfg.defaults()
	return &Manager{store: store, origin: origin, cfg: cfg, Now: time.Now}
}

// Config returns the effective configuration.
func (m *Manager) Config() Config { return m.cfg }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Manager) lifespans(typ string) (idle, maxLife time.Duration) {
	if typ == TypeSession {
		return m.cfg.IdleSessionLifespan, m.cfg.MaxSessionLifespan
	}
	return m.cfg.IdleRefreshTokenLifespan, m.cfg.MaxRefreshTokenLifespan
}

func (m *Manager) sign(s *Session) (string, error) {
	exp := int64(s.ExpiresAt.Sub(m.Now()).Seconds())
	if exp < 1 {
		exp = 1
	}
	return users.Sign([]users.KV{{Key: "userId", Value: s.UserID}, {Key: "sessionId", Value: s.SessionID}, {Key: "type", Value: "refresh"}}, m.cfg.Secret, m.cfg.Algorithm, fmt.Sprint(exp), m.Now())
}

// Login opens a session of typ for userID on deviceID (a new device id when
// empty) and returns its refresh token.
func (m *Manager) Login(ctx context.Context, userID, deviceID, typ string, metadata map[string]any) (string, *Session, error) {
	if m.cfg.Secret == "" {
		return "", nil, errors.New("session: missing secret")
	}
	m.cleanup++
	if m.cleanup >= 50 {
		m.cleanup = 0
		if err := m.store.DeleteExpired(ctx, m.Now()); err != nil {
			return "", nil, err
		}
	}
	if typ != TypeSession {
		typ = TypeRefresh
	}
	if deviceID == "" {
		deviceID = newID()
	}
	now := m.Now()
	idle, maxLife := m.lifespans(typ)
	s := &Session{
		UserID: userID, SessionID: newID(), DeviceID: deviceID, Origin: m.origin, Type: typ, Status: StatusActive,
		ExpiresAt: now.Add(idle), AbsoluteExpiresAt: now.Add(maxLife), CreatedAt: now, Metadata: metadata,
	}
	if s.AbsoluteExpiresAt.Before(s.ExpiresAt) {
		s.ExpiresAt = s.AbsoluteExpiresAt
	}
	if err := m.store.Create(ctx, s); err != nil {
		return "", nil, err
	}
	tok, err := m.sign(s)
	return tok, s, err
}

func (m *Manager) parse(token, typ string) (userID, sessionID string, err error) {
	p, err := users.Verify(token, m.cfg.Secret, m.cfg.Algorithm, m.Now())
	if err != nil || p["type"] != typ {
		return "", "", ErrInvalidToken
	}
	u, ok1 := p["userId"].(string)
	s, ok2 := p["sessionId"].(string)
	if !ok1 || !ok2 {
		return "", "", ErrInvalidToken
	}
	return u, s, nil
}

func (m *Manager) live(s *Session) bool {
	now := m.Now()
	return s != nil && s.Origin == m.origin && s.ExpiresAt.After(now) && (s.AbsoluteExpiresAt.IsZero() || s.AbsoluteExpiresAt.After(now))
}

// AccessToken issues an access token from an active refresh token.
func (m *Manager) AccessToken(ctx context.Context, refresh string) (string, error) {
	userID, sessionID, err := m.parse(refresh, "refresh")
	if err != nil {
		return "", err
	}
	s, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !m.live(s) || s.Status != StatusActive || s.UserID != userID {
		return "", ErrInvalidToken
	}
	return users.Sign([]users.KV{{Key: "userId", Value: userID}, {Key: "sessionId", Value: sessionID}, {Key: "type", Value: "access"}},
		m.cfg.Secret, m.cfg.Algorithm, fmt.Sprint(int64(m.cfg.AccessTokenLifespan.Seconds())), m.Now())
}

// Rotate exchanges a refresh token for the next one of its family. A token
// already rotated means it leaked: the whole device family is revoked.
func (m *Manager) Rotate(ctx context.Context, refresh string) (string, *Session, error) {
	userID, sessionID, err := m.parse(refresh, "refresh")
	if err != nil {
		return "", nil, err
	}
	cur, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return "", nil, err
	}
	if cur == nil || cur.UserID != userID || cur.Origin != m.origin {
		return "", nil, ErrInvalidToken
	}
	if cur.Status == StatusRotated {
		if err := m.store.DeleteDevice(ctx, m.origin, cur.UserID, cur.DeviceID); err != nil {
			return "", nil, err
		}
		return "", nil, ErrInvalidToken
	}
	if !m.live(cur) || cur.Status != StatusActive {
		return "", nil, ErrInvalidToken
	}
	now := m.Now()
	idle, _ := m.lifespans(cur.Type)
	child := &Session{
		UserID: cur.UserID, SessionID: newID(), DeviceID: cur.DeviceID, Origin: m.origin, Type: cur.Type, Status: StatusActive,
		ExpiresAt: now.Add(idle), AbsoluteExpiresAt: cur.AbsoluteExpiresAt, CreatedAt: now, Metadata: cur.Metadata,
	}
	if !child.AbsoluteExpiresAt.IsZero() && child.AbsoluteExpiresAt.Before(child.ExpiresAt) {
		child.ExpiresAt = child.AbsoluteExpiresAt
	}
	if err := m.store.Create(ctx, child); err != nil {
		return "", nil, err
	}
	if err := m.store.MarkRotated(ctx, cur.SessionID, child.SessionID); err != nil {
		return "", nil, err
	}
	tok, err := m.sign(child)
	return tok, child, err
}

// ValidateAccess checks an access token and that its session still exists
// and has not expired.
func (m *Manager) ValidateAccess(ctx context.Context, token string) (userID, sessionID string, err error) {
	userID, sessionID, err = m.parse(token, "access")
	if err != nil {
		return "", "", err
	}
	s, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return "", "", err
	}
	if s == nil || s.UserID != userID {
		return "", "", ErrInvalidToken
	}
	if !m.live(s) {
		if s.Origin == m.origin {
			_ = m.store.Delete(ctx, sessionID)
		}
		return "", "", ErrInvalidToken
	}
	return userID, sessionID, nil
}

// Get returns a session of this origin, nil when there is none.
func (m *Manager) Get(ctx context.Context, sessionID string) (*Session, error) {
	s, err := m.store.Get(ctx, sessionID)
	if err != nil || s == nil || s.Origin != m.origin {
		return nil, err
	}
	return s, nil
}

// List returns the active sessions of userID, newest first.
func (m *Manager) List(ctx context.Context, userID string) ([]*Session, error) {
	all, err := m.store.ListActive(ctx, m.origin, userID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, s := range all {
		if m.live(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// Revoke deletes the device family of sessionID when it belongs to userID.
func (m *Manager) Revoke(ctx context.Context, userID, sessionID string) (bool, error) {
	s, err := m.Get(ctx, sessionID)
	if err != nil || s == nil || s.UserID != userID {
		return false, err
	}
	return true, m.store.DeleteDevice(ctx, m.origin, userID, s.DeviceID)
}

// Logout deletes the sessions of userID on deviceID, every session when
// deviceID is empty.
func (m *Manager) Logout(ctx context.Context, userID, deviceID string) error {
	return m.store.DeleteDevice(ctx, m.origin, userID, deviceID)
}

// ParseRefresh reads a refresh token without checking its session.
func (m *Manager) ParseRefresh(token string) (userID, sessionID string, err error) {
	return m.parse(token, "refresh")
}
