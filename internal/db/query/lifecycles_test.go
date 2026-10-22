package query_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/fixturetest"
)

func engine(t *testing.T) *query.Engine {
	t.Helper()
	ctx := context.Background()
	_, md, err := builtin.Load(os.DirFS(filepath.Join(fixturetest.Compat(), "fixtures", "basic")))
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	desired, _ := md.Schema()
	ran, err := migrate.Run(ctx, d, migrate.Options{RunMigrations: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: ran}); err != nil {
		t.Fatal(err)
	}
	return &query.Engine{DB: d, MD: md, Lifecycles: &query.Lifecycles{}}
}

func TestLifecycles(t *testing.T) {
	e := engine(t)
	ctx := context.Background()
	var seen []string
	off := e.Lifecycles.Subscribe(query.Subscriber{
		Models: []string{"api::tag.tag"},
		Hooks: map[string]func(*query.Event) error{
			"beforeCreate": func(ev *query.Event) error {
				data := ev.Params.Data.(map[string]any)
				if _, ok := data["createdAt"]; !ok {
					t.Error("timestamps must run before the subscribers")
				}
				data["name"] = data["name"].(string) + "!"
				ev.State["n"] = 1
				seen = append(seen, ev.Action)
				return nil
			},
			"afterCreate": func(ev *query.Event) error {
				if ev.State["n"] != 1 || ev.Result == nil {
					t.Errorf("state %v, result %v", ev.State, ev.Result)
				}
				seen = append(seen, ev.Action)
				return nil
			},
			"beforeDelete": func(*query.Event) error { return errors.New("protected") },
		},
	})
	e.Lifecycles.Subscribe(query.Subscriber{All: func(ev *query.Event) error {
		if ev.Model.UID == "api::tag.tag" {
			seen = append(seen, "all:"+ev.Action)
		}
		return nil
	}})
	row, err := e.Create(ctx, "api::tag.tag", query.Data{"name": "go"}, query.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := row.Get("name"); name != "go!" {
		t.Fatalf("name = %v", name)
	}
	id, _ := row.Get("id")
	if _, err := e.Delete(ctx, "api::tag.tag", map[string]any{"id": id}, query.Params{}); err == nil || err.Error() != "protected" {
		t.Fatalf("delete = %v", err)
	}
	if n, _ := e.Count(ctx, "api::tag.tag", query.Params{}); n != 1 {
		t.Fatalf("count = %d", n)
	}
	off()
	if _, err := e.Delete(ctx, "api::tag.tag", map[string]any{"id": id}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	t.Log(seen)
	if seen[0] != "beforeCreate" || seen[1] != "all:beforeCreate" {
		t.Fatalf("order = %v", seen)
	}
}
