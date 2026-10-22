package schemasync

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/aluzed/grapi/internal/db/metadata"
)

// Status of a diff.
type Status string

// Statuses.
const (
	Changed   Status = "CHANGED"
	Unchanged Status = "UNCHANGED"
)

// Changes is what must be applied to the database.
type Changes struct {
	Status    Status
	Added     []*metadata.Table
	Updated   []*TableChanges
	Unchanged []*metadata.Table
	// Removed holds tracked tables the schema no longer has, with the
	// persisted tables that depend on them.
	Removed []*metadata.Table
}

// TableChanges is the diff of one table present in both schemas.
type TableChanges struct {
	Name        string
	Columns     Delta[metadata.Column]
	Indexes     Delta[metadata.Index]
	ForeignKeys Delta[metadata.ForeignKey]
}

// Delta lists added, updated (new definition), unchanged and removed objects.
type Delta[T any] struct {
	Added, Updated, Unchanged, Removed []T
}

func (d Delta[T]) changed() bool {
	return len(d.Added)+len(d.Updated)+len(d.Removed) > 0
}

// Dialect is what the diff needs to know about the database.
type Dialect interface {
	// SQLType maps a knex type to what the inspector reports for it.
	SQLType(knexType string) string
	UsesForeignKeys() bool
	SupportsUnsigned() bool
}

// PersistedTable is an entry of the core store's persisted_tables: a table
// kept across syncs, optionally dropped along with the tables it depends on.
type PersistedTable struct {
	Name      string
	DependsOn []string
}

// Tables the sync never drops: Strapi's own bookkeeping, and the tables of
// Strapi features grapi does not implement yet, so that opening a Strapi
// database does not destroy their data.
var reservedTables = []string{"strapi_migrations", "strapi_migrations_internal", "strapi_database_schema"}

var reservedPrefixes = []string{"strapi_release", "strapi_workflows", "strapi_history_", "strapi_ai_"}

func reserved(name string, persisted []PersistedTable) bool {
	if slices.Contains(reservedTables, name) {
		return true
	}
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, p := range persisted {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Diff compares the database with the desired schema. Objects absent from
// previous (the schema stored by the last sync) were not created by us and are
// left alone; previous is nil on a first sync.
func Diff(d Dialect, database, previous, desired *metadata.Schema, persisted []PersistedTable) *Changes {
	c := &Changes{}
	for _, want := range desired.Tables {
		have, ok := database.Table(want.Name)
		if !ok {
			c.Added = append(c.Added, want)
			continue
		}
		var prev *metadata.Table
		if previous != nil {
			prev, _ = previous.Table(want.Name)
		}
		tc := diffTable(d, have, prev, want)
		if tc.Columns.changed() || tc.Indexes.changed() || tc.ForeignKeys.changed() {
			c.Updated = append(c.Updated, tc)
		} else {
			c.Unchanged = append(c.Unchanged, have)
		}
	}
	for _, have := range database.Tables {
		_, inDesired := desired.Table(have.Name)
		tracked := false
		if previous != nil {
			_, tracked = previous.Table(have.Name)
		}
		if inDesired || !tracked || reserved(have.Name, persisted) {
			continue
		}
		c.Removed = append(c.Removed, have)
		for _, p := range persisted {
			if slices.Contains(p.DependsOn, have.Name) {
				if dep, ok := database.Table(p.Name); ok {
					c.Removed = append(c.Removed, dep)
				}
			}
		}
	}
	c.Status = Unchanged
	if len(c.Added)+len(c.Updated)+len(c.Removed) > 0 {
		c.Status = Changed
	}
	return c
}

func diffTable(d Dialect, have, prev, want *metadata.Table) *TableChanges {
	tc := &TableChanges{Name: have.Name}
	tc.Columns = diffObjects(have.Columns, prevOf(prev, func(t *metadata.Table) []metadata.Column { return t.Columns }), want.Columns,
		func(c metadata.Column) string { return c.Name },
		func(old, c metadata.Column) bool { return columnChanged(d, old, c) })
	tc.Indexes = diffObjects(have.Indexes, prevOf(prev, func(t *metadata.Table) []metadata.Index { return t.Indexes }), want.Indexes,
		func(i metadata.Index) string { return i.Name }, indexChanged)
	if d.UsesForeignKeys() {
		tc.ForeignKeys = diffObjects(have.ForeignKeys, prevOf(prev, func(t *metadata.Table) []metadata.ForeignKey { return t.ForeignKeys }), want.ForeignKeys,
			func(f metadata.ForeignKey) string { return f.Name }, foreignKeyChanged)
	}
	return tc
}

func prevOf[T any](prev *metadata.Table, get func(*metadata.Table) []T) []T {
	if prev == nil {
		return nil
	}
	return get(prev)
}

func diffObjects[T any](have, prev, want []T, name func(T) string, changed func(old, want T) bool) Delta[T] {
	var out Delta[T]
	find := func(list []T, n string) (T, bool) {
		for _, o := range list {
			if name(o) == n {
				return o, true
			}
		}
		var zero T
		return zero, false
	}
	for _, w := range want {
		h, ok := find(have, name(w))
		switch {
		case !ok:
			out.Added = append(out.Added, w)
		case changed(h, w):
			out.Updated = append(out.Updated, w)
		default:
			out.Unchanged = append(out.Unchanged, h)
		}
	}
	for _, h := range have {
		if _, inWant := find(want, name(h)); inWant {
			continue
		}
		if _, tracked := find(prev, name(h)); tracked {
			out.Removed = append(out.Removed, h)
		}
	}
	return out
}

func columnChanged(d Dialect, old, c metadata.Column) bool {
	if c.Type != "increments" && old.Type != d.SQLType(c.Type) {
		return true
	}
	if old.NotNullable != c.NotNullable {
		return true
	}
	if !sameDefault(old.DefaultTo, c.DefaultTo) {
		return true
	}
	return d.SupportsUnsigned() && old.Unsigned != c.Unsigned
}

// sameDefault compares like Strapi: case-insensitive, and a default read from
// the database may be the quoted form of the desired one.
func sameDefault(old, want json.RawMessage) bool {
	o, oNull := defaultString(old)
	w, wNull := defaultString(want)
	if oNull {
		return wNull
	}
	return strings.EqualFold(o, w) || strings.EqualFold(o, "'"+w+"'")
}

func defaultString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	return s, strings.EqualFold(s, "null")
}

func indexChanged(old, i metadata.Index) bool {
	if len(xor(i.Columns, old.Columns)) > 0 {
		return true
	}
	return old.Type != "" && i.Type != "" && !strings.EqualFold(old.Type, i.Type)
}

func foreignKeyChanged(old, f metadata.ForeignKey) bool {
	if len(difference(old.Columns, f.Columns)) > 0 || len(difference(old.ReferencedColumns, f.ReferencedColumns)) > 0 {
		return true
	}
	if old.ReferencedTable != f.ReferencedTable {
		return true
	}
	return actionChanged(old.OnDelete, f.OnDelete) || actionChanged(old.OnUpdate, f.OnUpdate)
}

func actionChanged(old, want string) bool {
	if old == "" || strings.EqualFold(old, "NO ACTION") {
		return want != "" && !strings.EqualFold(old, "NO ACTION")
	}
	return !strings.EqualFold(old, want)
}

func difference(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func xor(a, b []string) []string {
	return append(difference(a, b), difference(b, a)...)
}
