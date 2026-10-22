package schemasync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
)

// Options of Sync.
type Options struct {
	// KeepRemoved keeps the tables, columns and indexes the schema no longer
	// has. Strapi's settings.forceMigration defaults to true, which drops them.
	KeepRemoved bool
	// MigrationsRan forces a full sync even when the hash is unchanged.
	MigrationsRan bool
	// Schema is the Postgres schema (public when empty).
	Schema string
	Log    *slog.Logger
}

// Sync brings the database in line with desired, then stores desired. It
// returns Unchanged without touching the database when the stored hash
// matches and no migration ran.
func Sync(ctx context.Context, d *db.DB, desired *metadata.Schema, opts Options) (Status, error) {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	stored, err := ReadStored(ctx, d)
	if err != nil {
		return "", fmt.Errorf("read stored schema: %w", err)
	}
	hash, err := Hash(desired)
	if err != nil {
		return "", err
	}
	if !opts.MigrationsRan && stored != nil && stored.Hash == hash {
		return Unchanged, nil
	}
	database, err := Inspect(ctx, d, opts.Schema)
	if err != nil {
		return "", fmt.Errorf("inspect database: %w", err)
	}
	persisted, err := persistedTables(ctx, d)
	if err != nil {
		return "", err
	}
	var previous *metadata.Schema
	if stored != nil {
		previous = stored.Schema
	}
	changes := Diff(dialect{d.Dialect}, database, previous, desired, persisted)
	if changes.Status == Changed {
		if err := apply(ctx, d, changes, opts); err != nil {
			return "", err
		}
	}
	if err := Store(ctx, d, desired); err != nil {
		return "", fmt.Errorf("store schema: %w", err)
	}
	return changes.Status, nil
}

type dialect struct{ d db.Dialect }

func (x dialect) SQLType(t string) string {
	switch {
	case t == "timestamp":
		return "datetime"
	case x.d.Name() == "sqlite" && (t == "double" || t == "decimal"):
		return "float"
	case x.d.Name() == "sqlite" && t == "enum":
		return "text"
	}
	return t
}

func (x dialect) UsesForeignKeys() bool  { return x.d.Name() == "postgres" }
func (x dialect) SupportsUnsigned() bool { return false }

type applier struct {
	d    *db.DB
	g    ddl
	opts Options
}

