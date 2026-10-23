package grapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
	_ "modernc.org/sqlite"
)

// migrationProject is a fixture copy whose schemas the test edits between
// starts on the same database.
type migrationProject struct {
	t   *testing.T
	dir string
}

func newMigrationProject(t *testing.T, fixture string) *migrationProject {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", fixture, "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	return &migrationProject{t: t, dir: dir}
}

func (p *migrationProject) start() (*grapi.App, func()) {
	p.t.Helper()
	app := grapi.New(grapi.Options{Dir: p.dir, Stderr: &bytes.Buffer{}})
	srv, err := app.Setup()
	if err != nil {
		p.t.Fatal(err)
	}
	return app, func() { _ = srv.Close() }
}

// edit rewrites an option of a content type's schema.json.
func (p *migrationProject) edit(api string, change func(schema map[string]any)) {
	p.t.Helper()
	path := filepath.Join(p.dir, "src", "api", api, "content-types", api, "schema.json")
	raw, err := os.ReadFile(path) //nolint:gosec // the test's copy
	if err != nil {
		p.t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		p.t.Fatal(err)
	}
	change(s)
	out, _ := json.Marshal(s)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		p.t.Fatal(err)
	}
}

func (p *migrationProject) rows(query string) [][2]any {
	p.t.Helper()
	d, err := sql.Open("sqlite", "file:"+filepath.Join(p.dir, "data.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		p.t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	r, err := d.Query(query)
	if err != nil {
		p.t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var out [][2]any
	for r.Next() {
		var a, b any
		if err := r.Scan(&a, &b); err != nil {
			p.t.Fatal(err)
		}
		out = append(out, [2]any{a, b})
	}
	return out
}

// storedFlag reads a flag of the stored schema, as Strapi reads it.
func (p *migrationProject) storedFlag(uid string, path ...string) any {
	p.t.Helper()
	rows := p.rows("select key, value from strapi_core_store_settings where key = 'strapi_content_types_schema'")
	if len(rows) != 1 {
		p.t.Fatalf("stored schema: %v", rows)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(rows[0][1].(string)), &schema); err != nil {
		p.t.Fatal(err)
	}
	v := any(schema[uid])
	for _, k := range path {
		m, _ := v.(map[string]any)
		v = m[k]
	}
	return v
}

func TestDraftAndPublishMigrations(t *testing.T) {
	p := newMigrationProject(t, "relations")
	ctx := context.Background()
	app, stop := p.start()
	docs := app.Documents()
	draftOnly, err := docs.Create(ctx, "api::post.post", document.Params{"data": map[string]any{"title": "Draft only"}})
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := docs.Create(ctx, "api::post.post", document.Params{"data": map[string]any{"title": "Published"}})
	pubID, _ := pub.Get("documentId")
	if _, err := docs.Publish(ctx, "api::post.post", pubID.(string), document.Params{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"retro", "snes"} {
		if _, err := docs.Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": name}}); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	if p.storedFlag("api::post.post", "options", "draftAndPublish") != true {
		t.Fatal("the first start stores the flags")
	}

	p.edit("post", func(s map[string]any) { s["options"] = map[string]any{"draftAndPublish": false} })
	p.edit("tag", func(s map[string]any) { s["options"] = map[string]any{"draftAndPublish": true} })
	_, stop = p.start()
	stop()
	posts := p.rows("select title, published_at from posts order by id")
	if len(posts) != 1 || posts[0][0] != "Published" || posts[0][1] == nil {
		t.Fatalf("draft and publish off keeps the published rows only: %v (draft-only %v)", posts, draftOnly)
	}
	tags := p.rows("select name, published_at from tags order by name, id")
	if len(tags) != 4 || tags[0][1] != nil && tags[1][1] != nil {
		t.Fatalf("draft and publish on gives every published tag its draft: %v", tags)
	}
	drafts := 0
	for _, r := range tags {
		if r[1] == nil {
			drafts++
		}
	}
	if drafts != 2 {
		t.Fatalf("one draft per tag: %v", tags)
	}
	if p.storedFlag("api::post.post", "options", "draftAndPublish") != false || p.storedFlag("api::tag.tag", "options", "draftAndPublish") != true {
		t.Fatal("the stored flags follow, so that Strapi does not replay the migrations")
	}

	// Nothing changed: nothing migrates again.
	_, stop = p.start()
	stop()
	if n := len(p.rows("select name, published_at from tags")); n != 4 {
		t.Fatalf("a second start replays nothing: %d tag rows", n)
	}
}

func TestI18nOffMigration(t *testing.T) {
	p := newMigrationProject(t, "i18n-dp")
	ctx := context.Background()
	app, stop := p.start()
	docs := app.Documents()
	if _, err := docs.Create(ctx, "api::tag.tag", document.Params{"locale": "en", "data": map[string]any{"label": "retro"}}); err != nil {
		t.Fatal(err)
	}
	row, _ := docs.Create(ctx, "api::tag.tag", document.Params{"locale": "en", "data": map[string]any{"label": "snes"}})
	id, _ := row.Get("documentId")
	if _, err := docs.Update(ctx, "api::tag.tag", id.(string), document.Params{"locale": "fr", "data": map[string]any{"label": "snes fr"}}); err != nil {
		t.Fatal(err)
	}
	stop()
	p.edit("tag", func(s map[string]any) { delete(s, "pluginOptions") })
	_, stop = p.start()
	stop()
	rows := p.rows("select label, locale from tags order by label")
	if len(rows) != 2 || rows[0][1] != nil || rows[1][1] != nil || strings.Contains(rows[0][0].(string)+rows[1][0].(string), "fr") {
		t.Fatalf("i18n off keeps the default locale's rows, without a locale: %v", rows)
	}
	if p.storedFlag("api::tag.tag", "pluginOptions", "i18n", "localized") == true {
		t.Fatal("the stored flag follows")
	}
}
