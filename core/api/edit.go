package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// editRequest is the JSON body PUT /notes/{id} takes.
//
// Both fields are optional so that a caller can change the content without
// restating metadata it does not own, or add metadata without resending the
// text. A body with neither is a request that means nothing and is refused
// rather than silently accepted.
type editRequest struct {
	Content  *string         `json:"content"`
	Metadata *map[string]any `json:"metadata"`
}

// handleEdit replaces what a note says, never what it is.
//
// The identifier, the project and the source are what a note is; only the
// content and the metadata are what it currently says (ADR-0019). The server
// sets updated_at itself, because a clock it does not own has no business
// ordering its rows and updated_at is half of the sync cursor.
//
// A note from here is not marked tainted, for the same reason /ingest's is
// not: it arrives under the user's own token.
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	id, err := models.NormalizeULID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "the path has to end in a note id, as /ingest returned it",
			http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read the request body", http.StatusBadRequest)
		return
	}

	var req editRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "the body has to be a JSON object with content and/or metadata",
			http.StatusBadRequest)
		return
	}
	if req.Content == nil && req.Metadata == nil {
		http.Error(w, "nothing to change: pass content, metadata, or both",
			http.StatusBadRequest)
		return
	}

	note, err := s.store.Get(r.Context(), id)
	if err != nil {
		// 404 and nothing written. The phone is told the mirror no longer has
		// this note rather than being handed a new one, which would bring
		// back something somebody deleted here (ADR-0019).
		if errors.Is(err, database.ErrNotFound) {
			http.Error(w, "there is no such note", http.StatusNotFound)
			return
		}
		http.Error(w, "could not read the note", http.StatusInternalServerError)
		return
	}

	if req.Content != nil {
		trimmed := strings.TrimSpace(*req.Content)
		if trimmed == "" {
			// An empty note occupies a search result and says nothing; the
			// model layer refuses one on create and this is the same rule.
			http.Error(w, "content cannot be empty", http.StatusBadRequest)
			return
		}
		note.Content = trimmed
	}
	if req.Metadata != nil {
		note.Metadata = *req.Metadata
	}

	if err := s.store.Update(r.Context(), note); err != nil {
		http.Error(w, "could not store the note", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":     "ok",
		"id":         note.ID,
		"project":    note.ProjectID,
		"updated_at": note.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}
