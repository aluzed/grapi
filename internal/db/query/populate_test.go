package query

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/fixturetest"
)

type countingHandler struct{ n *atomic.Int64 }

func (h countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h countingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "sql" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "query" && strings.HasPrefix(strings.ToLower(a.Value.String()), "select") {
				h.n.Add(1)
			}
			return true
		})
	}
	return nil
}
func (h countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h countingHandler) WithGroup(string) slog.Handler      { return h }

// A three level populate on 100 entries runs one query per relation and
// level, whatever the number of entries.
func TestPopulateQueriesDoNotGrowWithEntries(t *testing.T) {
	ctx := context.Background()
	md, desired := fixturetest.Schema(t, "relations")
	var n atomic.Int64
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db"), Debug: true}, slog.New(countingHandler{&n}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := schemasync.Sync(ctx, d, desired, schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	e := &Engine{DB: d, MD: md}
	profile, _ := e.Create(ctx, "api::profile.profile", Data{"bio": "b"}, Params{})
	pid, _ := profile.Get("id")
	writer, _ := e.Create(ctx, "api::writer.writer", Data{"name": "w", "profile": pid}, Params{})
	wid, _ := writer.Get("id")
	tag, _ := e.Create(ctx, "api::tag.tag", Data{"name": "t"}, Params{})
	tid, _ := tag.Get("id")
	for range 100 {
		if _, err := e.Create(ctx, "api::post.post", Data{"title": "p", "writer": wid, "tags": []any{tid}}, Params{}); err != nil {
			t.Fatal(err)
		}
	}
	n.Store(0)
	rows, err := e.FindMany(ctx, "api::post.post", Params{Populate: map[string]any{
		"writer": map[string]any{"populate": map[string]any{"profile": map[string]any{"populate": []any{"writer"}}}},
		"tags":   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 100 {
		t.Fatalf("%d rows", len(rows))
	}
	// posts, writer, writer.profile, writer.profile.writer, tags.
	if got := n.Load(); got != 5 {
		t.Fatalf("%d select queries, want 5", got)
	}
}

// Rows of a morph table whose type no longer exists are removed by the
// repair, the others stay.
func TestRemoveOrphanMorphTypes(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, "relations")
	tag, _ := e.Create(ctx, "api::tag.tag", Data{"name": "t"}, Params{})
	tid, _ := tag.Get("id")
	if _, err := e.Create(ctx, "api::post.post", Data{"title": "p", "mentions": []any{map[string]any{"id": tid, "__type": "api::tag.tag"}}}, Params{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sqlExec(ctx, "insert into posts_mentions_mph (post_id, mentions_id, mentions_type, \"order\") values (1, 9, 'api::gone.gone', 2)"); err != nil {
		t.Fatal(err)
	}
	if err := schemasync.RemoveOrphanMorphTypes(ctx, e.DB, e.MD, "mentions_type", nil); err != nil {
		t.Fatal(err)
	}
	rows, _ := e.Table(ctx, "posts_mentions_mph")
	if len(rows) != 1 || rows[0]["mentions_type"] != "api::tag.tag" {
		t.Fatalf("%v", rows)
	}
}
