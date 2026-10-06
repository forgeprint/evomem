package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/forgeprint/evomem/shared/models"
)

// ingestRequest is the JSON body /ingest takes.
type ingestRequest struct {
	Project  string         `json:"project"`
	Content  string         `json:"content"`
	Source   string         `json:"source"`
	Metadata map[string]any `json:"metadata"`
}

// handleIngest stores one note posted by an automation trigger.
//
// Two body formats, because the first consumer is Apple Shortcuts. A Shortcut
// can post JSON, and it can just as easily post the text it captured with no
// structure at all; making the second an error would mean every Shortcut
// starts with a dictionary block for no reason. So a JSON content type is
// parsed and anything else is taken as the note's text, with the project from
// the query string or the configured default.
//
// A note from here is not marked tainted. It arrives under the user's own
// token, from the user's own device, carrying what the user dictated or typed
// — the same standing as evomem add. The adapters are the other case: their
// content is written by third parties.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}

	req, err := parseIngest(r, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	project := strings.TrimSpace(req.Project)
	if project == "" {
		project = s.cfg.DefaultProject
	}
	if project == "" {
		http.Error(w, "no project: pass one in the body or as ?project=, or configure a default",
			http.StatusBadRequest)
		return
	}

	source := models.SourceType(strings.TrimSpace(req.Source))
	if source == "" {
		source = models.SourceShortcut
	}

	note := &models.Note{
		ProjectID:  project,
		Content:    strings.TrimSpace(req.Content),
		SourceType: source,
		Metadata:   req.Metadata,
	}
	if err := note.Validate(); err != nil {
		// A note with no content is the mistake a half-built Shortcut
		// makes, so the reason has to come back rather than a bare 400.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.Create(r.Context(), note); err != nil {
		http.Error(w, "could not store the note", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"id":      note.ID,
		"project": note.ProjectID,
	})
}

// parseIngest reads the body in whichever of the two shapes it is.
func parseIngest(r *http.Request, body []byte) (*ingestRequest, error) {
	mediaType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))

	if mediaType == "application/json" {
		var req ingestRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("body is not valid JSON: %w", err)
		}
		return &req, nil
	}

	content := strings.TrimSpace(string(body))
	if content == "" {
		return nil, errors.New("body is empty")
	}
	return &ingestRequest{
		Project: r.URL.Query().Get("project"),
		Content: content,
		Source:  r.URL.Query().Get("source"),
	}, nil
}
