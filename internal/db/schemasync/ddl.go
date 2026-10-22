package schemasync

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
)

// ddl writes the statements knex would run for Strapi's schema builder. The
// text matters on SQLite: the CREATE TABLE statement is stored as written and
// knex parses it again whenever Strapi later rebuilds the table.
type ddl struct {
	d db.Dialect
}

func (g ddl) sqlite() bool { return g.d.Name() == "sqlite" }

func (g ddl) q(id string) string { return g.d.Quote(id) }

func (g ddl) list(ids []string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = g.q(id)
	}
	return strings.Join(out, ", ")
}

func dbColumn(c metadata.Column) (db.Column, error) {
	dc := db.Column{Name: c.Name, Type: c.Type, NotNullable: c.NotNullable, Unsigned: c.Unsigned}
	if len(c.Args) > 0 {
		if err := json.Unmarshal(c.Args, &dc.Args); err != nil {
			return dc, fmt.Errorf("column %s: args: %w", c.Name, err)
		}
	}
	return dc, nil
}

// columnDef is a column as knex writes it in CREATE TABLE and ADD COLUMN.
// rebuilt is the form knex writes when it recreates a SQLite table.
func (g ddl) columnDef(c metadata.Column, rebuilt bool) (string, error) {
	dc, err := dbColumn(c)
	if err != nil {
		return "", err
	}
	typ, err := g.d.ColumnType(dc)
	if err != nil {
		return "", err
	}
	def := g.q(c.Name) + " " + typ
	if c.Type == "increments" {
		switch {
		case !g.sqlite():
			return g.q(c.Name) + " serial primary key", nil
		case rebuilt:
			return def + " PRIMARY KEY AUTOINCREMENT NOT NULL", nil
		default:
			return def + " not null primary key autoincrement", nil
		}
	}
	null, notNull, dflt := " null", " not null", " default "
	if rebuilt {
		null, notNull, dflt = " NULL", " NOT NULL", " DEFAULT "
	}
	if lit, ok := g.defaultLiteral(c.DefaultTo); ok {
		def += dflt + lit
	}
	if c.NotNullable {
		return def + notNull, nil
	}
	return def + null, nil
}

func (g ddl) defaultLiteral(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	if arr, ok := v.([]any); ok && len(arr) == 2 {
		if opts, ok := arr[1].(map[string]any); ok && opts["isRaw"] == true {
			return fmt.Sprint(arr[0]), true
		}
		v = arr[0]
	}
	switch x := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'", true
	case bool:
		if g.sqlite() {
			if x {
				return "'1'", true
			}
			return "'0'", true
		}
		return strconv.FormatBool(x), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	b, _ := json.Marshal(v)
	return "'" + strings.ReplaceAll(string(b), "'", "''") + "'", true
}

func (g ddl) foreignKeyDef(fk metadata.ForeignKey) string {
	def := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
		g.q(fk.Name), g.list(fk.Columns), g.q(fk.ReferencedTable), g.list(fk.ReferencedColumns))
	if fk.OnDelete != "" {
		def += " ON DELETE " + fk.OnDelete
	}
	if fk.OnUpdate != "" {
		def += " ON UPDATE " + fk.OnUpdate
	}
	return def
}

// createTable returns the statements creating t with its indexes. On SQLite
// the foreign keys are part of the table; Strapi declares them inline then
// adds them again, which makes knex rebuild the table: the stored definition
// is the rebuilt one, with each constraint twice.
func (g ddl) createTable(t *metadata.Table) ([]string, error) {
	rebuilt := g.sqlite() && len(t.ForeignKeys) > 0
	defs := make([]string, 0, len(t.Columns)+2*len(t.ForeignKeys))
	for _, c := range t.Columns {
		def, err := g.columnDef(c, rebuilt)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		defs = append(defs, def)
	}
	var stmts []string
	switch {
	case rebuilt:
		for range 2 {
			for _, fk := range t.ForeignKeys {
				defs = append(defs, g.foreignKeyDef(fk))
			}
		}
		stmts = append(stmts, fmt.Sprintf(`CREATE TABLE "%s" (%s)`, strings.ReplaceAll(t.Name, `"`, `""`), strings.Join(defs, ", ")))
	default:
		stmts = append(stmts, fmt.Sprintf("create table %s (%s)", g.q(t.Name), strings.Join(defs, ", ")))
	}
	for _, i := range t.Indexes {
		stmts = append(stmts, g.createIndex(t.Name, i))
	}
	return stmts, nil
}

