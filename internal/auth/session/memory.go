package session

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Memory is an in-memory Store, for tests.
type Memory struct {
	mu   sync.Mutex
	rows map[string]Session
}

// Create implements Store.
func (s *Memory) Create(_ context.Context, r *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows == nil {
		s.rows = map[string]Session{}
	}
	s.rows[r.SessionID] = *r
	return nil
}

// Get implements Store.
func (s *Memory) Get(_ context.Context, id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

// MarkRotated implements Store.
func (s *Memory) MarkRotated(_ context.Context, id, child string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok {
		r.Status, r.ChildID = StatusRotated, child
		s.rows[id] = r
	}
	return nil
}

// Delete implements Store.
func (s *Memory) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, id)
	return nil
}

// ListActive implements Store.
func (s *Memory) ListActive(_ context.Context, origin, userID string) ([]*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Session
	for _, r := range s.rows {
		if r.Origin == origin && r.UserID == userID && r.Status == StatusActive {
			out = append(out, &r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// DeleteDevice implements Store.
func (s *Memory) DeleteDevice(_ context.Context, origin, userID, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.rows {
		if r.Origin == origin && r.UserID == userID && (deviceID == "" || r.DeviceID == deviceID) {
			delete(s.rows, id)
		}
	}
	return nil
}

// DeleteExpired implements Store.
func (s *Memory) DeleteExpired(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.rows {
		if !r.AbsoluteExpiresAt.IsZero() && r.AbsoluteExpiresAt.Before(now) {
			delete(s.rows, id)
		}
	}
	return nil
}

// Len is the number of rows.
// Len implements Store.
func (s *Memory) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}
