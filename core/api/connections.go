package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/forgeprint/evomem/core/connect"
	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Connections is the part of the store the panel needs.
//
// Its own interface, like Reader and Store: what holds other people's
// credentials should say so in the type it asks for.
type Connections interface {
	Connections(ctx context.Context) ([]*database.Connection, error)
	AddConnection(ctx context.Context, c *database.Connection, secret string, key database.SecretKey) error
	RemoveConnection(ctx context.Context, id string) error
}

// handleListConnections answers what is connected.
//
// Never the secret: the ciphertext is unexported in the struct and absent
// from its JSON, so there is no shape here that could carry it out.
func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := s.connections.Connections(r.Context())
	if err != nil {
		http.Error(w, "could not list the connections", http.StatusInternalServerError)
		return
	}
	if connections == nil {
		connections = []*database.Connection{}
	}
	writeReadJSON(w, map[string]any{"connections": connections})
}

// connectRequest is the JSON body POST /connections takes.
type connectRequest struct {
	SourceType string `json:"source_type"`
	ProjectID  string `json:"project_id"`
	BaseURL    string `json:"base_url"`
	Account    string `json:"account"`
	Query      string `json:"query"`
	Secret     string `json:"secret"`
}

// handleConnect stores a source and seals its token.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read the request body", http.StatusBadRequest)
		return
	}
	var req connectRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "the body has to be a JSON object", http.StatusBadRequest)
		return
	}

	key, err := s.secretKey()
	if err != nil {
		// A configuration problem on the server, not a bad request: the
		// person at the panel cannot fix it from there, and saying "bad
		// request" would send them looking in the wrong place.
		http.Error(w,
			"this server has no usable EVOMEM_SECRET_KEY, so a token cannot be stored",
			http.StatusServiceUnavailable)
		return
	}

	conn := &database.Connection{
		SourceType: strings.TrimSpace(req.SourceType),
		ProjectID:  strings.TrimSpace(req.ProjectID),
		BaseURL:    strings.TrimSpace(req.BaseURL),
		Account:    strings.TrimSpace(req.Account),
		Query:      strings.TrimSpace(req.Query),
	}
	if err := s.connections.AddConnection(r.Context(), conn, req.Secret, key); err != nil {
		if errors.Is(err, models.ErrEmptyProjectID) || strings.Contains(err.Error(), "needs") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "could not store the connection", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeReadJSON(w, map[string]any{"connection": conn})
}

// handleForget removes a source and the token with it.
func (s *Server) handleForget(w http.ResponseWriter, r *http.Request) {
	err := s.connections.RemoveConnection(r.Context(), r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, models.ErrInvalidULID):
		http.Error(w, "the path has to end in a connection id", http.StatusBadRequest)
	case errors.Is(err, database.ErrNotFound):
		http.Error(w, "there is no such connection", http.StatusNotFound)
	default:
		http.Error(w, "could not remove the connection", http.StatusInternalServerError)
	}
}

// handlePull calls every connected source now.
//
// Synchronous, and the write timeout is the limit (ADR-0026). A browser that
// gives up does not stop the pull: it carries on here and records its result
// on the connection, so the panel catches up on the next load.
func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	key, err := s.secretKey()
	if err != nil {
		http.Error(w,
			"this server has no usable EVOMEM_SECRET_KEY, so the stored tokens cannot be read",
			http.StatusServiceUnavailable)
		return
	}
	if s.puller == nil {
		http.Error(w, "this server cannot pull", http.StatusServiceUnavailable)
		return
	}

	result, err := s.puller(r.Context(), key)
	if err != nil {
		http.Error(w, "the pull could not be run", http.StatusInternalServerError)
		return
	}
	writeReadJSON(w, map[string]any{
		"considered": result.Considered,
		"pulled":     result.Pulled,
		"failed":     result.Failed,
	})
}

// PullFunc runs every connection. Injected so that core/api does not import
// the connectors: the panel's job is to ask, not to know how Jira works.
type PullFunc func(ctx context.Context, key database.SecretKey) (connect.Result, error)

// OrganizeFunc groups a project's ungrouped notes with the connected model.
//
// Injected for the same reason PullFunc is: core/api asks, and core/organize
// is what knows how to talk to a model.
type OrganizeFunc func(ctx context.Context, key database.SecretKey, projectID string, dryRun bool) (OrganizeResult, error)

// OrganizeResult is what one grouping run did, as the panel reports it.
type OrganizeResult struct {
	Offered  int `json:"offered"`
	Created  int `json:"created"`
	Grouped  int `json:"grouped"`
	Invented int `json:"invented"`
	Dropped  int `json:"dropped"`
}

// ErrNoModel is the feature being off rather than a failure: nobody has
// connected a model, so nothing calls one and no note leaves the machine.
var ErrNoModel = errors.New("api: no model connected")

// handleOrganize asks the connected model to group what is ungrouped.
//
// Synchronous like the pull, and bounded by the same write timeout
// (ADR-0027). A browser that gives up does not stop the run; the clusters it
// writes are there on the next load.
func (s *Server) handleOrganize(w http.ResponseWriter, r *http.Request) {
	key, err := s.secretKey()
	if err != nil {
		http.Error(w,
			"this server has no usable EVOMEM_SECRET_KEY, so the model's key cannot be read",
			http.StatusServiceUnavailable)
		return
	}
	if s.organizer == nil {
		http.Error(w, "this server cannot group notes", http.StatusServiceUnavailable)
		return
	}

	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project == "" {
		project = s.defaultProject()
	}
	dryRun := r.URL.Query().Get("dry_run") == "1"

	result, err := s.organizer(r.Context(), key, project, dryRun)
	switch {
	case errors.Is(err, ErrNoModel):
		// Not an error the panel should show as a failure: it is the
		// switch being off, and the panel says how to turn it on.
		http.Error(w, "no model is connected, so nothing was sent anywhere",
			http.StatusServiceUnavailable)
		return
	case err != nil:
		// The model's own words, not a generic message: "incorrect api
		// key" and "model not found" are the two things that actually
		// go wrong here and the person at the panel can fix both.
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeReadJSON(w, map[string]any{
		"offered": result.Offered, "created": result.Created,
		"grouped": result.Grouped, "invented": result.Invented,
		"dropped": result.Dropped, "project": project,
	})
}

// defaultProject is what a request that named no project is grouped under.
func (s *Server) defaultProject() string {
	if s.cfg.DefaultProject != "" {
		return s.cfg.DefaultProject
	}
	return "default"
}
