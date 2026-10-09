package database

import (
	"context"
	"database/sql"
)

// A pool is a connection pool that knows which SQL it is talking to.
//
// It exists so the fifty-odd call sites in this package do not have to. Each
// one still writes `?` and still calls ExecContext; the rewriting happens on
// the way out, once, here (ADR-0028).
//
// The embedded *sql.DB keeps everything else — Close, Ping, SetMaxOpenConns —
// reachable without naming it again. The methods below shadow the promoted
// ones on purpose, and they are *every* method that carries a query.
//
// Every one matters, not just the obvious three. A promoted method that is
// not shadowed here looks exactly like a correct call at the call site —
// `tx.PrepareContext(…)` reads the same whether or not it rewrites — so
// nothing in review or in the source scan can tell them apart. The first run
// against a real PostgreSQL found precisely that: CreateBatch prepared its
// statement through the promoted PrepareContext and sent a `?`.
type pool struct {
	*sql.DB
	dialect dialect
}

func (p pool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.DB.ExecContext(ctx, rewritePlaceholders(query, p.dialect), args...)
}

func (p pool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.DB.QueryContext(ctx, rewritePlaceholders(query, p.dialect), args...)
}

func (p pool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.DB.QueryRowContext(ctx, rewritePlaceholders(query, p.dialect), args...)
}

func (p pool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.DB.PrepareContext(ctx, rewritePlaceholders(query, p.dialect))
}

func (p pool) Exec(query string, args ...any) (sql.Result, error) {
	return p.DB.Exec(rewritePlaceholders(query, p.dialect), args...)
}

func (p pool) Query(query string, args ...any) (*sql.Rows, error) {
	return p.DB.Query(rewritePlaceholders(query, p.dialect), args...)
}

func (p pool) QueryRow(query string, args ...any) *sql.Row {
	return p.DB.QueryRow(rewritePlaceholders(query, p.dialect), args...)
}

func (p pool) Prepare(query string) (*sql.Stmt, error) {
	return p.DB.Prepare(rewritePlaceholders(query, p.dialect))
}

// BeginTx returns a transaction that rewrites the same way.
//
// It shadows the promoted BeginTx and returns *txn rather than *sql.Tx, so a
// statement inside a transaction cannot quietly skip the rewriting by using
// the handle it was given.
func (p pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (*txn, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &txn{Tx: tx, dialect: p.dialect}, nil
}

// A txn is a transaction that knows its dialect, for the same reason a pool
// does.
type txn struct {
	*sql.Tx
	dialect dialect
}

func (t *txn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, rewritePlaceholders(query, t.dialect), args...)
}

func (t *txn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, rewritePlaceholders(query, t.dialect), args...)
}

func (t *txn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, rewritePlaceholders(query, t.dialect), args...)
}

func (t *txn) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.Tx.PrepareContext(ctx, rewritePlaceholders(query, t.dialect))
}

func (t *txn) Exec(query string, args ...any) (sql.Result, error) {
	return t.Tx.Exec(rewritePlaceholders(query, t.dialect), args...)
}

func (t *txn) Query(query string, args ...any) (*sql.Rows, error) {
	return t.Tx.Query(rewritePlaceholders(query, t.dialect), args...)
}

func (t *txn) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(rewritePlaceholders(query, t.dialect), args...)
}

func (t *txn) Prepare(query string) (*sql.Stmt, error) {
	return t.Tx.Prepare(rewritePlaceholders(query, t.dialect))
}
