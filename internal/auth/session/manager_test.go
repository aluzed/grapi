package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newManager(st Store, c *clock) *Manager {
	m := New(st, "admin", Config{Secret: "s3cret"})
	m.Now = c.now
	return m
}

func TestLoginAccessRotate(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	m := newManager(st, c)
	refresh, s, err := m.Login(ctx, "1", "", TypeRefresh, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.DeviceID == "" || s.AbsoluteExpiresAt.Sub(c.t) != 30*24*time.Hour || s.ExpiresAt.Sub(c.t) != 14*24*time.Hour {
		t.Fatalf("session %+v", s)
	}
	access, err := m.AccessToken(ctx, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if u, sid, err := m.ValidateAccess(ctx, access); err != nil || u != "1" || sid != s.SessionID {
		t.Fatalf("ValidateAccess = %q %q %v", u, sid, err)
	}
	if _, _, err := m.ValidateAccess(ctx, refresh); !errors.Is(err, ErrInvalidToken) {
		t.Error("a refresh token is not an access token")
	}
	c.advance(31 * time.Minute)
	if _, _, err := m.ValidateAccess(ctx, access); !errors.Is(err, ErrInvalidToken) {
		t.Error("access token expires after 30 minutes")
	}
	next, child, err := m.Rotate(ctx, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if child.DeviceID != s.DeviceID || !child.AbsoluteExpiresAt.Equal(s.AbsoluteExpiresAt) {
		t.Errorf("child %+v keeps the family", child)
	}
	if _, err := m.AccessToken(ctx, refresh); !errors.Is(err, ErrInvalidToken) {
		t.Error("a rotated token gives no access token")
	}
	if _, err := m.AccessToken(ctx, next); err != nil {
		t.Error(err)
	}
}

func TestReplayRevokesTheFamily(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	m := newManager(st, c)
	refresh, _, _ := m.Login(ctx, "1", "dev", TypeRefresh, nil)
	_, _, _ = m.Login(ctx, "1", "other", TypeRefresh, nil)
	next, child, err := m.Rotate(ctx, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, again, err := m.Rotate(ctx, refresh); err != nil || again.SessionID != child.SessionID {
		t.Fatalf("a concurrent refresh gets the same child: %v %v", again, err)
	}
	c.advance(RotationGrace + time.Second)
	if _, _, err := m.Rotate(ctx, refresh); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("replay must fail")
	}
	if _, _, err := m.Rotate(ctx, next); !errors.Is(err, ErrInvalidToken) {
		t.Error("the stolen family is revoked")
	}
	if st.Len() != 1 {
		t.Errorf("other devices are kept, %d rows", st.Len())
	}
}

func TestIdleAndAbsoluteExpiry(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	m := newManager(st, c)
	refresh, _, _ := m.Login(ctx, "1", "", TypeSession, nil)
	c.advance(2*time.Hour + time.Second)
	if _, _, err := m.Rotate(ctx, refresh); !errors.Is(err, ErrInvalidToken) {
		t.Error("idle session lifespan is 2 hours")
	}

	refresh, _, _ = m.Login(ctx, "1", "", TypeSession, nil)
	for i := 0; i < 14; i++ {
		c.advance(time.Hour + 50*time.Minute)
		var err error
		refresh, _, err = m.Rotate(ctx, refresh)
		if i < 13 && err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
		if i == 13 && !errors.Is(err, ErrInvalidToken) {
			t.Fatal("max session lifespan is 1 day")
		}
	}
}

func TestListRevokeLogout(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	m := newManager(st, c)
	_, a, _ := m.Login(ctx, "1", "a", TypeRefresh, map[string]any{"deviceName": "Firefox"})
	c.advance(time.Second)
	refreshB, b, _ := m.Login(ctx, "1", "b", TypeRefresh, nil)
	_, _, _ = m.Login(ctx, "2", "c", TypeRefresh, nil)
	list, _ := m.List(ctx, "1")
	if len(list) != 2 || list[0].SessionID != b.SessionID {
		t.Fatalf("List = %v", list)
	}
	if ok, _ := m.Revoke(ctx, "2", a.SessionID); ok {
		t.Error("another user's session cannot be revoked")
	}
	if ok, _ := m.Revoke(ctx, "1", a.SessionID); !ok {
		t.Error("revoke")
	}
	_ = m.Logout(ctx, "1", "b")
	if _, err := m.AccessToken(ctx, refreshB); !errors.Is(err, ErrInvalidToken) {
		t.Error("logged out")
	}
	if st.Len() != 1 {
		t.Errorf("%d rows left", st.Len())
	}
}

func TestOtherOrigin(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	refresh, _, _ := newManager(st, c).Login(ctx, "1", "", TypeRefresh, nil)
	up := New(st, "users-permissions", Config{Secret: "s3cret"})
	up.Now = c.now
	if _, err := up.AccessToken(ctx, refresh); !errors.Is(err, ErrInvalidToken) {
		t.Error("a session of another origin is refused")
	}
}

func TestReplayAfterTheChildRotated(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	m := newManager(st, c)
	first, _, _ := m.Login(ctx, "1", "dev", TypeRefresh, nil)
	second, _, err := m.Rotate(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Rotate(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Rotate(ctx, first); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("the first token is two rotations old")
	}
	if st.Len() != 0 {
		t.Errorf("the family is revoked, %d rows", st.Len())
	}
}

func TestExpiredAccessToken(t *testing.T) {
	ctx := context.Background()
	st, c := &Memory{}, &clock{time.Unix(1_700_000_000, 0)}
	m := newManager(st, c)
	refresh, _, _ := m.Login(ctx, "1", "dev", TypeRefresh, nil)
	access, err := m.AccessToken(ctx, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ValidateAccess(ctx, access); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	c.advance(m.Config().AccessTokenLifespan + time.Second)
	if _, _, err := m.ValidateAccess(ctx, access); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("an expired access token is refused: %v", err)
	}
	if fresh, err := m.AccessToken(ctx, refresh); err != nil || fresh == access {
		t.Fatalf("the refresh token still gives a new one: %v", err)
	}
}
