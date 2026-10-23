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
	"sync"
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
	// BlockPrivateNetworks refuses deliveries to loopback, private and
	// link-local addresses (SSRF protection, a grapi option).
	BlockPrivateNetworks bool
}

// Webhook is a row of strapi_webhooks.
type Webhook struct {
	ID      int64             `json:"id"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Events  []string          `json:"events"`
	Enabled bool              `json:"isEnabled"`
}

// Runner listens to the hub and posts to the webhooks.
type Runner struct {
	DB     *db.DB
	Hub    *events.Hub
	Config Config
	Client *http.Client
	Log    *slog.Logger
	now    func() time.Time
	ctx    context.Context
	mu     sync.Mutex
	// queues hold one queue and one worker per webhook: each receiver gets
	// its events in order, and a slow one delays only itself.
	queues map[int64]chan job
}

type job struct {
	webhook Webhook
	body    []byte
	event   string
}

// Start subscribes to the hub; each webhook gets its own delivery worker
// on its first event.
func (r *Runner) Start(ctx context.Context) func() {
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 10 * time.Second}
		if r.Config.BlockPrivateNetworks {
			r.Client.Transport = publicOnlyTransport()
		}
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
	ctx, cancel := context.WithCancel(ctx)
	r.ctx, r.queues = ctx, map[int64]chan job{}
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
	for _, k := range []string{"model", "uid", "entry", "media", "isPublished", "release", "error"} {
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
		case r.queueOf(w.ID) <- job{webhook: w, body: body, event: name}:
		default:
			r.Log.Error("webhooks: queue full, event dropped", "event", name, "webhook", w.Name)
		}
	}
}

func (r *Runner) queueOf(id int64) chan job {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.queues[id]
	if !ok {
		q = make(chan job, 1000)
		r.queues[id] = q
		go r.worker(r.ctx, q)
	}
	return q
}

func (r *Runner) worker(ctx context.Context, q chan job) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-q:
			r.deliver(ctx, j)
		}
	}
}

// Send posts body to w once.
func (r *Runner) Send(ctx context.Context, w Webhook, event string, body []byte) error {
	status, err := r.post(ctx, w, event, body)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("status %d", status)
	}
	return nil
}

// Test posts the trigger-test event to w, as the admin's trigger button,
// and returns the response status.
func (r *Runner) Test(ctx context.Context, w Webhook) (int, error) {
	body, err := Body("trigger-test", r.clock(), map[string]any{})
	if err != nil {
		return 0, err
	}
	return r.post(ctx, w, "trigger-test", body)
}

func (r *Runner) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *Runner) post(ctx context.Context, w Webhook, event string, body []byte) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
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
		return 0, err
	}
	_ = res.Body.Close()
	return res.StatusCode, nil
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
