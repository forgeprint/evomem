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

// Tokens is the part of the store that holds device tokens (ADR-0030).
//
// Its own interface, like Connections: what decides whether a request is
// served should say so in the type it asks for.
type Tokens interface {
	// TokenFor returns the token a presented secret belongs to, or
	// database.ErrNotFound when there is none or it was revoked.
	TokenFor(ctx context.Context, plaintext string) (*database.Token, error)

	// TouchToken records that a token was used.
	TouchToken(ctx context.Context, token *database.Token) error

	// Tokens lists what exists, revoked ones included.
	Tokens(ctx context.Context) ([]*database.Token, error)

	// NewToken mints one, returning the secret exactly once.
	NewToken(ctx context.Context, name string) (*database.Token, string, error)

	// RevokeToken turns one off.
	RevokeToken(ctx context.Context, id string) error
}

// handleListTokens answers which devices can get in.
//
// Never a secret: the struct holds a hash and a prefix, and the token itself
// was handed over once and is not stored (ADR-0030).
func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.tokens.Tokens(r.Context())
	if err != nil {
		http.Error(w, "could not list the tokens", http.StatusInternalServerError)
		return
	}
	if tokens == nil {
		tokens = []*database.Token{}
	}
	writeReadJSON(w, map[string]any{"tokens": tokens})
}

// newTokenRequest is the JSON body POST /tokens takes.
type newTokenRequest struct {
	Name string `json:"name"`
}

// handleNewToken mints a token for a device.
//
// The reply carries the secret, and it is the only time it exists. Said in
// the field name so nobody treats it as something they can fetch again.
func (s *Server) handleNewToken(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read the request body", http.StatusBadRequest)
		return
	}
	var req newTokenRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "the body has to be a JSON object", http.StatusBadRequest)
		return
	}

	token, plaintext, err := s.tokens.NewToken(r.Context(), strings.TrimSpace(req.Name))
	switch {
	case errors.Is(err, database.ErrEmptyTokenName):
		http.Error(w, "a token needs a name, so it can be told from the others",
			http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "could not make a token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeReadJSON(w, map[string]any{
		"token": token,
		// Named for what it is: this reply is the only copy.
		"secret_shown_once": plaintext,
	})
}

// handleRevokeToken turns a device's token off.
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	err := s.tokens.RevokeToken(r.Context(), r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, models.ErrInvalidULID):
		http.Error(w, "the path has to end in a token id", http.StatusBadRequest)
	case errors.Is(err, database.ErrNotFound):
		http.Error(w, "there is no such token", http.StatusNotFound)
	default:
		http.Error(w, "could not revoke the token", http.StatusInternalServerError)
	}
}
