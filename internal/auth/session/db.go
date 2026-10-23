package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aluzed/grapi/internal/db/query"
)

// UID is the content type of the sessions.
const UID = "admin::session"

// DB stores the sessions in strapi_sessions.
type DB struct{ E *query.Engine }

// Create implements Store.
func (d DB) Create(ctx context.Context, s *Session) error {
	data := query.Data{
		"userId": s.UserID, "sessionId": s.SessionID, "deviceId": s.DeviceID, "origin": s.Origin,
		"type": s.Type, "status": s.Status, "expiresAt": s.ExpiresAt.UTC(),
	}
	if !s.AbsoluteExpiresAt.IsZero() {
		data["absoluteExpiresAt"] = s.AbsoluteExpiresAt.UTC()
	}
	if s.Metadata != nil {
		data["metadata"] = s.Metadata
	}
	_, err := d.E.Create(ctx, UID, data, query.Params{})
	return err
}

func str(r query.Row, k string) string {
	v, _ := r.Get(k)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// Time reads a datetime column, whatever the driver returned.
func Time(v any) time.Time {
	switch x := v.(type) {
	case time.Time:
		return x
	case string:
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t
			}
		}
	case int64:
		return time.UnixMilli(x)
	case float64:
		return time.UnixMilli(int64(x))
	}
	return time.Time{}
}

func fromRow(r query.Row) *Session {
	s := &Session{
		UserID: str(r, "userId"), SessionID: str(r, "sessionId"), ChildID: str(r, "childId"), DeviceID: str(r, "deviceId"),
		Origin: str(r, "origin"), Type: str(r, "type"), Status: str(r, "status"),
	}
	v, _ := r.Get("expiresAt")
	s.ExpiresAt = Time(v)
	v, _ = r.Get("absoluteExpiresAt")
	s.AbsoluteExpiresAt = Time(v)
	v, _ = r.Get("createdAt")
	s.CreatedAt = Time(v)
	v, _ = r.Get("metadata")
	switch m := v.(type) {
	case map[string]any:
		s.Metadata = m
	case string:
		_ = json.Unmarshal([]byte(m), &s.Metadata)
	}
	return s
}

// Get implements Store.
func (d DB) Get(ctx context.Context, id string) (*Session, error) {
	r, err := d.E.FindOne(ctx, UID, query.Params{Where: map[string]any{"sessionId": id}})
	if err != nil || r == nil {
		return nil, err
	}
	return fromRow(r), nil
}

// MarkRotated implements Store.
func (d DB) MarkRotated(ctx context.Context, id, child string) error {
	_, err := d.E.Update(ctx, UID, map[string]any{"sessionId": id}, query.Data{"status": StatusRotated, "childId": child}, query.Params{})
	return err
}

// Delete implements Store.
func (d DB) Delete(ctx context.Context, id string) error {
	_, err := d.E.DeleteMany(ctx, UID, query.Params{Where: map[string]any{"sessionId": id}})
	return err
}

// ListActive implements Store.
func (d DB) ListActive(ctx context.Context, origin, userID string) ([]*Session, error) {
	rows, err := d.E.FindMany(ctx, UID, query.Params{Where: map[string]any{"userId": userID, "origin": origin, "status": StatusActive}, OrderBy: []any{map[string]any{"createdAt": "desc"}}})
	if err != nil {
		return nil, err
	}
	out := make([]*Session, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}

// DeleteDevice implements Store.
func (d DB) DeleteDevice(ctx context.Context, origin, userID, deviceID string) error {
	where := map[string]any{"userId": userID, "origin": origin}
	if deviceID != "" {
		where["deviceId"] = deviceID
	}
	_, err := d.E.DeleteMany(ctx, UID, query.Params{Where: where})
	return err
}

// DeleteExpired implements Store.
func (d DB) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := d.E.DeleteMany(ctx, UID, query.Params{Where: map[string]any{"absoluteExpiresAt": map[string]any{"$lt": now.UTC()}}})
	return err
}
