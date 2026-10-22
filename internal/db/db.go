// Package db is grapi's database layer: dialects, connections and transactions
// carried by context. Sub-packages build names, schemas and queries.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Config is config/database's "connection" block.
type Config struct {
	Client           string // postgres, sqlite or mysql
	ConnectionString string
	Host             string
	Port             int
	Database         string
	User             string
	Password         string
	SSLMode          string
	Schema           string
	Filename         string // sqlite
	PoolMin          int
	PoolMax          int
	AcquireTimeout   time.Duration
	Debug            bool
}

// DB is an open database: a dialect and its connection pools.
type DB struct {
	Dialect Dialect
	write   *sql.DB
	read    *sql.DB
	log     *slog.Logger
	debug   bool
}

// Open connects according to cfg and checks the connection.
func Open(ctx context.Context, cfg Config, log *slog.Logger) (*DB, error) {
	if log == nil {
		log = slog.Default()
	}
	d, ok := dialects[cfg.Client]
	if !ok {
		return nil, fmt.Errorf("unsupported database client %q (postgres, sqlite)", cfg.Client)
	}
	write, read, err := d.open(cfg)
	if err != nil {
		return nil, err
	}
	db := &DB{Dialect: d, write: write, read: read, log: log, debug: cfg.Debug}
	if err := write.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to %s: %w", cfg.Client, err)
	}
	return db, nil
}

// Close releases the pools.
func (db *DB) Close() error {
	err := db.write.Close()
	if db.read != db.write {
		if e := db.read.Close(); err == nil {
			err = e
		}
	}
	return err
}

// Querier is what queries run on: a pool or the transaction found in ctx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Exec runs a statement in ctx's transaction, or on the write pool.
func (db *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := db.querier(ctx, true).ExecContext(ctx, query, args...)
	db.trace(ctx, query, args, start, err)
	return res, err
}

// Query runs a query in ctx's transaction, or on the read pool.
func (db *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.querier(ctx, false).QueryContext(ctx, query, args...)
	db.trace(ctx, query, args, start, err)
	return rows, err
}

// QueryRow runs a single-row query in ctx's transaction, or on the read pool.
func (db *DB) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := db.querier(ctx, false).QueryRowContext(ctx, query, args...)
	db.trace(ctx, query, args, start, row.Err())
	return row
}

func (db *DB) querier(ctx context.Context, write bool) Querier {
	if t := txFrom(ctx); t != nil && t.db == db {
		return t.tx
	}
	if write {
		return db.write
	}
	return db.read
}

func (db *DB) trace(ctx context.Context, query string, args []any, start time.Time, err error) {
	if !db.debug && err == nil {
		return
	}
	level := slog.LevelDebug
	if err != nil {
		level = slog.LevelWarn
	}
	db.log.Log(ctx, level, "sql", "query", query, "args", len(args), "duration", time.Since(start), "error", err)
}
