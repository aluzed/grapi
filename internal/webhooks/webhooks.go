// Package webhooks delivers the events of the hub to the webhooks of
// strapi_webhooks, like Strapi's webhook runner, with retries.
package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/events"
)

// Config is server.webhooks.
type Config struct {
	DefaultHeaders map[string]string
	// Retries after a failed delivery; 0 is Strapi's behavior (log only).
	Retries int
	// Backoff is the first wait between tries, doubled each time.
	Backoff time.Duration
}

// Webhook is a row of strapi_webhooks.
type Webhook struct {
	ID      int64
	Name    string
	URL     string
	Headers map[string]string
	Events  []string
}

// Runner listens to the hub and posts to the webhooks.
type Runner struct {
	DB     *db.DB
	Hub    *events.Hub
	Config Config
	Client *http.Client
	Log    *slog.Logger
	queue  chan job
	now    func() time.Time
}

type job struct {
	webhook Webhook
	body    []byte
	event   string
}

// Start subscribes to the hub and starts 5 delivery workers.
func (r *Runner) Start(ctx context.Context) func() {
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if r.Log == nil {
		r.Log = slog.Default()
	}
	if r.Config.Backoff == 0 {
		r.Config.Backoff = time.Second
	}
	if r.now == nil {
		r.now = time.Now
	}
	r.queue = make(chan job, 1000)
	ctx, cancel := context.WithCancel(ctx)
	for range 5 {
		go r.worker(ctx)
	}
	off := r.Hub.Subscribe(func(name string, payload map[string]any) { r.dispatch(ctx, name, payload) })
	return func() {
		off()
		cancel()
	}
}

// Enabled lists the enabled webhooks listening to event.
func (r *Runner) Enabled(ctx context.Context, event string) ([]Webhook, error) {
	q := `select "id", "name", "url", "headers", "events" from "strapi_webhooks" where "enabled" = ` + r.DB.Dialect.Placeholder(1)
	rows, err := r.DB.Query(ctx, q, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Webhook
	for rows.Next() {
		var (
			w            Webhook
			headers, evs []byte
			name, url    *string
		)
		if err := rows.Scan(&w.ID, &name, &url, &headers, &evs); err != nil {
			return nil, err
		}
		if name != nil {
			w.Name = *name
		}
		if url != nil {
			w.URL = *url
		}
		_ = json.Unmarshal(headers, &w.Headers)
		_ = json.Unmarshal(evs, &w.Events)
		for _, e := range w.Events {
			if e == event {
				out = append(out, w)
				break
			}
		}
	}
	return out, rows.Err()
}

// Body is the payload Strapi posts: {event, createdAt, ...info}.
func Body(event string, at time.Time, info map[string]any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"event":`)
	ev, _ := json.Marshal(event)
	b.Write(ev)
	b.WriteString(`,"createdAt":"` + at.UTC().Format("2006-01-02T15:04:05.000Z") + `"`)
	for _, k := range []string{"model", "uid", "entry", "media"} {
		v, ok := info[k]
		if !ok {
			continue
		}
		enc, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		b.WriteString(`,"` + k + `":`)
		b.Write(enc)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (r *Runner) dispatch(ctx context.Context, name string, payload map[string]any) {
	hooks, err := r.Enabled(ctx, name)
	if err != nil {
		r.Log.Error("webhooks: cannot list webhooks", "error", err)
		return
	}
	if len(hooks) == 0 {
		return
	}
	body, err := Body(name, r.now(), payload)
	if err != nil {
		r.Log.Error("webhooks: cannot encode the event", "event", name, "error", err)
		return
	}
	for _, w := range hooks {
		select {
		case r.queue <- job{webhook: w, body: body, event: name}:
		default:
			r.Log.Error("webhooks: queue full, event dropped", "event", name, "webhook", w.Name)
		}
	}
}

func (r *Runner) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-r.queue:
			r.deliver(ctx, j)
		}
	}
}

// Send posts body to w once.
func (r *Runner) Send(ctx context.Context, w Webhook, event string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range r.Config.DefaultHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-Strapi-Event", event)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.Client.Do(req)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return nil
}

func (r *Runner) deliver(ctx context.Context, j job) {
	wait := r.Config.Backoff
	for try := 0; ; try++ {
		err := r.Send(ctx, j.webhook, j.event, j.body)
		if err == nil {
			return
		}
		if try >= r.Config.Retries {
			r.Log.Error("Error running webhook", "webhook", j.webhook.Name, "event", j.event, "error", err)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait *= 2
	}
}