func (a *applier) exec(ctx context.Context, stmts ...string) error {
	for _, s := range stmts {
		if _, err := a.d.Exec(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	return nil
}

func apply(ctx context.Context, d *db.DB, c *Changes, opts Options) (err error) {
	a := &applier{d: d, g: ddl{d.Dialect}, opts: opts}
	if d.Dialect.Name() == "sqlite" {
		// Rebuilding a table must not cascade deletes to the rows pointing at it.
		if err := a.exec(ctx, "pragma foreign_keys = off"); err != nil {
			return err
		}
		defer func() {
			if e := a.exec(ctx, "pragma foreign_keys = on"); err == nil {
				err = e
			}
		}()
	}
	return d.Tx(ctx, func(ctx context.Context) error {
		if err := a.createTables(ctx, c.Added); err != nil {
			return err
		}
		if !opts.KeepRemoved {
			for _, t := range c.Removed {
				for _, fk := range t.ForeignKeys {
					if fk.Name != "" && !a.g.sqlite() {
						if err := a.exec(ctx, a.g.dropForeignKey(t.Name, fk)); err != nil {
							return err
						}
					}
				}
			}
			for _, t := range c.Removed {
				opts.Log.Warn("dropping table removed from the schema", "table", t.Name)
				if err := a.exec(ctx, a.g.dropTable(t.Name)); err != nil {
					return err
				}
			}
		} else {
			for _, t := range c.Removed {
				opts.Log.Warn("table removed from the schema is kept", "table", t.Name)
			}
		}
		for _, t := range c.Updated {
			if err := a.alterTable(ctx, t); err != nil {
				return fmt.Errorf("alter table %s: %w", t.Name, err)
			}
		}
		return nil
	})
}

func (a *applier) createTables(ctx context.Context, tables []*metadata.Table) error {
	for _, t := range tables {
		stmts, err := a.g.createTable(t)
		if err != nil {
			return err
		}
		if err := a.exec(ctx, stmts...); err != nil {
			return err
		}
	}
	for _, t := range tables {
		if err := a.exec(ctx, a.g.addForeignKeys(t)...); err != nil {
			return err
		}
	}
	return nil
}

// alterTable applies a table diff in Strapi's order: foreign keys and indexes
// are dropped first, columns changed, then constraints recreated.
func (a *applier) alterTable(ctx context.Context, t *TableChanges) error {
	g, drop := a.g, !a.opts.KeepRemoved
	for _, fk := range append(append([]metadata.ForeignKey{}, t.ForeignKeys.Removed...), t.ForeignKeys.Updated...) {
		if err := a.exec(ctx, g.dropForeignKey(t.Name, fk)); err != nil {
			return err
		}
	}
	for _, i := range t.Indexes.Removed {
		if drop {
			if err := a.exec(ctx, g.dropIndex(t.Name, i)); err != nil {
				return err
			}
		} else {
			a.opts.Log.Warn("index removed from the schema is kept", "table", t.Name, "index", i.Name)
		}
	}
	var recreate []metadata.Index
	for _, i := range t.Indexes.Updated {
		// Strapi only drops an index with forceMigration; recreating it
		// without dropping it would fail, so it is left as is.
		if !drop {
			a.opts.Log.Warn("changed index is kept as is", "table", t.Name, "index", i.Name)
			continue
		}
		if err := a.exec(ctx, g.dropIndex(t.Name, i)); err != nil {
			return err
		}
		recreate = append(recreate, i)
	}
	for _, c := range t.Columns.Removed {
		if !drop {
			a.opts.Log.Warn("column removed from the schema is kept", "table", t.Name, "column", c.Name)
			continue
		}
		if err := a.exec(ctx, g.dropColumn(t.Name, c.Name)); err != nil {
			return err
		}
	}
	if len(t.Columns.Updated) > 0 {
		if err := a.updateColumns(ctx, t.Name, t.Columns.Updated); err != nil {
			return err
		}
	}
	for _, c := range t.Columns.Added {
		stmt, err := g.addColumn(t.Name, c)
		if err != nil {
			return err
		}
		if err := a.exec(ctx, stmt); err != nil {
			return err
		}
	}
	for _, fk := range t.ForeignKeys.Updated {
		if err := a.exec(ctx, g.addForeignKey(t.Name, fk)); err != nil {
			return err
		}
	}
	for _, i := range recreate {
		if err := a.exec(ctx, g.createIndex(t.Name, i)); err != nil {
			return err
		}
	}
	for _, fk := range t.ForeignKeys.Added {
		if err := a.exec(ctx, g.addForeignKey(t.Name, fk)); err != nil {
			return err
		}
	}
	for _, i := range t.Indexes.Added {
		if err := a.exec(ctx, g.createIndex(t.Name, i)); err != nil {
			return err
		}
	}
	return nil
}

func (a *applier) updateColumns(ctx context.Context, table string, cols []metadata.Column) error {
	if a.g.sqlite() {
		return a.rebuildSQLite(ctx, table, cols)
	}
	for _, c := range cols {
		stmts, err := a.g.alterColumn(table, c)
		if err != nil {
			return err
		}
		if err := a.exec(ctx, stmts...); err != nil {
			return err
		}
	}
	return nil
}

// rebuildSQLite changes column definitions the only way SQLite allows: a new
// table with the edited CREATE statement, a copy of the rows, then the swap.
// Every other part of the statement is kept as it was, untracked columns and
// constraints included.
func (a *applier) rebuildSQLite(ctx context.Context, table string, cols []metadata.Column) error {
	var create string
	if err := a.d.QueryRow(ctx, `select sql from sqlite_master where type = 'table' and name = ?`, table).Scan(&create); err != nil {
		return err
	}
	open, end := strings.Index(create, "("), strings.LastIndex(create, ")")
	if open < 0 || end < open {
		return fmt.Errorf("cannot parse the definition of %s", table)
	}
	parts := splitTopLevel(create[open+1 : end])
	byName := map[string]metadata.Column{}
	for _, c := range cols {
		byName[c.Name] = c
	}
	var names []string
	for i, p := range parts {
		name, isColumn := leadingIdentifier(p)
		if !isColumn {
			continue
		}
		names = append(names, a.g.q(name))
		if c, ok := byName[name]; ok {
			def, err := a.g.columnDef(c, true)
			if err != nil {
				return err
			}
			parts[i] = def
		}
	}
	rows, err := a.d.Query(ctx, `select sql from sqlite_master where type = 'index' and tbl_name = ? and sql is not null`, table)
	if err != nil {
		return err
	}
	var indexes []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			_ = rows.Close()
			return err
		}
		indexes = append(indexes, s)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tmp := "_grapi_tmp_" + table
	cols1 := strings.Join(names, ", ")
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE "%s" (%s)`, tmp, strings.Join(parts, ", ")),
		fmt.Sprintf("insert into %s (%s) select %s from %s", a.g.q(tmp), cols1, cols1, a.g.q(table)),
		"drop table " + a.g.q(table),
		fmt.Sprintf("alter table %s rename to %s", a.g.q(tmp), a.g.q(table)),
	}
	return a.exec(ctx, append(stmts, indexes...)...)
}

// splitTopLevel splits a column list on commas outside parentheses and quotes.
func splitTopLevel(s string) []string {
	var (
		parts []string
		depth int
		quote rune
		start int
	)
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '`' || r == '"' || r == '\'' || r == '[':
			quote = r
			if r == '[' {
				quote = ']'
			}
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

var constraintKeywords = []string{"constraint", "primary", "foreign", "unique", "check"}

// leadingIdentifier returns the column name a definition starts with, or
// false for a table constraint.
func leadingIdentifier(def string) (string, bool) {
	if def == "" {
		return "", false
	}
	switch def[0] {
	case '`', '"', '[':
		closing := map[byte]byte{'`': '`', '"': '"', '[': ']'}[def[0]]
		end := strings.IndexByte(def[1:], closing)
		if end < 0 {
			return "", false
		}
		return def[1 : end+1], true
	}
	word := strings.Fields(def)[0]
	for _, k := range constraintKeywords {
		if strings.EqualFold(word, k) {
			return "", false
		}
	}
	return word, true
}
