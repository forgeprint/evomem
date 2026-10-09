package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Reader is the part of the database the read side needs. Separate from
// Store, which is what the write side uses, so each says what it touches.
type Reader interface {
	List(ctx context.Context, opts database.ListOptions) ([]*models.Note, error)
	Search(ctx context.Context, q database.SearchQuery) ([]database.SearchHit, error)
	Clusters(ctx context.Context, projectID string) ([]database.Cluster, error)
	Cluster(ctx context.Context, id string) (*database.Cluster, []*models.Note, error)
}

// handleListNotes answers what the server holds.
//
// A view, not a sync mechanism (ADR-0024): a client asking what is here is
// not the same as a client pulling it into a store of its own, and calling it
// a pull would reopen the question ADR-0012 closed.
//
// No warning lines, unlike the MCP tools. The marks are in the metadata and a
// client renders them for a person; the prose exists because a model reads
// prose.
func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := intParam(query.Get("limit"))
	offset := intParam(query.Get("offset"))

	if text := strings.TrimSpace(query.Get("q")); text != "" {
		hits, err := s.reader.Search(r.Context(), database.SearchQuery{
			Text:       text,
			ProjectID:  query.Get("project"),
			SourceType: models.SourceType(query.Get("source")),
			Limit:      limit,
		})
		if err != nil {
			http.Error(w, "could not search", http.StatusInternalServerError)
			return
		}
		if hits == nil {
			hits = []database.SearchHit{}
		}
		writeReadJSON(w, map[string]any{"hits": hits})
		return
	}

	notes, err := s.reader.List(r.Context(), database.ListOptions{
		ProjectID:  query.Get("project"),
		SourceType: models.SourceType(query.Get("source")),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		http.Error(w, "could not list notes", http.StatusInternalServerError)
		return
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	writeReadJSON(w, map[string]any{"notes": notes})
}

// handleListClusters answers what the server has grouped.
func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := s.reader.Clusters(r.Context(), r.URL.Query().Get("project"))
	if err != nil {
		http.Error(w, "could not list clusters", http.StatusInternalServerError)
		return
	}
	if clusters == nil {
		clusters = []database.Cluster{}
	}
	writeReadJSON(w, map[string]any{"clusters": clusters})
}

// handleGetCluster answers one grouping and the notes in it.
func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cluster, notes, err := s.reader.Cluster(r.Context(), id)
	switch {
	case errors.Is(err, models.ErrInvalidULID):
		http.Error(w, "the path has to end in a cluster id", http.StatusBadRequest)
		return
	case errors.Is(err, database.ErrNotFound):
		http.Error(w, "there is no such cluster", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "could not read the cluster", http.StatusInternalServerError)
		return
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	writeReadJSON(w, map[string]any{"cluster": cluster, "notes": notes})
}

// intParam reads a bound from the query string. Anything unreadable is zero,
// which the store clamps to its own default rather than erroring: a client
// that sent limit=many wanted a page, not a 400.
func intParam(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func writeReadJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
