//go:build integration

package schemasync_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/fixturetest"
)

func openPostgres(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("GRAPI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("GRAPI_TEST_PG_DSN not set")
	}
	d, err := db.Open(context.Background(), db.Config{Client: "postgres", ConnectionString: dsn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	resetPostgres(t, d)
	return d
}

func resetPostgres(t *testing.T, d *db.DB) {
	t.Helper()
	if _, err := d.Exec(context.Background(), "drop schema public cascade; create schema public"); err != nil {
		t.Fatal(err)
	}
}

// The queries of compat/scripts/remote.sh, which produced the golden files.
var physicalQueries = map[string]string{
	"physical-columns.json":     `select coalesce(json_agg(c order by c.table_name, c.ordinal_position), '[]') from (select table_name, column_name, ordinal_position, data_type, is_nullable, column_default, character_maximum_length, numeric_precision, numeric_scale from information_schema.columns where table_schema = 'public') c`,
	"physical-indexes.json":     `select coalesce(json_agg(i order by i.tablename, i.indexname), '[]') from (select tablename, indexname, indexdef from pg_indexes where schemaname = 'public') i`,
	"physical-constraints.json": `select coalesce(json_agg(f order by f.table_name, f.constraint_name), '[]') from (select tc.table_name, tc.constraint_name, tc.constraint_type, pg_get_constraintdef(pc.oid) as definition from information_schema.table_constraints tc join pg_constraint pc on pc.conname = tc.constraint_name where tc.table_schema = 'public' and tc.constraint_type in ('FOREIGN KEY', 'UNIQUE', 'PRIMARY KEY')) f`,
}

func tableOf(row map[string]any) string {
	for _, k := range []string{"table_name", "tablename"} {
		if s, ok := row[k].(string); ok {
			return s
		}
	}
	return ""
}

func physicalRows(t *testing.T, raw []byte) []string {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		table := tableOf(r)
		if fixturetest.Excluded(table) || table == "strapi_migrations" || table == "strapi_migrations_internal" {
			continue
		}
		b, _ := json.Marshal(r)
		out = append(out, string(b))
	}
	return out
}

func TestSyncEmptyPostgresMatchesStrapi(t *testing.T) {
	ctx := context.Background()
	d := openPostgres(t)
	for _, fixture := range fixturetest.Names {
		t.Run(fixture, func(t *testing.T) {
			resetPostgres(t, d)
			_, desired := fixturetest.Schema(t, fixture)
			if status, err := schemasync.Sync(ctx, d, desired, schemasync.Options{}); err != nil || status != schemasync.Changed {
				t.Fatalf("first sync: %s, %v", status, err)
			}
			for file, query := range physicalQueries {
				var got []byte
				if err := d.QueryRow(ctx, query).Scan(&got); err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join(fixturetest.Compat(), "golden", fixture+"-postgres", "schema", file))
				if err != nil {
					t.Fatal(err)
				}
				g, w := physicalRows(t, got), physicalRows(t, bytes.TrimSpace(want))
				if len(w) < 10 {
					t.Fatalf("%s: only %d golden rows", file, len(w))
				}
				gs := map[string]bool{}
				for _, r := range g {
					gs[r] = true
				}
				ws := map[string]bool{}
				for _, r := range w {
					ws[r] = true
					if !gs[r] {
						t.Errorf("%s: missing %s", file, r)
					}
				}
				for _, r := range g {
					if !ws[r] {
						t.Errorf("%s: not created by Strapi: %s", file, r)
					}
				}
			}
			if status, err := schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: true}); err != nil || status != schemasync.Unchanged {
				t.Fatalf("full sync on a synced database: %s, %v", status, err)
			}
		})
	}
}

func TestPostgresChangeColumnType(t *testing.T) {
	ctx := context.Background()
	d := openPostgres(t)
	_, full := fixturetest.Schema(t, "basic")
	if _, err := schemasync.Sync(ctx, d, full, schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "insert into tags (document_id, weight) values ('d1', 42)"); err != nil {
		t.Fatal(err)
	}
	changed := clone(t, full)
	for i, tb := range changed.Tables {
		if tb.Name != "tags" {
			continue
		}
		cols := append([]metadata.Column(nil), tb.Columns...)
		for j := range cols {
			if cols[j].Name == "weight" {
				cols[j].Type = "bigInteger"
			}
		}
		changed.Tables[i] = &metadata.Table{Name: tb.Name, Indexes: tb.Indexes, ForeignKeys: tb.ForeignKeys, Columns: cols}
	}
	if status, err := schemasync.Sync(ctx, d, changed, schemasync.Options{}); err != nil || status != schemasync.Changed {
		t.Fatalf("%s, %v", status, err)
	}
	var typ string
	var w int64
	if err := d.QueryRow(ctx, "select data_type from information_schema.columns where table_name = 'tags' and column_name = 'weight'").Scan(&typ); err != nil || typ != "bigint" {
		t.Fatalf("type %q, %v", typ, err)
	}
	if err := d.QueryRow(ctx, "select weight from tags where document_id = 'd1'").Scan(&w); err != nil || w != 42 {
		t.Fatalf("value %d, %v", w, err)
	}
}
