package webhooks_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/events"
	"github.com/aluzed/grapi/internal/fixturetest"
	"github.com/aluzed/grapi/internal/webhooks"
)

type received struct {
	mu    sync.Mutex
	calls []map[string]any
	hdrs  []http.Header
	fail  int
}

func (rc *received) handler(w http.ResponseWriter, r *http.Request) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.fail > 0 {
		rc.fail--
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	rc.calls = append(rc.calls, m)
	rc.hdrs = append(rc.hdrs, r.Header.Clone())
}

func (rc *received) wait(t *testing.T, n int) {
	t.Helper()
	for range 100 {
		rc.mu.Lock()
		got := len(rc.calls)
		rc.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("got %d deliveries, want %d", len(rc.calls), n)
}

func TestDelivery(t *testing.T) {
	ctx := context.Background()
	res, md, err := builtin.Load(os.DirFS(filepath.Join(fixturetest.Compat(), "fixtures", "basic")))
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	desired, _ := md.Schema()
	ran, _ := migrate.Run(ctx, d, migrate.Options{RunMigrations: true})
	if _, err := schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: ran}); err != nil {
		t.Fatal(err)
	}
	rc := &received{fail: 1}
	ts := httptest.NewServer(http.HandlerFunc(rc.handler))
	defer ts.Close()
	e := &query.Engine{DB: d, MD: md}
	for _, w := range []query.Data{
		{"name": "on", "url": ts.URL, "headers": map[string]any{"Authorization": "Bearer x"}, "events": []any{"entry.create", "entry.publish"}, "enabled": true},
		{"name": "off", "url": ts.URL, "headers": map[string]any{}, "events": []any{"entry.create"}, "enabled": false},
	} {
		if _, err := e.Create(ctx, "strapi::webhook", w, query.Params{}); err != nil {
			t.Fatal(err)
		}
	}
	docs := document.New(e, res.Set)
	docs.Events = events.New(nil)
	r := &webhooks.Runner{DB: d, Hub: docs.Events, Config: webhooks.Config{Retries: 2, Backoff: 10 * time.Millisecond, DefaultHeaders: map[string]string{"X-Default": "1"}}}
	stop := r.Start(ctx)
	defer stop()
	row, err := docs.Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "Go"}})
	if err != nil {
		t.Fatal(err)
	}
	rc.wait(t, 1)
	got := rc.calls[0]
	if got["event"] != "entry.create" || got["model"] != "tag" || got["uid"] != "api::tag.tag" {
		t.Fatalf("payload %v", got)
	}
	entry, _ := got["entry"].(map[string]any)
	if entry["name"] != "Go" {
		t.Fatalf("entry %v", entry)
	}
	h := rc.hdrs[0]
	if h.Get("X-Strapi-Event") != "entry.create" || h.Get("Authorization") != "Bearer x" || h.Get("X-Default") != "1" || h.Get("Content-Type") != "application/json" {
		t.Fatalf("headers %v", h)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", got["createdAt"].(string)); err != nil {
		t.Fatalf("createdAt %v", got["createdAt"])
	}
	doc, _ := row.Get("documentId")
	_ = doc
	// A rolled back transaction emits nothing.
	err = d.Tx(ctx, func(ctx context.Context) error {
		if _, err := docs.Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "Rolled back"}}); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("want the rollback error")
	}
	time.Sleep(200 * time.Millisecond)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if len(rc.calls) != 1 {
		t.Fatalf("%d deliveries, the disabled webhook or the rolled back create got one", len(rc.calls))
	}
}

func TestDeliveryOrder(t *testing.T) {
	ctx := context.Background()
	_, md, err := builtin.Load(os.DirFS(filepath.Join(fixturetest.Compat(), "fixtures", "basic")))
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	desired, _ := md.Schema()
	ran, _ := migrate.Run(ctx, d, migrate.Options{RunMigrations: true})
	if _, err := schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: ran}); err != nil {
		t.Fatal(err)
	}
	rc := &received{}
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Uneven latency: parallel deliveries would overtake each other.
		time.Sleep(time.Duration(hits.Add(1)%3) * time.Millisecond)
		rc.handler(w, r)
	}))
	defer ts.Close()
	e := &query.Engine{DB: d, MD: md}
	if _, err := e.Create(ctx, "strapi::webhook", query.Data{"name": "order", "url": ts.URL, "headers": map[string]any{}, "events": []any{"entry.create"}, "enabled": true}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	hub := events.New(nil)
	r := &webhooks.Runner{DB: d, Hub: hub}
	stop := r.Start(ctx)
	defer stop()
	for i := range 30 {
		hub.Emit("entry.create", map[string]any{"entry": map[string]any{"n": i}})
	}
	rc.wait(t, 30)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for i, c := range rc.calls {
		if n := c["entry"].(map[string]any)["n"].(float64); int(n) != i {
			t.Fatalf("delivery %d carries event %v: a receiver gets its events in order", i, n)
		}
	}
}
