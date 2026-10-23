package schemasync_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/fixturetest"
)

func openSQLite(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

type masterRow struct {
	Type, Name string
	TblName    string `json:"tbl_name"`
	SQL        string
}

func sqliteMaster(t *testing.T, d *db.DB) map[string]masterRow {
	t.Helper()
	rows, err := d.Query(context.Background(), `select type, name, tbl_name, sql from sqlite_master where sql is not null and name not like 'sqlite%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]masterRow{}
	for rows.Next() {
		var r masterRow
		if err := rows.Scan(&r.Type, &r.Name, &r.TblName, &r.SQL); err != nil {
			t.Fatal(err)
		}
		out[r.Type+" "+r.Name] = r
	}
	return out
}

// Tables Strapi's migration runner creates (DB-004), not the sync.
var migrationTables = map[string]bool{"strapi_migrations": true, "strapi_migrations_internal": true}

// On an empty database, the sync creates exactly what Strapi 5.59 creates:
// the same CREATE TABLE and CREATE INDEX statements, byte for byte.
func TestSyncEmptySQLiteMatchesStrapi(t *testing.T) {
	ctx := context.Background()
	for _, fixture := range fixturetest.All() {
		t.Run(fixture, func(t *testing.T) {
			_, desired := fixturetest.Schema(t, fixture)
			d := openSQLite(t)
			status, err := schemasync.Sync(ctx, d, desired, schemasync.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if status != schemasync.Changed {
				t.Fatalf("first sync: %s", status)
			}
			raw, err := os.ReadFile(fixturetest.Golden(fixture, "sqlite", "schema", "physical-sqlite-master.json"))
			if err != nil {
				t.Fatal(err)
			}
			var golden []masterRow
			if err := json.Unmarshal(raw, &golden); err != nil {
				t.Fatal(err)
			}
			got := sqliteMaster(t, d)
			compared := 0
			for _, want := range golden {
				if fixturetest.Excluded(want.TblName) || migrationTables[want.TblName] {
					continue
				}
				compared++
				key := want.Type + " " + want.Name
				g, ok := got[key]
				if !ok {
					t.Errorf("missing %s", key)
					continue
				}
				delete(got, key)
				if g.SQL != want.SQL {
					t.Errorf("%s\n got: %s\nwant: %s", key, g.SQL, want.SQL)
				}
			}
			var extra []string
			for k := range got {
				extra = append(extra, k)
			}
			sort.Strings(extra)
			if len(extra) > 0 {
				t.Errorf("not created by Strapi: %v", extra)
			}
			if compared < 20 {
				t.Fatalf("only %d objects compared", compared)
			}

			status, err = schemasync.Sync(ctx, d, desired, schemasync.Options{})
			if err != nil || status != schemasync.Unchanged {
				t.Fatalf("second sync: %s, %v", status, err)
			}
			// A full diff (as after a migration) finds nothing either.
			status, err = schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: true})
			if err != nil || status != schemasync.Unchanged {
				t.Fatalf("full sync on a synced database: %s, %v", status, err)
			}
		})
	}
}

func TestSyncStoresStrapiHash(t *testing.T) {
	ctx := context.Background()
	_, desired := fixturetest.Schema(t, "basic")
	d := openSQLite(t)
	if _, err := schemasync.Sync(ctx, d, desired, schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	stored, err := schemasync.ReadStored(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("testdata/strapi-hashes.json")
	var h struct {
		Hashes map[string]struct{ CE string } `json:"hashes"`
	}
	_ = json.Unmarshal(b, &h)
	if want := h.Hashes["golden/basic-sqlite/schema/database-schema.json"].CE; stored.Hash != want {
		t.Fatalf("stored hash %s, Strapi computes %s for the same tables", stored.Hash, want)
	}
	if len(stored.Schema.Tables) != len(desired.Tables) {
		t.Fatalf("stored %d tables, want %d", len(stored.Schema.Tables), len(desired.Tables))
	}
}

func clone(t *testing.T, s *metadata.Schema) *metadata.Schema {
	t.Helper()
	out := &metadata.Schema{}
	for _, table := range s.Tables {
		b, err := metadata.Marshal(table)
		if err != nil {
			t.Fatal(err)
		}
		c, err := metadata.ParseTable(b)
		if err != nil {
			t.Fatal(err)
		}
		out.Tables = append(out.Tables, c)
	}
	return out
}

func columnNames(t *testing.T, d *db.DB, table string) string {
	t.Helper()
	rows, err := d.Query(context.Background(), "pragma table_info("+d.Dialect.Quote(table)+")")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, typ        string
			def              any
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return strings.Join(names, ",")
}

func withoutColumn(t *testing.T, s *metadata.Schema, table, column string) *metadata.Schema {
	t.Helper()
	out := clone(t, s)
	for i, tb := range out.Tables {
		if tb.Name != table {
			continue
		}
		var cols []metadata.Column
		for _, c := range tb.Columns {
			if c.Name != column {
				cols = append(cols, c)
			}
		}
		// A new table value: ParseTable keeps the raw JSON, which must change too.
		nt := &metadata.Table{Name: tb.Name, Indexes: tb.Indexes, ForeignKeys: tb.ForeignKeys, Columns: cols}
		out.Tables[i] = nt
	}
	return out
}

func TestSyncAddsColumnWithoutDataLoss(t *testing.T) {
	ctx := context.Background()
	_, full := fixturetest.Schema(t, "basic")
	d := openSQLite(t)
	if _, err := schemasync.Sync(ctx, d, withoutColumn(t, full, "tags", "name"), schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "insert into tags (document_id) values ('keep-me')"); err != nil {
		t.Fatal(err)
	}
	status, err := schemasync.Sync(ctx, d, full, schemasync.Options{})
	if err != nil || status != schemasync.Changed {
		t.Fatalf("%s, %v", status, err)
	}
	if !strings.Contains(columnNames(t, d, "tags"), "name") {
		t.Fatal("column not added")
	}
	var n int
	if err := d.QueryRow(ctx, "select count(*) from tags where document_id = 'keep-me'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("row lost: %d, %v", n, err)
	}
}

func TestSyncRemovedColumn(t *testing.T) {
	ctx := context.Background()
	_, full := fixturetest.Schema(t, "basic")
	for _, keep := range []bool{true, false} {
		d := openSQLite(t)
		if _, err := schemasync.Sync(ctx, d, full, schemasync.Options{}); err != nil {
			t.Fatal(err)
		}
		if _, err := schemasync.Sync(ctx, d, withoutColumn(t, full, "tags", "name"), schemasync.Options{KeepRemoved: keep}); err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(columnNames(t, d, "tags"), "name")
		if has != keep {
			t.Errorf("KeepRemoved %v: column still there: %v", keep, has)
		}
	}
}

func TestSyncLeavesUntrackedTablesAlone(t *testing.T) {
	ctx := context.Background()
	_, full := fixturetest.Schema(t, "basic")
	d := openSQLite(t)
	if _, err := d.Exec(ctx, "create table my_own_table (id integer primary key, v text)"); err != nil {
		t.Fatal(err)
	}
	if _, err := schemasync.Sync(ctx, d, full, schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := schemasync.Sync(ctx, d, withoutColumn(t, full, "tags", "name"), schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	if columnNames(t, d, "my_own_table") != "id,v" {
		t.Fatal("untracked table was changed")
	}
}

func TestSyncRebuildsChangedSQLiteColumn(t *testing.T) {
	ctx := context.Background()
	_, full := fixturetest.Schema(t, "basic")
	d := openSQLite(t)
	if _, err := schemasync.Sync(ctx, d, full, schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "insert into tags (document_id, name) values ('d1', '42')"); err != nil {
		t.Fatal(err)
	}
	changed := clone(t, full)
	for i, tb := range changed.Tables {
		if tb.Name != "tags" {
			continue
		}
		cols := append([]metadata.Column(nil), tb.Columns...)
		for j := range cols {
			if cols[j].Name == "name" {
				cols[j].Type = "text"
				cols[j].Args = json.RawMessage(`["longtext"]`)
			}
		}
		changed.Tables[i] = &metadata.Table{Name: tb.Name, Indexes: tb.Indexes, ForeignKeys: tb.ForeignKeys, Columns: cols}
	}
	status, err := schemasync.Sync(ctx, d, changed, schemasync.Options{})
	if err != nil || status != schemasync.Changed {
		t.Fatalf("%s, %v", status, err)
	}
	var typ string
	if err := d.QueryRow(ctx, "select type from pragma_table_info('tags') where name = 'name'").Scan(&typ); err != nil || !strings.EqualFold(typ, "text") {
		t.Fatalf("type %q, %v", typ, err)
	}
	var name string
	if err := d.QueryRow(ctx, "select name from tags where document_id = 'd1'").Scan(&name); err != nil || name != "42" {
		t.Fatalf("row lost: %q, %v", name, err)
	}
	idx, err := schemasync.SQLiteIndexes(ctx, d, "tags")
	if err != nil || len(idx) == 0 {
		t.Fatalf("indexes lost: %v, %v", idx, err)
	}
}
