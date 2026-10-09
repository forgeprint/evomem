// Package organize asks a model to group notes that are in no group yet.
//
// The second path to a grouping, beside the agent that does it over MCP
// (ADR-0023). Nothing here runs on its own: `evomem organize` and the panel's
// button drive it, and with no model key configured the feature is off and no
// note leaves the machine (ADR-0027).
//
// Evomem is the client here, never the server.
package organize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Defaults for a service that was not told otherwise.
const (
	// DefaultModel is what the request names when the connection does not.
	// The field is required by the interface, so there is no "unset".
	DefaultModel = "gpt-4o-mini"

	// DefaultTimeout bounds one request. Grouping two hundred notes is a
	// long generation, slower than a web request and far faster than the
	// transcription of an hour of audio.
	DefaultTimeout = 3 * time.Minute

	// MaxNotes is how many notes one run offers the model, and MaxContent
	// how much of each. Both are ceilings on a bill as much as on a
	// request: a catalog larger than this takes several presses
	// (ADR-0027).
	MaxNotes   = 200
	MaxContent = 2000

	// maxCompletionTokens bounds the reply. Deliberately generous: the
	// answer carries an id per note.
	maxCompletionTokens = 8000
)

// ErrNotConfigured is what a caller gets when no model is connected. It is
// not a failure: it is the feature being switched off.
var ErrNotConfigured = errors.New("organize: no model connected")

// ErrNoAnswer is a reply that arrived with nothing in it: no choices, or a
// choice whose content was null. The schema allows both.
var ErrNoAnswer = errors.New("organize: the model answered with no content")

// Config is how a model is reached.
//
// Verified against OpenAI's own OpenAPI document (openai/openai-openapi,
// openapi.yaml, info.version 2.3.0, read 2026-10-09): the request goes to
// /v1/chat/completions, requires `model` and `messages`, and the security
// scheme is HTTP bearer.
type Config struct {
	// BaseURL of an OpenAI-compatible server, without the path: the
	// request goes to BaseURL + "/v1/chat/completions". Empty means the
	// feature is off.
	BaseURL string

	// Token for the Authorization header. A self-hosted server often
	// wants none.
	Token string

	// Model names the model in the request. Required by the interface,
	// defaulted to DefaultModel.
	Model string

	// Timeout bounds one request, defaulted to DefaultTimeout.
	Timeout time.Duration

	// HTTPClient is for tests to substitute. Normally nil.
	HTTPClient *http.Client
}

// A Service asks a model to group notes.
type Service struct {
	cfg    Config
	client *http.Client
}

// New builds a service, or reports that the feature is off.
func New(cfg Config) (*Service, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &Service{cfg: cfg, client: client}, nil
}

// An Item is one note as the model sees it.
type Item struct {
	ID      string
	Content string
}

// A Group is one grouping the model proposed.
type Group struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary"`
	NoteIDs []string `json:"note_ids"`
}

// chatRequest is the body of POST /v1/chat/completions.
//
// Field names are the spec's. `max_tokens` is deprecated there in favour of
// `max_completion_tokens`, so the current name is what gets sent; a server
// that only knows the old one ignores this and uses its own default.
type chatRequest struct {
	Model               string         `json:"model"`
	Messages            []chatMessage  `json:"messages"`
	Temperature         float64        `json:"temperature"`
	MaxCompletionTokens int            `json:"max_completion_tokens"`
	ResponseFormat      map[string]any `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the part of the reply this reads.
//
// Content is a pointer because the schema allows it to be null, and a null
// that decoded as "" would be indistinguishable from a model that answered
// with an empty string.
type chatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content *string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Group asks the model to arrange the given notes.
//
// What comes back is what the model said, unfiltered: dropping ids it
// invented is the caller's job, because the caller is the one that knows
// what it sent (ADR-0027).
func (s *Service) Group(ctx context.Context, items []Item) ([]Group, error) {
	if len(items) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(chatRequest{
		Model: s.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: renderItems(items)},
		},
		// Low rather than zero: grouping is a judgement, and a server
		// that rejects 0 is a server this has to work on.
		Temperature:         0.2,
		MaxCompletionTokens: maxCompletionTokens,
		// Sent, never relied on. A server that does not know the field
		// still answers JSON because the prompt asks for it — which
		// the spec says is required for this mode anyway.
		ResponseFormat: map[string]any{"type": "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("organize: building the request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("organize: building the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("organize: calling the model: %w", err)
	}
	defer resp.Body.Close()

	// Bounded: an error page from a proxy can be a megabyte of HTML, and
	// it is going into a message somebody reads.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("organize: reading the reply: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("organize: the model answered %s: %s",
			resp.Status, firstLine(raw))
	}

	var reply chatResponse
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("organize: the reply was not the expected JSON: %w", err)
	}
	if len(reply.Choices) == 0 || reply.Choices[0].Message.Content == nil {
		return nil, ErrNoAnswer
	}
	if reason := reply.Choices[0].FinishReason; reason == "length" {
		// Worth its own message: the JSON is cut off mid-object and
		// the parse below will fail with something unhelpful.
		return nil, fmt.Errorf("organize: the model ran out of room (finish_reason %q); fewer notes per run", reason)
	}
	return parseGroups(*reply.Choices[0].Message.Content)
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
