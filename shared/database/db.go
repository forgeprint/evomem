// Package database is Evomem's local store: one SQLite file, opened with a
// pure-Go driver so the binary needs no C toolchain and no runtime.
package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite" // pure Go, no cgo
)

// DB is a handle on the store. It is two connection pools over one file, and
// that split is the whole reason this type exists.
//
// SQLite in WAL mode allows any number of readers alongside one writer, but not
// two writers: a second one gets SQLITE_BUSY. Evomem writes from several
// places at once — a Telegram webhook, an Apple Shortcut, an MCP client — so
// leaving that to chance means intermittent failures under exactly the load
// the project is for. The write pool is capped at a single connection, which
// turns contention into waiting inside the process instead of an error coming
// back out of it.
type DB struct {
	write *sql.DB
	read  *sql.DB
	path  string

	// files removes a note's recording when the note goes. Nil when
	// nothing in this process keeps recordings, which is every caller
	// that only reads. See SetFiles.
	files Files
}

// Files is where a note's recording is kept. shared/audio implements it; this
// package takes an interface so that storage stays out of the schema.
type Files interface {
	// Remove deletes one note's recording. A note with none is not an
	// error: most notes are text.
	Remove(noteID string) error

	// RemoveAll deletes every recording, which is what replacing the
	// whole store needs.
	RemoveAll() error
}

// SetFiles says where recordings are kept, so that deleting a note deletes
// its recording too.
//
// Without it a delete leaves the file behind. That is the right default for a
// process that only reads, and the wrong one for anything that deletes, which
// is why ADR-0018 makes removing the file a condition of keeping it at all:
// content a person deleted must not survive on disk.
func (d *DB) SetFiles(f Files) { d.files = f }

// removeRecording is called after the rows are committed, never inside the
// transaction. A filesystem does not roll back with SQLite, and a file
// removed for a delete that then failed would be the worse of the two
// mistakes.
func (d *DB) removeRecording(ids ...string) error {
	if d.files == nil {
		return nil
	}
	for _, id := range ids {
		if err := d.files.Remove(id); err != nil {
			// Reported, not swallowed. The row is gone and the
			// recording is not, which is exactly the state
			// ADR-0018 says must never pass unnoticed.
			return fmt.Errorf("database: the note is deleted but its recording is not: %w", err)
		}
	}
	return nil
}

// busyTimeoutMS is how long SQLite waits for a lock before giving up. The
// single writer pool makes in-process contention wait rather than fail; this
// covers the other case, another process holding the file.
const busyTimeoutMS = 5000

// Open opens the store at path, creating the file and the schema if they are
// not there yet.
func Open(path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database: no path given")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("database: creating %s: %w", dir, err)
		}
	}

	// _txlock=immediate makes a write transaction take its lock when it
	// begins. Without it SQLite starts deferred and upgrades on the first
	// write, and an upgrade that loses the race cannot be retried safely
	// because the transaction has already read.
	write, err := openPool(path, "immediate")
	if err != nil {
		return nil, err
	}
	// One connection, so two writers inside this process queue instead of
	// colliding.
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)

	read, err := openPool(path, "")
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(max(4, runtime.NumCPU()))

	db := &DB{write: write, read: read, path: path}
	if err := db.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// OpenMemory opens a private in-memory store. Tests use it; nothing else
// should, because it is gone when the handle closes.
//
// The two pools share one database through the shared cache, and the single
// writer stays single: an in-memory database has no file for a second process
// to contend over, but the rest of the code must behave identically here or
// the tests prove nothing.
func OpenMemory() (*DB, error) {
	dsn := "file:evomem-" + memoryName() + "?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

	write, err := sql.Open("sqlite", dsn+"&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)
	// A shared-cache memory database lives as long as one connection to it
	// does, so the writer's single idle connection is what keeps it alive.
	if err := write.Ping(); err != nil {
		write.Close()
		return nil, err
	}

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(4)

	db := &DB{write: write, read: read, path: ":memory:"}
	if err := db.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// openPool opens one pool over the file. txlock may be empty for readers.
func openPool(path, txlock string) (*sql.DB, error) {
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMS))
	// WAL is what lets readers run while a write is in flight, and it
	// survives in the file, so setting it on every open is a no-op after
	// the first.
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	// NORMAL is the documented companion to WAL: a crash cannot corrupt
	// the database, only lose the last commits, and that trade is worth
	// the order of magnitude it buys on ingestion.
	q.Add("_pragma", "synchronous(NORMAL)")
	if txlock != "" {
		q.Set("_txlock", txlock)
	}

	dsn := "file:" + filepath.ToSlash(path) + "?" + q.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: opening %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database: opening %s: %w", path, err)
	}
	return db, nil
}

// Path returns the file the store was opened from.
func (d *DB) Path() string { return d.path }

// Close releases both pools.
func (d *DB) Close() error {
	readErr := d.read.Close()
	writeErr := d.write.Close()
	if writeErr != nil {
		return writeErr
	}
	return readErr
}

// memoryName gives each in-memory store its own name, so two tests running in
// parallel do not land in the same shared-cache database.
func memoryName() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("database: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
