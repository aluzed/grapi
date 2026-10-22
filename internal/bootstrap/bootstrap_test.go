package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi/internal/bootstrap"
	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/db/store"
)

func TestFreshDatabase(t *testing.T) {
	ctx := context.Background()
	res, md, err := builtin.Load(os.DirFS(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
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
	e := &query.Engine{DB: d, MD: md}
	for range 2 {
		if err := bootstrap.Run(ctx, e, res.Set, bootstrap.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(uid string) int64 {
		n, err := e.Count(ctx, uid, query.Params{})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("plugin::users-permissions.role") != 2 || count("plugin::users-permissions.permission") != 13 || count("plugin::i18n.locale") != 1 {
		t.Fatalf("roles %d, permissions %d, locales %d", count("plugin::users-permissions.role"), count("plugin::users-permissions.permission"), count("plugin::i18n.locale"))
	}
	var advanced map[string]any
	if found, _ := (store.Store{DB: d}).Get(ctx, store.Scope{Type: "plugin", Name: "users-permissions"}, "advanced", &advanced); !found || advanced["default_role"] != "authenticated" {
		t.Fatalf("advanced = %v", advanced)
	}
	var code string
	if _, _ = (store.Store{DB: d}).Get(ctx, store.Scope{Type: "plugin", Name: "i18n"}, "default_locale", &code); code != "en" {
		t.Fatalf("default_locale = %q", code)
	}
}
