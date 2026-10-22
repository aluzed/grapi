package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type txKey struct{}

type txState struct {
	db         *DB
	tx         *sql.Tx
	onCommit   []func()
	onRollback []func()
}

func txFrom(ctx context.Context) *txState {
	t, _ := ctx.Value(txKey{}).(*txState)
	return t
}

// InTx reports whether ctx carries a transaction of db.
func (db *DB) InTx(ctx context.Context) bool {
	t := txFrom(ctx)
	return t != nil && t.db == db
}

// Tx runs fn in a transaction. When ctx already carries one, fn joins it: like
// Strapi, nesting does not create savepoints, the outermost call commits.
func (db *DB) Tx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if db.InTx(ctx) {
		return fn(ctx)
	}
	tx, err := db.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	state := &txState{db: db, tx: tx}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			runAll(state.onRollback)
			panic(p)
		}
	}()
	if err = fn(context.WithValue(ctx, txKey{}, state)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, rbErr)
		}
		runAll(state.onRollback)
		return err
	}
	if err = tx.Commit(); err != nil {
		runAll(state.onRollback)
		return fmt.Errorf("commit: %w", err)
	}
	runAll(state.onCommit)
	return nil
}

// OnCommit runs fn after the outermost transaction of ctx commits, or right
// away when ctx has no transaction.
func OnCommit(ctx context.Context, fn func()) {
	if t := txFrom(ctx); t != nil {
		t.onCommit = append(t.onCommit, fn)
		return
	}
	fn()
}

// OnRollback runs fn if the outermost transaction of ctx rolls back.
func OnRollback(ctx context.Context, fn func()) {
	if t := txFrom(ctx); t != nil {
		t.onRollback = append(t.onRollback, fn)
	}
}

func runAll(fns []func()) {
	for _, fn := range fns {
		fn()
	}
}
