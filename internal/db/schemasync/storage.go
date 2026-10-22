package schemasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
)

const schemaTable = "strapi_database_schema"

// Stored is the schema saved by the last sync.
type Stored struct {
	Schema *metadata.Schema
	Hash   string
}

func hasTable(ctx context.Context, d *db.DB, name string) (bool, error) {
	var query string
	if d.Dialect.Name() == "postgres" {
		query = `select count(*) from information_schema.tables where table_schema = current_schema() and table_name = $1`
	} else {
		query = `select count(*) from sqlite_master where type = 'table' and name = ?`
	}
	var n int
	if err := d.QueryRow(ctx, query, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func ensureSchemaTable(ctx context.Context, d *db.DB) error {
	ok, err := hasTable(ctx, d, schemaTable)
	if err != nil || ok {
		return err
	}
	var stmt string
	if d.Dialect.Name() == "postgres" {
		stmt = `create table "strapi_database_schema" ("id" serial primary key, "schema" json, "time" timestamp, "hash" varchar(255))`
	} else {
		stmt = "create table `strapi_database_schema` (`id` integer not null primary key autoincrement, `schema` json, `time` datetime, `hash` varchar(255))"
	}
	_, err = d.Exec(ctx, stmt)
	return err
}

// ReadStored returns the last stored schema, or nil when there is none.
func ReadStored(ctx context.Context, d *db.DB) (*Stored, error) {
	if err := ensureSchemaTable(ctx, d); err != nil {
		return nil, err
	}
	var (
		raw  string
		hash sql.NullString
	)
	err := d.QueryRow(ctx, `select "schema", "hash" from "strapi_database_schema" order by "time" desc limit 1`).Scan(&raw, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var head struct {
		Tables []json.RawMessage `json:"tables"`
	}
	if err := json.Unmarshal([]byte(raw), &head); err != nil {
		return nil, err
	}
	s := &metadata.Schema{Tables: []*metadata.Table{}}
	for _, t := range head.Tables {
		table, err := metadata.ParseTable(t)
		if err != nil {
			return nil, err
		}
		s.Tables = append(s.Tables, table)
	}
	return &Stored{Schema: s, Hash: hash.String}, nil
}

// Store replaces the stored schema with s.
func Store(ctx context.Context, d *db.DB, s *metadata.Schema) error {
	if err := ensureSchemaTable(ctx, d); err != nil {
		return err
	}
	b, err := metadata.Marshal(s)
	if err != nil {
		return err
	}
	hash, err := Hash(s)
	if err != nil {
		return err
	}
	return d.Tx(ctx, func(ctx context.Context) error {
		if _, err := d.Exec(ctx, `delete from "strapi_database_schema"`); err != nil {
			return err
		}
		_, err := d.Exec(ctx, `insert into "strapi_database_schema" ("schema", "hash", "time") values (`+
			d.Dialect.Placeholder(1)+`, `+d.Dialect.Placeholder(2)+`, `+d.Dialect.Placeholder(3)+`)`,
			string(b), hash, timeValue(d, time.Now()))
		return err
	})
}

// timeValue is how Strapi stores a datetime: epoch milliseconds on SQLite.
func timeValue(d *db.DB, t time.Time) any {
	if d.Dialect.Name() == "sqlite" {
		return t.UnixMilli()
	}
	return t.UTC()
}

// persistedTables reads the core store's persisted_tables: names, or
// {name, dependsOn: [{name}]} objects.
func persistedTables(ctx context.Context, d *db.DB) ([]PersistedTable, error) {
	ok, err := hasTable(ctx, d, "strapi_core_store_settings")
	if err != nil || !ok {
		return nil, err
	}
	var raw sql.NullString
	err = d.QueryRow(ctx, `select "value" from "strapi_core_store_settings" where "key" = 'core_persisted_tables' and "environment" is null and "tag" is null`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || !raw.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []json.RawMessage
	if json.Unmarshal([]byte(raw.String), &entries) != nil {
		return nil, nil
	}
	out := make([]PersistedTable, 0, len(entries))
	for _, e := range entries {
		var name string
		if json.Unmarshal(e, &name) == nil {
			out = append(out, PersistedTable{Name: name})
			continue
		}
		var obj struct {
			Name      string `json:"name"`
			DependsOn []struct {
				Name string `json:"name"`
			} `json:"dependsOn"`
		}
		if json.Unmarshal(e, &obj) != nil {
			continue
		}
		p := PersistedTable{Name: obj.Name}
		for _, dep := range obj.DependsOn {
			p.DependsOn = append(p.DependsOn, dep.Name)
		}
		out = append(out, p)
	}
	return out, nil
}
