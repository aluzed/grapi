package schemasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
)

// Inspect reads the tables, columns, indexes and foreign keys of the database,
// in the shape of Strapi's schema inspectors.
func Inspect(ctx context.Context, d *db.DB, pgSchema string) (*metadata.Schema, error) {
	if d.Dialect.Name() == "postgres" {
		return inspectPostgres(ctx, d, orPublic(pgSchema))
	}
	return inspectSQLite(ctx, d)
}

func orPublic(s string) string {
	if s == "" {
		return "public"
	}
	return s
}

var rootType = regexp.MustCompile(`[^(), ]+`)

func column(name, typ string, def sql.NullString, notNullable bool) metadata.Column {
	c := metadata.Column{Name: name, Type: typ, NotNullable: notNullable, DefaultTo: json.RawMessage("null")}
	if def.Valid {
		c.DefaultTo, _ = json.Marshal(def.String)
	}
	return c
}

func inspectSQLite(ctx context.Context, d *db.DB) (*metadata.Schema, error) {
	names, err := queryStrings(ctx, d, `select name from sqlite_master where type = 'table' and name NOT LIKE 'sqlite%'`)
	if err != nil {
		return nil, err
	}
	s := &metadata.Schema{Tables: []*metadata.Table{}}
	for _, name := range names {
		t := &metadata.Table{Name: name, Columns: []metadata.Column{}, Indexes: []metadata.Index{}, ForeignKeys: []metadata.ForeignKey{}}
		if err := sqliteColumns(ctx, d, t); err != nil {
			return nil, err
		}
		if t.Indexes, err = SQLiteIndexes(ctx, d, name); err != nil {
			return nil, err
		}
		if t.ForeignKeys, err = sqliteForeignKeys(ctx, d, name); err != nil {
			return nil, err
		}
		s.Tables = append(s.Tables, t)
	}
	return s, nil
}

func sqliteColumns(ctx context.Context, d *db.DB, t *metadata.Table) error {
	rows, err := d.Query(ctx, "pragma table_info("+d.Dialect.Quote(t.Name)+")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, typ        string
			def              sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			return err
		}
		t.Columns = append(t.Columns, column(name, sqliteType(typ, pk > 0), def, notnull != 0))
	}
	return rows.Err()
}

func sqliteType(typ string, pk bool) string {
	switch rootType.FindString(strings.ToLower(typ)) {
	case "integer":
		if pk {
			return "increments"
		}
		return "integer"
	case "float":
		return "float"
	case "bigint":
		return "bigInteger"
	case "varchar":
		return "string"
	case "text":
		return "text"
	case "json":
		return "jsonb"
	case "boolean", "datetime", "date", "time":
		return rootType.FindString(strings.ToLower(typ))
	}
	return "specificType"
}

