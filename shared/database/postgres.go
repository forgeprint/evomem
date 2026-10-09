package database

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"

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
		// Not a file. Reported where the SQLite path would be, because
		// every message that says where the store is has to say
		// something, and a password does not belong in one.
		path: "postgres",
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
