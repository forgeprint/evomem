package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Nothing outside pool.go may reach past the wrapper to the handle it
// embeds.
//
// This is a source scan rather than a behavioural test because there is no
// behaviour to observe: SQLite accepts `$1` as a parameter name and binds it
// positionally, so a query rewritten for PostgreSQL runs correctly against
// SQLite anyway. Verified, not assumed — which means the suite cannot tell a
// wired rewriter from an unwired one, and the thing actually worth guarding
// is that a new call site does not quietly skip it.
//
// `d.write.ExecContext(…)` goes through the wrapper. `d.write.DB.ExecContext(…)`
// does not, and on PostgreSQL would send a `?` the server cannot parse. The
// same for `tx.Tx.ExecContext`.
func TestNothingBypassesTheRewriting(t *testing.T) {
	// The embedded handles, named explicitly. Close, Ping and
	// SetMaxOpenConns carry no query and are not here.
	bypasses := []string{
		".DB.ExecContext", ".DB.QueryContext", ".DB.QueryRowContext",
		".DB.Exec", ".DB.Query", ".DB.QueryRow", ".DB.Prepare", ".DB.PrepareContext",
		".Tx.ExecContext", ".Tx.QueryContext", ".Tx.QueryRowContext",
		".Tx.Exec", ".Tx.Query", ".Tx.QueryRow", ".Tx.Prepare", ".Tx.PrepareContext",
	}

	all, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, file := range all {
		base := filepath.Base(file)
		// pool.go is where the wrapper delegates, so it is the one
		// place these are correct. Test files are out of scope:
		// production code is what ships, and this file holds the list
		// of needles and would match itself.
		if base == "pool.go" || strings.HasSuffix(base, "_test.go") {
			continue
		}
		files = append(files, file)
	}
	if len(files) < 5 {
		t.Fatalf("found %d files to scan; this test is looking in the wrong place", len(files))
	}

	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, bypass := range bypasses {
			if strings.Contains(string(source), bypass) {
				t.Errorf("%s uses %s, which skips the placeholder rewriting (ADR-0028). "+
					"Call it on the pool or the txn instead.", file, bypass)
			}
		}
	}
}

// And the test above only means anything if it can fail.
func TestTheBypassScanWouldCatchOne(t *testing.T) {
	const bypass = ".DB.ExecContext"
	if !strings.Contains(`	p.DB.ExecContext(ctx, query)`, bypass) {
		t.Fatal("the needle does not match the shape it is looking for")
	}
	if strings.Contains(`	p.ExecContext(ctx, query)`, bypass) {
		t.Fatal("the scan would flag a correct call site")
	}
}
