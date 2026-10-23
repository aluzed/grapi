package document_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/fixturetest"
)

func service(t *testing.T, fixture string) *document.Service {
	t.Helper()
	return serviceOf(t, os.DirFS(filepath.Join(fixturetest.Compat(), "fixtures", fixture)))
}

func serviceOf(t *testing.T, app fs.FS) *document.Service {
	t.Helper()
	ctx := context.Background()
	res, md, err := builtin.Load(app)
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	desired, err := md.Schema()
	if err != nil {
		t.Fatal(err)
	}
	ran, err := migrate.Run(ctx, d, migrate.Options{RunMigrations: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: ran}); err != nil {
		t.Fatal(err)
	}
	return document.New(&query.Engine{DB: d, MD: md}, res.Set)
}

func TestMiddlewares(t *testing.T) {
	s := service(t, "basic")
	ctx := context.Background()
	var calls []string
	off := s.Use(func(c *document.Context, next document.Next) (any, error) {
		calls = append(calls, "outer:"+string(c.Action))
		if c.Action == document.Create {
			data, _ := c.Params["data"].(map[string]any)
			data["name"] = data["name"].(string) + " (checked)"
		}
		res, err := next()
		calls = append(calls, "outer:after")
		return res, err
	})
	s.Use(func(c *document.Context, next document.Next) (any, error) {
		calls = append(calls, "inner:"+c.UID)
		return next()
	})
	row, err := s.Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "Go"}})
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := row.Get("name"); name != "Go (checked)" {
		t.Fatalf("name = %v", name)
	}
	want := []string{"outer:create", "inner:api::tag.tag", "outer:after"}
	if len(calls) != 3 || calls[0] != want[0] || calls[1] != want[1] || calls[2] != want[2] {
		t.Fatalf("calls = %v", calls)
	}
	off()
	calls = nil
	doc, _ := row.Get("documentId")
	if _, err := s.FindOne(ctx, "api::tag.tag", doc.(string), document.Params{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "inner:api::tag.tag" {
		t.Fatalf("after removal: %v", calls)
	}
	s.Use(func(c *document.Context, next document.Next) (any, error) {
		if c.Action == document.FindOne {
			c.Params["documentId"] = "doesnotexist000000000000"
		}
		return next()
	})
	got, err := s.FindOne(ctx, "api::tag.tag", doc.(string), document.Params{})
	if err != nil || got != nil {
		t.Fatalf("documentId rewritten by a middleware: %v, %v", got, err)
	}
}

func TestPublishRunsBeforeCreate(t *testing.T) {
	s := service(t, "basic")
	s.E.Lifecycles = &query.Lifecycles{}
	ctx := context.Background()
	var created []any
	s.E.Lifecycles.Subscribe(query.Subscriber{Models: []string{"api::article.article"}, Hooks: map[string]func(*query.Event) error{
		"beforeCreate": func(e *query.Event) error {
			data := e.Params.Data.(map[string]any)
			created = append(created, data["publishedAt"])
			data["slug"] = "from-hook"
			return nil
		},
	}})
	row, err := s.Create(ctx, "api::article.article", document.Params{"data": map[string]any{"title": "Hello"}})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := row.Get("documentId")
	pub, err := s.Publish(ctx, "api::article.article", doc.(string), document.Params{})
	if err != nil || len(pub) != 1 {
		t.Fatalf("publish: %v, %v", pub, err)
	}
	if len(created) != 2 || created[0] != nil || created[1] == nil {
		t.Fatalf("beforeCreate saw publishedAt %v", created)
	}
	if slug, _ := pub[0].Get("slug"); slug != "from-hook" {
		t.Fatalf("published slug = %v", slug)
	}
}

func TestPublishedVersionAppearsOnPublish(t *testing.T) {
	s := service(t, "basic")
	ctx := context.Background()
	row, err := s.Create(ctx, "api::article.article", document.Params{"data": map[string]any{"title": "Draft only"}})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := row.Get("documentId")
	id := doc.(string)
	if pub, err := s.FindOne(ctx, "api::article.article", id, document.Params{"status": "published"}); err != nil || pub != nil {
		t.Fatalf("nothing published yet: %v %v", pub, err)
	}
	if draft, err := s.FindOne(ctx, "api::article.article", id, document.Params{}); err != nil || draft == nil {
		t.Fatalf("the draft is found by default: %v %v", draft, err)
	}
	if _, err := s.Publish(ctx, "api::article.article", id, document.Params{}); err != nil {
		t.Fatal(err)
	}
	if pub, err := s.FindOne(ctx, "api::article.article", id, document.Params{"status": "published"}); err != nil || pub == nil {
		t.Fatalf("published now: %v %v", pub, err)
	}
}
