package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aluzed/grapi/internal/db"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	// Strapi's DDL for the table.
	if _, err := d.Exec(ctx, "create table `strapi_core_store_settings` (`id` integer not null primary key autoincrement, `key` varchar(255) null, `value` text null, `type` varchar(255) null, `environment` varchar(255) null, `tag` varchar(255) null)"); err != nil {
		t.Fatal(err)
	}
	s := Store{DB: d}
	up := Scope{Type: "plugin", Name: "users-permissions"}
	values := map[string]any{
		"advanced": map[string]any{"unique_email": true, "default_role": "authenticated"},
		"list":     []any{"a", float64(2)},
		"flag":     true,
		"locale":   "en",
		"count":    float64(42),
	}
	for k, v := range values {
		if err := s.Set(ctx, up, k, v); err != nil {
			t.Fatal(err)
		}
	}
	for k, want := range values {
		var got any
		found, err := s.Get(ctx, up, k, &got)
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, %v, %v; want %#v", k, got, found, err, want)
		}
	}
	var raw, typ string
	if err := d.QueryRow(ctx, "select `value`, `type` from `strapi_core_store_settings` where `key` = 'plugin_users-permissions_locale'").Scan(&raw, &typ); err != nil {
		t.Fatal(err)
	}
	if raw != `"en"` || typ != "string" {
		t.Fatalf("stored %q as %q, Strapi stores \"\\\"en\\\"\" as string", raw, typ)
	}
	if err := d.QueryRow(ctx, "select `type` from `strapi_core_store_settings` where `key` = 'plugin_users-permissions_list'").Scan(&typ); err != nil || typ != "object" {
		t.Fatalf("array type = %q (typeof [] is object)", typ)
	}
	if err := s.Set(ctx, up, "flag", false); err != nil {
		t.Fatal(err)
	}
	var flag bool
	if _, err := s.Get(ctx, up, "flag", &flag); err != nil || flag {
		t.Fatalf("flag = %v, %v", flag, err)
	}
	var missing any
	if found, err := s.Get(ctx, Scope{Type: "core", Name: "admin"}, "nope", &missing); found || err != nil {
		t.Fatalf("missing key: %v, %v", found, err)
	}
	if err := s.Delete(ctx, up, "flag"); err != nil {
		t.Fatal(err)
	}
	if found, _ := s.Get(ctx, up, "flag", &flag); found {
		t.Fatal("deleted key still there")
	}
}
