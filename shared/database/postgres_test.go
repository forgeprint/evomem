package database

import "testing"

// The point of naming the store is that two processes on one machine can
// open two different ones without saying so: `evomem mcp` on a laptop's
// file while the server writes to PostgreSQL is a silent split, and an
// agent reading an empty store looks exactly like somebody who has written
// nothing.
func TestAPostgresStoreNamesItselfWithoutTheCredentials(t *testing.T) {
	for name, c := range map[string]struct{ dsn, want string }{
		"with a password": {
			"postgres://evomem:hunter2@db:5432/evomem?sslmode=disable",
			"postgres://db:5432/evomem",
		},
		"loopback": {
			"postgres://evomem:pw@127.0.0.1:5432/evomem",
			"postgres://127.0.0.1:5432/evomem",
		},
		"keyword form": {
			"host=db user=evomem password=hunter2 dbname=evomem",
			"postgres",
		},
		"nonsense": {"not a dsn at all", "postgres"},
	} {
		t.Run(name, func(t *testing.T) {
			got := describePostgres(c.dsn)
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Whatever it says, it must never carry the password.
func TestTheDescriptionNeverCarriesTheSecret(t *testing.T) {
	const secret = "hunter2"
	for _, dsn := range []string{
		"postgres://evomem:" + secret + "@db:5432/evomem",
		"postgres://evomem:" + secret + "@db:5432/evomem?sslmode=require",
		"host=db password=" + secret + " dbname=evomem",
	} {
		if got := describePostgres(dsn); contains(got, secret) {
			t.Fatalf("%q leaked the password: %q", dsn, got)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
