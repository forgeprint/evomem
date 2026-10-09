package database

import (
	"strings"
	"testing"
)

// The SQLite path has to be the identity function, exactly. Every test in
// this package runs against SQLite, so if this rewrote anything the whole
// suite would be testing something other than what the code ships.
func TestSQLiteIsLeftAlone(t *testing.T) {
	for _, query := range []string{
		`SELECT id FROM notes WHERE project_id = ? AND source_type = ?`,
		`INSERT INTO notes (id, content) VALUES (?, ?)`,
		`SELECT 1`,
		`UPDATE notes SET metadata = ? WHERE id = ? AND project_id = ?`,
	} {
		if got := rewritePlaceholders(query, dialectSQLite); got != query {
			t.Errorf("sqlite rewrote a query:\n  %s\n  %s", query, got)
		}
	}
}

func TestPostgresNumbersThePlaceholders(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"none": {
			`SELECT COUNT(*) FROM notes`,
			`SELECT COUNT(*) FROM notes`,
		},
		"one": {
			`DELETE FROM notes WHERE id = ?`,
			`DELETE FROM notes WHERE id = $1`,
		},
		"several": {
			`INSERT INTO notes (id, project_id, content) VALUES (?, ?, ?)`,
			`INSERT INTO notes (id, project_id, content) VALUES ($1, $2, $3)`,
		},
		"past nine": {
			`VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := rewritePlaceholders(c.in, dialectPostgres); got != c.want {
				t.Errorf("\n got: %s\nwant: %s", got, c.want)
			}
		})
	}
}

// No query in this package has a `?` inside a string literal today. The point
// is the one somebody writes next: a rewriter that renumbers a question mark
// in a LIKE pattern or a JSON path corrupts the query silently, and the
// failure arrives as wrong results rather than as an error.
func TestPostgresLeavesStringLiteralsAlone(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"only in a literal": {
			`SELECT ? WHERE name = 'who?'`,
			`SELECT $1 WHERE name = 'who?'`,
		},
		"around a literal": {
			`SELECT ? FROM t WHERE a = 'x?y' AND b = ?`,
			`SELECT $1 FROM t WHERE a = 'x?y' AND b = $2`,
		},
		"doubled quote inside": {
			// '' is SQL's escape for a quote, so this is one
			// string containing it''s? — the ? stays untouched.
			`SELECT ? WHERE a = 'it''s? fine' AND b = ?`,
			`SELECT $1 WHERE a = 'it''s? fine' AND b = $2`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := rewritePlaceholders(c.in, dialectPostgres); got != c.want {
				t.Errorf("\n got: %s\nwant: %s", got, c.want)
			}
		})
	}
}

// The numbering has to follow the argument order the caller passes, which is
// left to right. A query whose placeholders came back out of order would bind
// the wrong value to the wrong column and still run.
func TestPostgresNumbersInOrder(t *testing.T) {
	got := rewritePlaceholders(
		`UPDATE notes SET content = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
		dialectPostgres)
	want := `UPDATE notes SET content = $1, updated_at = $2 WHERE id = $3 AND project_id = $4`
	if got != want {
		t.Fatalf("\n got: %s\nwant: %s", got, want)
	}
	if strings.Count(got, "$") != 4 {
		t.Fatalf("wrong number of placeholders: %s", got)
	}
}

func TestDialectNames(t *testing.T) {
	if dialectSQLite.String() != "sqlite" || dialectPostgres.String() != "postgres" {
		t.Fatal("a dialect that cannot name itself is one nobody can report")
	}
}
