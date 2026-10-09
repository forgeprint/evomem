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
// reachable without naming it again. The three methods below shadow the
// promoted ones on purpose: those are exactly the ones that carry a query.
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
