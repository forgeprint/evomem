package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

// testPostgresDSN is what makes this whole suite run against PostgreSQL
// instead of a file.
//
// One switch rather than a second copy of every test: the store has to behave
// the same on both backends (ADR-0028), and the only way to know that is to
// run the same assertions against each. `go test ./shared/database/` on its
// own still runs offline against SQLite, which is what `scripts/ci.sh`
// needs; `scripts/test-postgres.sh` sets this and runs it again.
const testPostgresDSN = "EVOMEM_TEST_POSTGRES_DSN"

// onPostgres reports which backend this run is exercising, for the handful of
// assertions that are genuinely about one of them.
func onPostgres() bool { return os.Getenv(testPostgresDSN) != "" }

// skipUntilPostgresSearch marks a test that cannot pass yet because the
// full-text search is still SQLite's FTS5 and PostgreSQL's tsvector has not
// been written (ADR-0028, step 3).
//
// A skip that names what it is waiting for, so it reads as a known gap rather
// than as a test somebody switched off.
func skipUntilPostgresSearch(t *testing.T) {
	t.Helper()
	if onPostgres() {
		t.Skip("search is still FTS5-only; PostgreSQL's tsvector is ADR-0028 step 3")
	}
}

// openTemp opens a store for a test.
//
// With no DSN it is a file in a temp directory: the WAL and the single-writer
// pool this package is built around only exist for a file, so the tests that
// care about them must use one.
//
// With a DSN it is PostgreSQL, in a schema of this test's own. A schema per
// test rather than a database per test because creating one is cheap and
// dropping it takes everything in it; `go test` runs these in one process, so
// sharing public would have every test see every other test's rows.
func openTemp(t *testing.T) *DB {
	t.Helper()

	dsn := os.Getenv(testPostgresDSN)
	if dsn == "" {
		db, err := Open(filepath.Join(t.TempDir(), "evomem.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}

	// A fresh ULID is already a legal identifier apart from its first
	// character, which may be a digit.
	schema := "t" + models.NewULID()
	ctx := context.Background()

	admin, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("%s is set but the server did not answer: %v", testPostgresDSN, err)
	}
	if _, err := admin.write.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	// search_path in the DSN is what puts this test's tables in its own
	// schema without a single query having to name one.
	db, err := OpenPostgres(ctx, withSearchPath(dsn, schema))
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := admin.write.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Logf("could not drop %s: %v", schema, err)
		}
		admin.Close()
	})
	return db
}

// withSearchPath adds the schema to a DSN, in whichever of the two forms it
// is written.
func withSearchPath(dsn, schema string) string {
	sep := "?"
	if containsRune(dsn, '?') {
		sep = "&"
	}
	if !containsRune(dsn, ':') || !containsRune(dsn, '/') {
		// A keyword/value DSN ("host=… dbname=…") rather than a URL.
		return fmt.Sprintf("%s search_path=%s", dsn, schema)
	}
	return fmt.Sprintf("%s%ssearch_path=%s", dsn, sep, schema)
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// skipOnPostgres marks a test that asserts something true only of the SQLite
// backend, with the reason it does not apply.
//
// Not a gap, unlike skipUntilPostgresSearch: these are tests of decisions
// ADR-0004 and ADR-0001 made about a file on disk, and PostgreSQL does not
// have the problem they solve.
func skipOnPostgres(t *testing.T, why string) {
	t.Helper()
	if onPostgres() {
		t.Skip(why)
	}
}
