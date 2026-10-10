package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
)

// postgresDriver is what pgx registers with database/sql. The same driver
// core/sync already uses, so this adds no dependency (ADR-0002).
const postgresDriver = "pgx"

// OpenPostgres opens the store on a PostgreSQL server (ADR-0028).
//
// The other half of Open. Which one a process uses is decided by whether a
// DSN was configured: the server in Docker has one, a laptop running
// `evomem mcp` does not.
//
// Unlike the SQLite side there is no single-writer pool here. ADR-0004 caps
// SQLite's writer at one connection because a second writer gets SQLITE_BUSY;
// a PostgreSQL server handles concurrency itself, and one connection to it
// would be a bottleneck invented for a problem this database does not have.
// The two pools stay — read and write are still separate — but both are
// ordinary.
func OpenPostgres(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database: no PostgreSQL DSN given")
	}

	write, err := openPostgresPool(ctx, dsn)
	if err != nil {
		return nil, err
	}
	read, err := openPostgresPool(ctx, dsn)
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(max(4, runtime.NumCPU()))

	db := &DB{
		write: pool{DB: write, dialect: dialectPostgres},
		read:  pool{DB: read, dialect: dialectPostgres},
		// Not a file, and never the DSN: that carries a password. The
		// host and database are what somebody needs to tell one store
		// from another, and they are not secret.
		path: describePostgres(dsn),
	}
	if err := db.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openPostgresPool(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open(postgresDriver, dsn)
	if err != nil {
		// The DSN carries a password, so it is never in the message.
		return nil, fmt.Errorf("database: opening PostgreSQL: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database: opening PostgreSQL: %w", err)
	}
	return db, nil
}

// describePostgres says which server and database a DSN points at, without
// the credentials.
//
// Printed where a file path would be. The point is that a process can say
// which memory it opened: pointing `evomem mcp` at a laptop's file while the
// server writes to PostgreSQL is a silent split, and an agent reading an
// empty store looks exactly like a person who has written nothing.
func describePostgres(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Host == "" {
		// A keyword/value DSN, or something unparseable. Saying which
		// kind of store it is beats guessing at its shape.
		return "postgres"
	}
	return "postgres://" + parsed.Host + strings.TrimSuffix(parsed.Path, "/")
}