// addForeignKeys returns the statements adding t's foreign keys, run once
// every table exists (Postgres only: SQLite declares them in CREATE TABLE).
func (g ddl) addForeignKeys(t *metadata.Table) []string {
	if g.sqlite() {
		return nil
	}
	stmts := make([]string, 0, len(t.ForeignKeys))
	for _, fk := range t.ForeignKeys {
		stmts = append(stmts, g.addForeignKey(t.Name, fk))
	}
	return stmts
}

func (g ddl) addForeignKey(table string, fk metadata.ForeignKey) string {
	s := fmt.Sprintf("alter table %s add constraint %s foreign key (%s) references %s (%s)",
		g.q(table), g.q(fk.Name), g.list(fk.Columns), g.q(fk.ReferencedTable), g.list(fk.ReferencedColumns))
	if fk.OnDelete != "" {
		s += " on delete " + fk.OnDelete
	}
	if fk.OnUpdate != "" {
		s += " on update " + fk.OnUpdate
	}
	return s
}

func (g ddl) dropForeignKey(table string, fk metadata.ForeignKey) string {
	return fmt.Sprintf("alter table %s drop constraint %s", g.q(table), g.q(fk.Name))
}

func (g ddl) createIndex(table string, i metadata.Index) string {
	switch {
	case i.Type == "primary":
		return fmt.Sprintf("alter table %s add constraint %s primary key (%s)", g.q(table), g.q(i.Name), g.list(i.Columns))
	case i.Type == "unique" && !g.sqlite():
		return fmt.Sprintf("alter table %s add constraint %s unique (%s)", g.q(table), g.q(i.Name), g.list(i.Columns))
	case i.Type == "unique":
		return fmt.Sprintf("create unique index %s on %s (%s)", g.q(i.Name), g.q(table), g.list(i.Columns))
	}
	return fmt.Sprintf("create index %s on %s (%s)", g.q(i.Name), g.q(table), g.list(i.Columns))
}

func (g ddl) dropIndex(table string, i metadata.Index) string {
	if !g.sqlite() && (i.Type == "unique" || i.Type == "primary") {
		return fmt.Sprintf("alter table %s drop constraint %s", g.q(table), g.q(i.Name))
	}
	return "drop index " + g.q(i.Name)
}

func (g ddl) addColumn(table string, c metadata.Column) (string, error) {
	if c.Type == "increments" && g.sqlite() {
		// SQLite cannot add an autoincrement column: knex adds a plain one.
		c = metadata.Column{Name: c.Name, Type: "integer", DefaultTo: c.DefaultTo}
	}
	def, err := g.columnDef(c, false)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("alter table %s add column %s", g.q(table), def), nil
}

func (g ddl) dropColumn(table, column string) string {
	return fmt.Sprintf("alter table %s drop column %s", g.q(table), g.q(column))
}

// alterColumn changes a Postgres column in place. SQLite cannot: see rebuild.
func (g ddl) alterColumn(table string, c metadata.Column) ([]string, error) {
	if c.Type == "increments" {
		c.Type = "integer"
	}
	dc, err := dbColumn(c)
	if err != nil {
		return nil, err
	}
	typ, err := g.d.ColumnType(dc)
	if err != nil {
		return nil, err
	}
	t, col := g.q(table), g.q(c.Name)
	stmts := []string{
		fmt.Sprintf("alter table %s alter column %s drop default", t, col),
		fmt.Sprintf("alter table %s alter column %s drop not null", t, col),
		fmt.Sprintf("alter table %s alter column %s type %s using (%s::%s)", t, col, typ, col, typ),
	}
	if lit, ok := g.defaultLiteral(c.DefaultTo); ok {
		stmts = append(stmts, fmt.Sprintf("alter table %s alter column %s set default %s", t, col, lit))
	}
	if c.NotNullable {
		stmts = append(stmts, fmt.Sprintf("alter table %s alter column %s set not null", t, col))
	}
	return stmts, nil
}

func (g ddl) dropTable(table string) string {
	return "drop table if exists " + g.q(table)
}
