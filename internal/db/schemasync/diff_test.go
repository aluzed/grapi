package schemasync

import (
	"encoding/json"
	"testing"

	"github.com/aluzed/grapi/internal/db/metadata"
)

type pg struct{}

func (pg) SQLType(t string) string {
	if t == "timestamp" {
		return "datetime"
	}
	return t
}
func (pg) UsesForeignKeys() bool  { return true }
func (pg) SupportsUnsigned() bool { return false }

func tbl(name string, cols ...string) *metadata.Table {
	t := &metadata.Table{Name: name, Indexes: []metadata.Index{}, ForeignKeys: []metadata.ForeignKey{}}
	for _, c := range cols {
		t.Columns = append(t.Columns, metadata.Column{Name: c, Type: "string", DefaultTo: json.RawMessage("null")})
	}
	return t
}

func schemaOf(tables ...*metadata.Table) *metadata.Schema { return &metadata.Schema{Tables: tables} }

func names(tables []*metadata.Table) []string {
	var out []string
	for _, t := range tables {
		out = append(out, t.Name)
	}
	return out
}

func TestDiffTracksOnlyWhatItCreated(t *testing.T) {
	database := schemaOf(tbl("a", "x", "custom"), tbl("old"), tbl("mine"), tbl("strapi_releases"), tbl("kept"), tbl("strapi_migrations"))
	previous := schemaOf(tbl("a", "x"), tbl("old"), tbl("strapi_releases"), tbl("kept"), tbl("strapi_migrations"))
	desired := schemaOf(tbl("a", "x", "y"), tbl("new"))
	c := Diff(pg{}, database, previous, desired, []PersistedTable{{Name: "kept"}})
	if got := names(c.Added); len(got) != 1 || got[0] != "new" {
		t.Errorf("added %v", got)
	}
	// "mine" was never tracked, the reserved and persisted tables are kept.
	if got := names(c.Removed); len(got) != 1 || got[0] != "old" {
		t.Errorf("removed %v", got)
	}
	if len(c.Updated) != 1 {
		t.Fatalf("updated %v", c.Updated)
	}
	cols := c.Updated[0].Columns
	if len(cols.Added) != 1 || cols.Added[0].Name != "y" || len(cols.Removed) != 0 {
		t.Errorf("columns %+v: the untracked column must stay", cols)
	}
}

func TestDiffFirstSyncRemovesNothing(t *testing.T) {
	c := Diff(pg{}, schemaOf(tbl("a", "x", "extra"), tbl("other")), nil, schemaOf(tbl("a", "x")), nil)
	if c.Status != Unchanged || len(c.Removed) != 0 {
		t.Fatalf("%+v", c)
	}
}

func TestDiffPersistedDependencies(t *testing.T) {
	database := schemaOf(tbl("gone"), tbl("child"))
	previous := schemaOf(tbl("gone"), tbl("child"))
	c := Diff(pg{}, database, previous, schemaOf(), []PersistedTable{{Name: "child", DependsOn: []string{"gone"}}})
	if got := names(c.Removed); len(got) != 2 || got[0] != "gone" || got[1] != "child" {
		t.Fatalf("removed %v", got)
	}
}

func TestColumnChanges(t *testing.T) {
	base := metadata.Column{Name: "c", Type: "string", DefaultTo: json.RawMessage("null")}
	for _, tc := range []struct {
		name string
		db   metadata.Column
		want bool
	}{
		{"same", base, false},
		{"type", metadata.Column{Name: "c", Type: "text", DefaultTo: json.RawMessage("null")}, true},
		{"not null", metadata.Column{Name: "c", Type: "string", NotNullable: true, DefaultTo: json.RawMessage("null")}, true},
		{"quoted default", metadata.Column{Name: "c", Type: "string", DefaultTo: json.RawMessage(`"'x'"`)}, true},
	} {
		if got := columnChanged(pg{}, tc.db, base); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	withDefault := base
	withDefault.DefaultTo = json.RawMessage(`"x"`)
	if columnChanged(pg{}, metadata.Column{Name: "c", Type: "string", DefaultTo: json.RawMessage(`"'x'"`)}, withDefault) {
		t.Error("a default read back quoted is the same default")
	}
	if columnChanged(pg{}, metadata.Column{Name: "id", Type: "integer"}, metadata.Column{Name: "id", Type: "increments"}) {
		t.Error("increments is read back as integer")
	}
}
