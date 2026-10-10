package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Groupings is the part of the store that changes clusters.
//
// Its own interface beside Reader, which only looks: a type that can rename
// and dissolve a grouping should say so (ADR-0024's amendment).
type Groupings interface {
	UpdateCluster(ctx context.Context, id string, change database.ClusterChange) error
	DeleteCluster(ctx context.Context, id string) error
}

// editClusterRequest is the JSON body PUT /clusters/{id} takes.
//
// Pointers for the text, so that "not given" and "set it to empty" are
// different things: a client renaming a group must not be made to resend the
// summary, and a client clearing a summary must be able to.
type editClusterRequest struct {
	Name    *string  `json:"name"`
	Summary *string  `json:"summary"`
	Add     []string `json:"add"`
	Remove  []string `json:"remove"`
}

// handleEditCluster renames a grouping, re-summarises it, or changes who is
// in it.
//
// The authority split from ADR-0024 is unchanged: a cluster still lives on
// the server and nowhere else. This is a client asking the server to change
// one, which is what the MCP agent already does — the same act through a
// different door, not a second copy of the truth.
//
// It never touches what a note says. That line is older than this endpoint
// (ADR-0023) and still holds: the only thing here is the label and the
// membership.
func (s *Server) handleEditCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read the request body", http.StatusBadRequest)
		return
	}
	var req editClusterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "the body has to be a JSON object", http.StatusBadRequest)
		return
	}
	if req.Name == nil && req.Summary == nil && len(req.Add) == 0 && len(req.Remove) == 0 {
		http.Error(w, "nothing to change", http.StatusBadRequest)
		return
	}

	change := database.ClusterChange{Add: req.Add, Remove: req.Remove}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		change.Name = &trimmed
	}
	if req.Summary != nil {
		trimmed := strings.TrimSpace(*req.Summary)
		change.Summary = &trimmed
	}

	switch err := s.groupings.UpdateCluster(r.Context(), id, change); {
	case err == nil:
	case errors.Is(err, models.ErrInvalidULID):
		http.Error(w, "the path has to end in a cluster id", http.StatusBadRequest)
		return
	case errors.Is(err, database.ErrNotFound):
		http.Error(w, "there is no such grouping", http.StatusNotFound)
		return
	case errors.Is(err, database.ErrEmptyClusterName):
		http.Error(w, "a grouping needs a name", http.StatusBadRequest)
		return
	default:
		http.Error(w, "could not change the grouping", http.StatusInternalServerError)
		return
	}

	cluster, notes, err := s.reader.Cluster(r.Context(), id)
	if err != nil {
		// Changed, but it cannot be read back. Saying 200 with no body
		// would leave the caller guessing what it now is.
		http.Error(w, "the grouping changed but could not be read back",
			http.StatusInternalServerError)
		return
	}
	writeReadJSON(w, map[string]any{"cluster": cluster, "notes": notes})
}

// handleDeleteCluster takes a grouping off, and leaves the notes.
//
// The label goes and the members stay, which is what makes this safe to
// offer at all (ADR-0023): the only thing a person can undo by hand is a
// grouping, so the only thing they can destroy here is one.
func (s *Server) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	switch err := s.groupings.DeleteCluster(r.Context(), r.PathValue("id")); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, models.ErrInvalidULID):
		http.Error(w, "the path has to end in a cluster id", http.StatusBadRequest)
	case errors.Is(err, database.ErrNotFound):
		http.Error(w, "there is no such grouping", http.StatusNotFound)
	default:
		http.Error(w, "could not delete the grouping", http.StatusInternalServerError)
	}
}
