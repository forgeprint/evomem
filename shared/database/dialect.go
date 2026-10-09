package database

import (
	"strconv"
	"strings"
)

// A dialect is which SQL the store on the other end speaks.
//
// The first step of ADR-0028. The server's store is moving to PostgreSQL and
// the phone's stays SQLite, so this package has to speak both; the backend is
// chosen at runtime by whether a DSN was configured.
//
// Nothing here makes PostgreSQL work. It makes the difference expressible,
// and the places that differ — placeholders first, the search index and the
// metadata predicates later — name the dialect rather than assume one.
type dialect int

const (
	// dialectSQLite is the file on disk: the phone's store, the command
	// line's, and what every test runs against so `scripts/ci.sh` keeps
	// working with no container and no network.
	dialectSQLite dialect = iota

	// dialectPostgres is the server's store.
	dialectPostgres
)

func (d dialect) String() string {
	if d == dialectPostgres {
		return "postgres"
	}
	return "sqlite"
}

// rewritePlaceholders turns the `?` this package writes into whatever the
// dialect wants.
//
// Every query in this package is written with `?` because that is what SQLite
// takes and what the existing code already says. PostgreSQL wants `$1`, `$2`
// — numbered, so the rewriting has to count rather than substitute.
//
// Doing it here rather than writing each query twice is the decision: a
// placeholder is the one difference that touches every single statement, and
// two copies of fifty queries differing only in their `?` is fifty chances to
// change one and forget the other.
//
// A `?` inside a string literal is left alone. No query here has one today;
// the rewriter that silently corrupts the first one somebody writes is worse
// than the four lines this costs.
func rewritePlaceholders(query string, d dialect) string {
	if d != dialectPostgres {
		return query
	}
	if !strings.ContainsRune(query, '?') {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 8)

	n := 0
	inString := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case c == '\'':
			// SQL escapes a quote by doubling it, so a '' inside a
			// string is two flips and ends up back inside, which
			// is the right answer.
			inString = !inString
			b.WriteByte(c)
		case c == '?' && !inString:
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
