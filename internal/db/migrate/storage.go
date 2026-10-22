package migrate

import (
	"context"
	"fmt"
	"time"

	"github.com/aluzed/grapi/internal/db"
)

func tableExists(ctx context.Context, d *db.DB, name string) (bool, error) {
	query := `select count(*) from sqlite_master where type = 'table' and name = ?`
	if d.Dialect.Name() == "postgres" {
		query = `select count(*) from information_schema.tables where table_schema = current_schema() and table_name = $1`
	}
	var n int
	err := d.QueryRow(ctx, query, name).Scan(&n)
	return n > 0, err
}

func columnExists(ctx context.Context, d *db.DB, table, column string) (bool, error) {
	query := `select count(*) from pragma_table_info(?) where name = ?`
	if d.Dialect.Name() == "postgres" {
		query = `select count(*) from information_schema.columns where table_schema = current_schema() and table_name = $1 and column_name = $2`
	}
	var n int
	err := d.QueryRow(ctx, query, table, column).Scan(&n)
	return n > 0, err
}

// ensureTable creates a migrations table as Strapi does (knex DDL).
func ensureTable(ctx context.Context, d *db.DB, table string) error {
	ok, err := tableExists(ctx, d, table)
	if err != nil || ok {
		return err
	}
	stmt := fmt.Sprintf("create table `%s` (`id` integer not null primary key autoincrement, `name` varchar(255), `time` datetime)", table)
	if d.Dialect.Name() == "postgres" {
		stmt = fmt.Sprintf(`create table "%s" ("id" serial primary key, "name" varchar(255), "time" timestamp)`, table)
	}
	_, err = d.Exec(ctx, stmt)
	return err
}

// Executed returns the names recorded in a migrations table, oldest first.
func Executed(ctx context.Context, d *db.DB, table string) ([]string, error) {
	rows, err := d.Query(ctx, fmt.Sprintf(`select "name" from "%s" order by "time", "id"`, table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func logMigration(ctx context.Context, d *db.DB, table, name string) error {
	var now any = time.Now().UTC()
	if d.Dialect.Name() == "sqlite" {
		now = time.Now().UnixMilli()
	}
	_, err := d.Exec(ctx, fmt.Sprintf(`insert into "%s" ("name", "time") values (%s, %s)`,
		table, d.Dialect.Placeholder(1), d.Dialect.Placeholder(2)), name, now)
	return err
}

// isFresh reports whether no Strapi or grapi instance ever ran on the database.
func isFresh(ctx context.Context, d *db.DB) (bool, error) {
	for _, t := range []string{"strapi_database_schema", "strapi_core_store_settings", InternalTable} {
		ok, err := tableExists(ctx, d, t)
		if err != nil || ok {
			return false, err
		}
	}
	return true, nil
}

// checkNotStrapi4 refuses a Strapi 4 database: its admin users have no
// document_id, which Strapi 5's internal migrations add.
func checkNotStrapi4(ctx context.Context, d *db.DB) error {
	ok, err := tableExists(ctx, d, "admin_users")
	if err != nil || !ok {
		return err
	}
	ok, err = columnExists(ctx, d, "admin_users", "document_id")
	if err != nil {
		return err
	}
	if !ok {
		return ErrStrapi4
	}
	return nil
}

// lockKey is the Postgres advisory lock key of grapi's startup migrations.
const lockKey = 7_311_946_207

// WithLock runs fn while holding a database-wide lock, so that two instances
// starting together do not migrate the same database at once. On Postgres it
// is an advisory lock; SQLite serializes writers itself.
func WithLock(ctx context.Context, d *db.DB, fn func(ctx context.Context) error) error {
	if d.Dialect.Name() != "postgres" {
		return fn(ctx)
	}
	return d.WithConn(ctx, func(ctx context.Context, conn db.Conn) error {
		if _, err := conn.ExecContext(ctx, "select pg_advisory_lock($1)", lockKey); err != nil {
			return err
		}
		defer func() {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "select pg_advisory_unlock($1)", lockKey)
		}()
		return fn(ctx)
	})
}