// SQLiteIndexes lists the indexes of a table, without SQLite's automatic ones.
func SQLiteIndexes(ctx context.Context, d *db.DB, table string) ([]metadata.Index, error) {
	rows, err := d.Query(ctx, "pragma index_list("+d.Dialect.Quote(table)+")")
	if err != nil {
		return nil, err
	}
	type idx struct {
		name   string
		unique bool
	}
	var list []idx
	for rows.Next() {
		var (
			seq, unique, partial int
			name, origin         string
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if !strings.HasPrefix(name, "sqlite_") {
			list = append(list, idx{name, unique != 0})
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := []metadata.Index{}
	for _, i := range list {
		cols, err := indexColumns(ctx, d, "pragma index_info("+d.Dialect.Quote(i.name)+")")
		if err != nil {
			return nil, err
		}
		index := metadata.Index{Name: i.name, Columns: cols}
		if i.unique {
			index.Type = "unique"
		}
		out = append(out, index)
	}
	return out, nil
}

func sqliteForeignKeys(ctx context.Context, d *db.DB, table string) ([]metadata.ForeignKey, error) {
	rows, err := d.Query(ctx, "pragma foreign_key_list("+d.Dialect.Quote(table)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []metadata.ForeignKey{}
	byID := map[int]int{}
	for rows.Next() {
		var (
			id, seq                              int
			ref, from, to, onUpdate, onDelete, m string
		)
		if err := rows.Scan(&id, &seq, &ref, &from, &to, &onUpdate, &onDelete, &m); err != nil {
			return nil, err
		}
		if i, ok := byID[id]; ok {
			out[i].Columns = append(out[i].Columns, from)
			out[i].ReferencedColumns = append(out[i].ReferencedColumns, to)
			continue
		}
		byID[id] = len(out)
		out = append(out, metadata.ForeignKey{
			Columns: []string{from}, ReferencedColumns: []string{to}, ReferencedTable: ref,
			OnUpdate: strings.ToUpper(onUpdate), OnDelete: strings.ToUpper(onDelete),
		})
	}
	return out, rows.Err()
}

func inspectPostgres(ctx context.Context, d *db.DB, schema string) (*metadata.Schema, error) {
	names, err := queryStrings(ctx, d, `select table_name from information_schema.tables
		where table_schema = $1 and table_type = 'BASE TABLE'
		and table_name != 'geometry_columns' and table_name != 'spatial_ref_sys'`, schema)
	if err != nil {
		return nil, err
	}
	s := &metadata.Schema{Tables: []*metadata.Table{}}
	if len(names) == 0 {
		return s, nil
	}
	byName := map[string]*metadata.Table{}
	for _, n := range names {
		t := &metadata.Table{Name: n, Columns: []metadata.Column{}, Indexes: []metadata.Index{}, ForeignKeys: []metadata.ForeignKey{}}
		byName[n] = t
		s.Tables = append(s.Tables, t)
	}
	if err := pgColumns(ctx, d, schema, names, byName); err != nil {
		return nil, err
	}
	if err := pgIndexes(ctx, d, schema, names, byName); err != nil {
		return nil, err
	}
	if err := pgForeignKeys(ctx, d, schema, names, byName); err != nil {
		return nil, err
	}
	return s, nil
}

func pgColumns(ctx context.Context, d *db.DB, schema string, names []string, byName map[string]*metadata.Table) error {
	rows, err := d.Query(ctx, `select table_name, data_type, column_name, column_default, is_nullable
		from information_schema.columns where table_schema = $1 and table_name = any($2)
		order by table_name, ordinal_position`, schema, names)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			table, dataType, name, nullable string
			def                             sql.NullString
		)
		if err := rows.Scan(&table, &dataType, &name, &def, &nullable); err != nil {
			return err
		}
		if def.Valid && strings.Contains(def.String, "nextval(") {
			def.Valid = false
		}
		byName[table].Columns = append(byName[table].Columns, column(name, pgType(dataType), def, nullable == "NO"))
	}
	return rows.Err()
}

func pgType(dataType string) string {
	switch rootType.FindString(strings.ToLower(dataType)) {
	case "integer":
		return "integer"
	case "text":
		return "text"
	case "boolean":
		return "boolean"
	case "character":
		return "string"
	case "timestamp":
		return "datetime"
	case "date":
		return "date"
	case "time":
		return "time"
	case "numeric":
		return "decimal"
	case "real", "double":
		return "double"
	case "bigint":
		return "bigInteger"
	case "jsonb":
		return "jsonb"
	}
	return "specificType"
}

func pgIndexes(ctx context.Context, d *db.DB, schema string, names []string, byName map[string]*metadata.Table) error {
	rows, err := d.Query(ctx, `select t.relname, ix.indexrelid::text, i.relname, a.attname, ix.indisunique, ix.indisprimary
		from pg_class t, pg_namespace s, pg_class i, pg_index ix, pg_attribute a
		where t.oid = ix.indrelid and i.oid = ix.indexrelid and a.attrelid = t.oid
		and a.attnum = any(ix.indkey) and t.relkind = 'r' and t.relnamespace = s.oid
		and s.nspname = $1 and t.relname = any($2)
		order by t.relname, i.relname, array_position(ix.indkey::int2[], a.attnum)`, schema, names)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	pos := map[string]int{}
	for rows.Next() {
		var (
			table, relid, name, col string
			unique, primary         bool
		)
		if err := rows.Scan(&table, &relid, &name, &col, &unique, &primary); err != nil {
			return err
		}
		if col == "id" {
			continue
		}
		t := byName[table]
		if i, ok := pos[relid]; ok {
			t.Indexes[i].Columns = append(t.Indexes[i].Columns, col)
			continue
		}
		index := metadata.Index{Name: name, Columns: []string{col}}
		switch {
		case primary:
			index.Type = "primary"
		case unique:
			index.Type = "unique"
		}
		pos[relid] = len(t.Indexes)
		t.Indexes = append(t.Indexes, index)
	}
	return rows.Err()
}

func pgForeignKeys(ctx context.Context, d *db.DB, schema string, names []string, byName map[string]*metadata.Table) error {
	rows, err := d.Query(ctx, `select cl.relname, c.conname, att.attname, fcl.relname, fatt.attname,
		case c.confupdtype when 'a' then 'NO ACTION' when 'r' then 'RESTRICT' when 'c' then 'CASCADE' when 'n' then 'SET NULL' when 'd' then 'SET DEFAULT' end,
		case c.confdeltype when 'a' then 'NO ACTION' when 'r' then 'RESTRICT' when 'c' then 'CASCADE' when 'n' then 'SET NULL' when 'd' then 'SET DEFAULT' end
		from pg_constraint c
		join pg_class cl on cl.oid = c.conrelid
		join pg_namespace n on n.oid = cl.relnamespace
		join pg_class fcl on fcl.oid = c.confrelid
		join lateral unnest(c.conkey, c.confkey) as cols(conkey, confkey) on true
		join pg_attribute att on att.attrelid = c.conrelid and att.attnum = cols.conkey
		join pg_attribute fatt on fatt.attrelid = c.confrelid and fatt.attnum = cols.confkey
		where c.contype = 'f' and n.nspname = $1 and cl.relname = any($2) and fcl.relnamespace = cl.relnamespace
		order by cl.relname, c.conname, cols.conkey`, schema, names)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	pos := map[string]int{}
	for rows.Next() {
		var table, name, col, ref, refCol, onUpdate, onDelete string
		if err := rows.Scan(&table, &name, &col, &ref, &refCol, &onUpdate, &onDelete); err != nil {
			return err
		}
		t := byName[table]
		key := table + "\x00" + name
		if i, ok := pos[key]; ok {
			fk := &t.ForeignKeys[i]
			if !contains(fk.Columns, col) {
				fk.Columns = append(fk.Columns, col)
			}
			if !contains(fk.ReferencedColumns, refCol) {
				fk.ReferencedColumns = append(fk.ReferencedColumns, refCol)
			}
			continue
		}
		pos[key] = len(t.ForeignKeys)
		t.ForeignKeys = append(t.ForeignKeys, metadata.ForeignKey{
			Name: name, Columns: []string{col}, ReferencedColumns: []string{refCol}, ReferencedTable: ref,
			OnUpdate: onUpdate, OnDelete: onDelete,
		})
	}
	return rows.Err()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func queryStrings(ctx context.Context, d *db.DB, query string, args ...any) ([]string, error) {
	rows, err := d.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// indexColumns returns the column names of pragma index_info.
func indexColumns(ctx context.Context, d *db.DB, query string) ([]string, error) {
	rows, err := d.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var (
			seqno, cid int
			name       sql.NullString
		)
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		out = append(out, name.String)
	}
	return out, rows.Err()
}
