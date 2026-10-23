package webhooks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// Events are the events a webhook can listen to.
var Events = []string{"entry.create", "entry.update", "entry.delete", "entry.publish", "entry.unpublish", "media.create", "media.update", "media.delete"}

func (r *Runner) ph(n int) string { return r.DB.Dialect.Placeholder(n) }

func (r *Runner) scan(rows interface{ Scan(...any) error }) (Webhook, error) {
	var (
		w            Webhook
		headers, evs []byte
		name, url    *string
		enabled      any
	)
	if err := rows.Scan(&w.ID, &name, &url, &headers, &evs, &enabled); err != nil {
		return w, err
	}
	if name != nil {
		w.Name = *name
	}
	if url != nil {
		w.URL = *url
	}
	_ = json.Unmarshal(headers, &w.Headers)
	_ = json.Unmarshal(evs, &w.Events)
	switch v := enabled.(type) {
	case bool:
		w.Enabled = v
	case int64:
		w.Enabled = v != 0
	}
	if w.Headers == nil {
		w.Headers = map[string]string{}
	}
	if w.Events == nil {
		w.Events = []string{}
	}
	return w, nil
}

const columns = `"id", "name", "url", "headers", "events", "enabled"`

// All lists the webhooks.
func (r *Runner) All(ctx context.Context) ([]Webhook, error) {
	rows, err := r.DB.Query(ctx, `select `+columns+` from "strapi_webhooks" order by "id"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Webhook{}
	for rows.Next() {
		w, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ErrNotFound is a missing webhook.
var ErrNotFound = errors.New("webhook not found")

// Get returns the webhook id.
func (r *Runner) Get(ctx context.Context, id int64) (Webhook, error) {
	w, err := r.scan(r.DB.QueryRow(ctx, `select `+columns+` from "strapi_webhooks" where "id" = `+r.ph(1), id))
	if errors.Is(err, sql.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

func encode(w Webhook) (headers, events string) {
	h, _ := json.Marshal(w.Headers)
	e, _ := json.Marshal(w.Events)
	return string(h), string(e)
}

// Create stores w and returns it with its id.
func (r *Runner) Create(ctx context.Context, w Webhook) (Webhook, error) {
	h, e := encode(w)
	q := `insert into "strapi_webhooks" ("name", "url", "headers", "events", "enabled") values (` + strings.Join([]string{r.ph(1), r.ph(2), r.ph(3), r.ph(4), r.ph(5)}, ", ") + `) returning "id"`
	if err := r.DB.QueryRow(ctx, q, w.Name, w.URL, h, e, w.Enabled).Scan(&w.ID); err != nil {
		return w, err
	}
	return r.Get(ctx, w.ID)
}

// Update replaces the webhook w.ID.
func (r *Runner) Update(ctx context.Context, w Webhook) (Webhook, error) {
	h, e := encode(w)
	q := `update "strapi_webhooks" set "name" = ` + r.ph(1) + `, "url" = ` + r.ph(2) + `, "headers" = ` + r.ph(3) + `, "events" = ` + r.ph(4) + `, "enabled" = ` + r.ph(5) + ` where "id" = ` + r.ph(6)
	res, err := r.DB.Exec(ctx, q, w.Name, w.URL, h, e, w.Enabled, w.ID)
	if err != nil {
		return w, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return w, ErrNotFound
	}
	return r.Get(ctx, w.ID)
}

// Delete removes the webhook id.
func (r *Runner) Delete(ctx context.Context, id int64) error {
	res, err := r.DB.Exec(ctx, `delete from "strapi_webhooks" where "id" = `+r.ph(1), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
