package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	_ "modernc.org/sqlite"             // registers the "sqlite" driver
)

// Column is one column of a table, in the shape Strapi stores in
// strapi_database_schema: a knex column type, its arguments and constraints.
type Column struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Args        []any  `json:"args"`
	DefaultTo   any    `json:"defaultTo"`
	NotNullable bool   `json:"notNullable"`
	Unsigned    bool   `json:"unsigned"`
}

// Dialect is what differs between databases.
type Dialect interface {
	Name() string
	Quote(identifier string) string
	Placeholder(n int) string
	// ColumnType is the SQL type of a column, as knex creates it.
	ColumnType(c Column) (string, error)
	SupportsReturning() bool
	// MaxParams is the bind parameter limit of one statement.
	MaxParams() int
	open(cfg Config) (write, read *sql.DB, err error)
}

var dialects = map[string]Dialect{
	"postgres": postgres{},
	"sqlite":   sqlite{},
}

type postgres struct{}

func (postgres) Name() string             { return "postgres" }
func (postgres) Quote(id string) string   { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }
func (postgres) Placeholder(n int) string { return "$" + strconv.Itoa(n) }
func (postgres) SupportsReturning() bool  { return true }
func (postgres) MaxParams() int           { return 65535 }

func (postgres) ColumnType(c Column) (string, error) {
	switch c.Type {
	case "increments":
		return "serial", nil
	case "string":
		return "varchar(255)", nil
	case "text":
		return "text", nil
	case "integer":
		return "integer", nil
	case "bigInteger":
		return "bigint", nil
	case "double", "float":
		return "double precision", nil
	case "decimal":
		p, s := decimalArgs(c.Args)
		return fmt.Sprintf("decimal(%d, %d)", p, s), nil
	case "boolean":
		return "boolean", nil
	case "date":
		return "date", nil
	case "time":
		return fmt.Sprintf("time(%d)", precision(c.Args, 3)), nil
	case "datetime", "timestamp":
		tz := "without"
		if useTz(c.Args) {
			tz = "with"
		}
		return fmt.Sprintf("timestamp(%d) %s time zone", precision(c.Args, 6), tz), nil
	case "jsonb":
		return "jsonb", nil
	case "json":
		return "json", nil
	}
	return "", fmt.Errorf("postgres: unsupported column type %q", c.Type)
}

func (postgres) open(cfg Config) (*sql.DB, *sql.DB, error) {
	dsn := cfg.ConnectionString
	if dsn == "" {
		u := url.URL{Scheme: "postgres", Host: fmt.Sprintf("%s:%d", cfg.Host, orDefault(cfg.Port, 5432)), Path: "/" + cfg.Database}
		if cfg.User != "" {
			u.User = url.UserPassword(cfg.User, cfg.Password)
		}
		q := url.Values{}
		q.Set("sslmode", orDefaultString(cfg.SSLMode, "disable"))
		if cfg.Schema != "" {
			q.Set("search_path", cfg.Schema)
		}
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	pool.SetMaxOpenConns(orDefault(cfg.PoolMax, 10))
	pool.SetMaxIdleConns(orDefault(cfg.PoolMin, 2))
	return pool, pool, nil
}

type sqlite struct{}

func (sqlite) Name() string            { return "sqlite" }
func (sqlite) Quote(id string) string  { return "`" + strings.ReplaceAll(id, "`", "``") + "`" }
func (sqlite) Placeholder(int) string  { return "?" }
func (sqlite) SupportsReturning() bool { return true }
func (sqlite) MaxParams() int          { return 32766 }

// SQLite types follow what knex's sqlite3 client writes (see the physical
// schema dumps in compat/golden): decimals become float there.
func (sqlite) ColumnType(c Column) (string, error) {
	switch c.Type {
	case "increments":
		return "integer", nil
	case "string":
		return "varchar(255)", nil
	case "text":
		return "text", nil
	case "integer":
		return "integer", nil
	case "bigInteger":
		return "bigint", nil
	case "double", "float", "decimal":
		return "float", nil
	case "boolean":
		return "boolean", nil
	case "date":
		return "date", nil
	case "time":
		return "time", nil
	case "datetime", "timestamp":
		return "datetime", nil
	case "jsonb", "json":
		return "json", nil
	}
	return "", fmt.Errorf("sqlite: unsupported column type %q", c.Type)
}

func (sqlite) open(cfg Config) (*sql.DB, *sql.DB, error) {
	file := cfg.Filename
	if file == "" {
		file = ".tmp/data.db"
	}
	params := "_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	dsn := "file:" + file + "?" + params
	// One writer: concurrent writers in WAL mode would get "database is locked".
	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, err
	}
	write.SetMaxOpenConns(1)
	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = write.Close()
		return nil, nil, err
	}
	read.SetMaxOpenConns(orDefault(cfg.PoolMax, 4))
	if cfg.AcquireTimeout > 0 {
		write.SetConnMaxIdleTime(cfg.AcquireTimeout)
	}
	return write, read, nil
}

func decimalArgs(args []any) (int, int) {
	p, s := 10, 2
	if len(args) >= 2 {
		p, s = toInt(args[0], p), toInt(args[1], s)
	}
	return p, s
}

func precision(args []any, def int) int {
	if len(args) > 0 {
		if m, ok := args[0].(map[string]any); ok {
			return toInt(m["precision"], def)
		}
	}
	return def
}

func useTz(args []any) bool {
	if len(args) > 0 {
		if m, ok := args[0].(map[string]any); ok {
			b, _ := m["useTz"].(bool)
			return b
		}
	}
	return false
}

func toInt(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return def
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func orDefaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
