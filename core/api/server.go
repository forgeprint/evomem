// Package api is Evomem's HTTP entrypoint: the ingestion surface that
// adapters and automation triggers post to.
//
// It binds to the loopback interface by default. The webhook endpoints have
// to be reachable by Telegram and by Jira, which means a tunnel the user puts
// in front of it, and that is deliberate: this server has no TLS, no rate
// limiting and no account model, so it is not something to expose directly.
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/forgeprint/evomem/core/api/adapters/jira"
	"github.com/forgeprint/evomem/core/api/adapters/telegram"
	"github.com/forgeprint/evomem/shared/models"
)

// DefaultAddr is where the server listens unless told otherwise. Loopback,
// not :8765, so a laptop on a café network is not running an open ingest
// endpoint by accident.
const DefaultAddr = "127.0.0.1:8765"

// Timeouts. An ingestion endpoint talks to the open internet through a
// tunnel, and a connection that is never closed is a connection that cannot
// be reused.
const (
	readTimeout     = 15 * time.Second
	writeTimeout    = 15 * time.Second
	idleTimeout     = 60 * time.Second
	shutdownTimeout = 10 * time.Second
)

// maxBody bounds a request body on the endpoints this package serves itself.
// The Jira adapter sets its own, because it has to buffer the whole body to
// check the signature.
const maxBody = 1 << 20

// Store is the part of the database this package needs.
type Store interface {
	Create(ctx context.Context, n *models.Note) error
}

// Config is how a server is set up. Every secret is separate: a Telegram
// secret is chosen by setWebhook and a Jira secret by the webhook's own
// configuration, so there is no single credential that could cover both.
type Config struct {
	// Addr to listen on. Empty means DefaultAddr.
	Addr string

	// Token authenticates /ingest, the endpoint Apple Shortcuts and any
	// other automation posts to. Without it that endpoint is not served.
	Token string

	// DefaultProject is what /ingest files a note under when the request
	// does not say.
	DefaultProject string

	// TelegramSecret is the token passed to setWebhook's secret_token.
	// Without it, and a project, /telegram/webhook is not served.
	TelegramSecret  string
	TelegramProject string

	// JiraSecret is the secret configured on the Jira webhook. Without it
	// /jira/webhook is not served. JiraProject overrides the issue's own
	// project key and is normally empty.
	JiraSecret  string
	JiraProject string
}

// Server is the HTTP entrypoint.
type Server struct {
	http  *http.Server
	store Store
	cfg   Config

	// routes is what was actually served, for the caller to report. A
	// silently missing endpoint is the failure this makes visible.
	routes []string
}

// New builds a server. An endpoint whose secret is missing is not registered
// at all, so the failure is a 404 at a known address rather than an open way
// to write to the user's memory.
//
// It is an error for nothing at all to be configured: a server with no
// endpoint is never what the user meant.
func New(store Store, cfg Config) (*Server, error) {
	if store == nil {
		return nil, errors.New("api: no store")
	}
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}

	s := &Server{store: store, cfg: cfg}
	mux := http.NewServeMux()

	// Unauthenticated on purpose, and says nothing but that the process is
	// up. It is what a tunnel and a launch agent check.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	if cfg.Token != "" {
		mux.Handle("POST /ingest", s.authenticated(http.HandlerFunc(s.handleIngest)))
		s.routes = append(s.routes, "POST /ingest")
	}

	if cfg.TelegramSecret != "" && cfg.TelegramProject != "" {
		h, err := telegram.New(store, cfg.TelegramSecret, cfg.TelegramProject)
		if err != nil {
			return nil, err
		}
		mux.Handle("POST /telegram/webhook", limitBody(h))
		s.routes = append(s.routes, "POST /telegram/webhook")
	}

	if cfg.JiraSecret != "" {
		h, err := jira.New(store, cfg.JiraSecret, cfg.JiraProject)
		if err != nil {
			return nil, err
		}
		// No limitBody here: the adapter has to read the whole body to
		// check the signature and bounds it itself.
		mux.Handle("POST /jira/webhook", h)
		s.routes = append(s.routes, "POST /jira/webhook")
	}

	if len(s.routes) == 0 {
		return nil, errors.New("api: nothing is configured; set a token or an adapter secret")
	}

	s.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	return s, nil
}

// Routes is what the server serves, in the order it registered them.
func (s *Server) Routes() []string { return s.routes }

// Addr is where the server listens.
func (s *Server) Addr() string { return s.cfg.Addr }

// Handler exposes the mux, for tests.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// ListenAndServe serves until ctx is cancelled, then stops, giving requests
// in flight a moment to finish.
func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("api: listening on %s: %w", s.cfg.Addr, err)
	}
	// A port of 0 means the kernel chose one, and the caller has to be
	// able to find out which.
	s.cfg.Addr = listener.Addr().String()
	if s.http != nil {
		s.http.Addr = s.cfg.Addr
	}

	errs := make(chan error, 1)
	go func() {
		err := s.http.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errs <- err
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return s.http.Shutdown(stop)
	}
}

// authenticated requires the bearer token on Config.Token.
func (s *Server) authenticated(next http.Handler) http.Handler {
	return limitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			// No WWW-Authenticate challenge: there is nothing for a
			// browser to prompt for, and this is not a login.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// authorized checks the bearer token in constant time.
func (s *Server) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	return constantTimeEqual(token, s.cfg.Token)
}

// limitBody caps what a handler can be made to read.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

// readBody reads a bounded body, reporting a payload that was too large
// distinctly from one that could not be read.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return nil, false
		}
		http.Error(w, "could not read the request", http.StatusBadRequest)
		return nil, false
	}
	return data, true
}
