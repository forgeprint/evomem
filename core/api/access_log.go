package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// whoKey carries the name of whoever a request was authorised as, from the
// check that did the authorising to the line that records it.
type whoKey struct{}

// Anonymous is what the log says when a request carried no usable token.
// It is not an error on its own — a probe, a stale device, a typo — but it
// is the thing somebody asks about afterwards.
const Anonymous = "-"

// whoHolder is a box the log puts in the context before the request goes
// anywhere, for the authorising check to fill in later.
//
// A plain context value does not work here and the reason is easy to miss:
// the log sits outermost and holds the request it was handed, while the
// middleware that learns the name builds a *new* request with
// `WithContext`. The outer one never sees it, so every line read `as=-` no
// matter who had been let in. A pointer put in first is shared by both.
type whoHolder struct{ name string }

// withWho records who this request was authorised as.
func withWho(r *http.Request, who string) *http.Request {
	if holder, ok := r.Context().Value(whoKey{}).(*whoHolder); ok {
		holder.name = who
	}
	return r
}

// whoFor reads it back.
func whoFor(holder *whoHolder) string {
	if holder != nil && holder.name != "" {
		return holder.name
	}
	return Anonymous
}

// recorder remembers the status a handler chose, because http.ResponseWriter
// does not offer it back.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *recorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *recorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// logRequests writes one line per request.
//
// This exists because of a day when ten notes were deleted through the API
// and nothing anywhere could say by whom. The store had the tombstones and
// the times; there was no record of a request at all, so the question had no
// answer and the only honest report was "I do not know".
//
// What a line carries, and what it deliberately does not:
//
//   - the method and the **path without its query**. A query string here
//     carries the user's own content — `GET /notes?q=…` is a search term —
//     and a log is a second copy of it in a file nobody is thinking about.
//     Cursors are lost along with it; that is the price.
//   - **who**, by the name on the device token, or `environment` for the
//     token in the server's own configuration. Never the token itself, nor
//     its prefix: a log that leaks a credential is worse than no log.
//   - the status, the bytes written, how long it took, and the peer's
//     address.
//
// `GET /healthz` is skipped. Compose checks it every ten seconds, which is
// eight thousand lines a day that answer nothing.
func logRequests(out io.Writer, next http.Handler) http.Handler {
	if out == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		started := time.Now()
		rec := &recorder{ResponseWriter: w}
		holder := &whoHolder{}
		next.ServeHTTP(rec, r.WithContext(
			context.WithValue(r.Context(), whoKey{}, holder)))

		status := rec.status
		if status == 0 {
			// A handler that wrote nothing at all still answered 200.
			status = http.StatusOK
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}

		fmt.Fprintf(out, "%s %s %s %d %dB %s from=%s as=%s\n",
			started.UTC().Format(time.RFC3339),
			r.Method, r.URL.Path, status, rec.bytes,
			time.Since(started).Round(time.Millisecond), host, whoFor(holder))
	})
}
