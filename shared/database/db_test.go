package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenCreatesSchema(t *testing.T) {
	db := openTemp(t)

	version, err := db.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("schema version is %d, want %d", version, schemaVersion)
	}
}

// WAL is what allows a read while an ingestion write is in flight. It is set
// in the DSN, so a change there would silently drop it.
func TestOpenUsesWAL(t *testing.T) {
	skipOnPostgres(t, "WAL is a SQLite journal mode; PostgreSQL has its own write-ahead log and no PRAGMA")
	db := openTemp(t)

	var mode string
	if err := db.write.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("the write pool is in %q, want wal", mode)
	}

	if err := db.read.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("the read pool is in %q, want wal", mode)
	}
}

// The single writer pattern is a cap on the pool, not a comment. Losing it
// would not fail any other test; it would just make concurrent ingestion
// flaky in production.
func TestWritePoolIsSingleConnection(t *testing.T) {
	skipOnPostgres(t, "ADR-0004 caps the writer because a second one gets SQLITE_BUSY; a PostgreSQL server handles concurrency itself")
	db := openTemp(t)

	if got := db.write.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("write pool allows %d connections, want 1", got)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("an empty path was accepted")
	}
}

func TestReopenKeepsNotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evomem.db")

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	n := newNote("evomem", "kalici olmali")
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()

	if _, err := again.Get(context.Background(), n.ID); err != nil {
		t.Fatalf("the note did not survive a reopen: %v", err)
	}
}

func TestOpenMemory(t *testing.T) {
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	n := newNote("evomem", "bellekte")
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	// Written through the write pool, read back through the read pool:
	// the two have to be looking at the same database.
	if _, err := db.Get(context.Background(), n.ID); err != nil {
		t.Fatal(err)
	}
}

// Two in-memory stores must not be the same store, or parallel tests would
// see each other's notes.
func TestOpenMemoryIsolated(t *testing.T) {
	a, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	n := newNote("evomem", "sadece a")
	if err := a.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get(context.Background(), n.ID); err == nil {
		t.Error("a note written to one in-memory store was visible in another")
	}
}
