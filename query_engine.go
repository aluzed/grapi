package grapi

import (
	"context"
	"database/sql"

	"github.com/aluzed/grapi/internal/db"
	iquery "github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/query"
)

// Query is strapi.db.query(uid): the Query Engine for one model. It works
// on rows by id, under the Document Service: no draft and publish or locale
// logic, the row lifecycles run.
type Query struct {
	e   *iquery.Engine
	uid string
}

// Query is the Query Engine for uid, available once Setup ran.
func (a *App) Query(uid string) *Query {
	if a.docs == nil {
		return nil
	}
	return &Query{e: a.docs.E, uid: uid}
}

// FindOne is findOne; nil when nothing matches.
func (q *Query) FindOne(ctx context.Context, p query.Params) (Entry, error) {
	return q.e.FindOne(ctx, q.uid, p)
}

// FindMany is findMany.
func (q *Query) FindMany(ctx context.Context, p query.Params) ([]Entry, error) {
	return q.e.FindMany(ctx, q.uid, p)
}

// Count is count.
func (q *Query) Count(ctx context.Context, p query.Params) (int64, error) {
	return q.e.Count(ctx, q.uid, p)
}

// Create is create({ data }).
func (q *Query) Create(ctx context.Context, data query.Data, p query.Params) (Entry, error) {
	return q.e.Create(ctx, q.uid, data, p)
}

// CreateMany is createMany({ data }): the count and the ids.
func (q *Query) CreateMany(ctx context.Context, data []query.Data) (int, []any, error) {
	return q.e.CreateMany(ctx, q.uid, data)
}

// Update is update({ where, data }); nil when nothing matches.
func (q *Query) Update(ctx context.Context, where any, data query.Data, p query.Params) (Entry, error) {
	return q.e.Update(ctx, q.uid, where, data, p)
}

// UpdateMany is updateMany({ where, data }): the count.
func (q *Query) UpdateMany(ctx context.Context, p query.Params, data query.Data) (int64, error) {
	return q.e.UpdateMany(ctx, q.uid, p, data)
}

// Delete is delete({ where }); nil when nothing matches.
func (q *Query) Delete(ctx context.Context, where any, p query.Params) (Entry, error) {
	return q.e.Delete(ctx, q.uid, where, p)
}

// DeleteMany is deleteMany({ where }): the count.
func (q *Query) DeleteMany(ctx context.Context, p query.Params) (int64, error) {
	return q.e.DeleteMany(ctx, q.uid, p)
}

// SQL is strapi.db.connection for raw statements. It runs in the
// transaction of ctx when there is one, so a hook does not wait on its own
// write lock.
type SQL struct{ db *db.DB }

// SQL is the raw database access, available once Setup ran.
func (a *App) SQL() *SQL {
	if a.docs == nil {
		return nil
	}
	return &SQL{a.docs.E.DB}
}

// Exec runs a statement.
func (s *SQL) Exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.db.Exec(ctx, q, args...)
}

// Query runs a query.
func (s *SQL) Query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.db.Query(ctx, q, args...)
}

// QueryRow runs a single-row query.
func (s *SQL) QueryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.db.QueryRow(ctx, q, args...)
}

// Dialect is the database client: sqlite, postgres or mysql.
func (s *SQL) Dialect() string { return s.db.Dialect.Name() }

// Placeholder is the n-th bind parameter of the dialect, from 1: ? or $n.
func (s *SQL) Placeholder(n int) string { return s.db.Dialect.Placeholder(n) }

// Quote quotes an identifier for the dialect.
func (s *SQL) Quote(name string) string { return s.db.Dialect.Quote(name) }

// Tx is strapi.db.transaction: fn runs in a transaction, committed when it
// returns nil. The Document Service, the Query Engine and SQL calls made
// with the ctx fn gets join it.
func (s *SQL) Tx(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.db.Tx(ctx, fn)
}
