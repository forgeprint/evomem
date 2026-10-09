package api

import (
	"net/http"
	"strings"
)

// corsOrigins reads the configured list once, at startup.
//
// Empty means no origin is allowed, which is the same shape as every secret
// here: absent is off, not open. A wildcard is not accepted at all — an
// endpoint that serves a token's worth of somebody's memory to `*` is one
// that any page they visit can read (ADR-0024).
func corsOrigins(configured string) map[string]bool {
	allowed := map[string]bool{}
	for _, one := range strings.Split(configured, ",") {
		origin := strings.TrimSpace(one)
		if origin == "" || origin == "*" {
			continue
		}
		allowed[origin] = true
	}
	return allowed
}

// withCORS answers a browser's preflight and marks the response for the
// origins that were configured.
//
// Wrapped around the whole mux rather than the read routes alone: a browser
// posting a note hits the same wall, and a rule that applies to half the
// server is a rule somebody debugs twice.
func withCORS(next http.Handler, allowed map[string]bool) http.Handler {
	if len(allowed) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			header := w.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			// The answer depends on the request's Origin, so a cache
			// that ignored it would serve one origin's headers to
			// another.
			header.Add("Vary", "Origin")
			header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			header.Set("Access-Control-Max-Age", "600")
		}

		// A preflight is answered whether or not the origin was allowed:
		// without the headers above the browser refuses it anyway, and
		// that is the refusal the developer needs to see.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
