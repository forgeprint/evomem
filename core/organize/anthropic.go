package organize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Anthropic's own API, which is a different shape from the one Service
// speaks (ADR-0027's amendment).
//
// Verified against Anthropic's own documentation on 2026-10-10:
//
//   - https://platform.claude.com/docs/en/api/messages
//   - https://platform.claude.com/docs/en/api/versioning
//
// What those pages say, and what this therefore does:
//
//   - `POST /v1/messages`, with `x-api-key` rather than a bearer token.
//   - `anthropic-version` is required; the versioning page gives
//     `2023-06-01` as the current value and the only other one, `2023-01-01`,
//     as the initial release.
//   - `model`, `max_tokens` and `messages` are required. max_tokens has no
//     default, unlike the OpenAI-compatible side where leaving it out is
//     allowed.
//   - **The system prompt is a top-level field, not a message.** The page
//     says there is no `system` role for input messages. Sending one as a
//     message is the mistake this comment exists to prevent.
//   - The reply's text is in `content`, an array of blocks, in the ones
//     whose `type` is `"text"`.
//   - `stop_reason` includes `max_tokens`, which means the JSON is cut off.
const (
	anthropicVersion  = "2023-06-01"
	anthropicDefaults = "https://api.anthropic.com"

	// anthropicDefaultModel is what a connection that named no model gets.
	anthropicDefaultModel = "claude-sonnet-4-6"
)

// AnthropicService asks Claude to group notes.
//
// A second Grouper rather than a flag on the first: the two APIs agree on
// almost nothing above the transport, and a single type with two code paths
// would be two implementations wearing one name.
type AnthropicService struct {
	cfg    Config
	client *http.Client
}

// NewAnthropic builds a service against Anthropic's API, or reports that
// the feature is off.
func NewAnthropic(cfg Config) (*AnthropicService, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		// Unlike the OpenAI-compatible side, where a self-hosted server
		// often wants no key at all, this API always does.
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = anthropicDefaults
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = anthropicDefaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &AnthropicService{cfg: cfg, client: client}, nil
}

// messagesRequest is the body of POST /v1/messages.
type messagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	// Low rather than zero: grouping is a judgement.
	Temperature float64 `json:"temperature"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the part of the reply this reads.
type messagesResponse struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Group asks Claude to arrange the given notes.
func (s *AnthropicService) Group(ctx context.Context, items []Item) ([]Group, error) {
	if len(items) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(messagesRequest{
		Model:     s.cfg.Model,
		MaxTokens: maxCompletionTokens,
		// Top-level, because there is no system role for input messages.
		System:      systemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: renderItems(items)}},
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("organize: building the request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("organize: building the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.cfg.Token)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("organize: calling the model: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("organize: reading the reply: %w", err)
	}

	var reply messagesResponse
	// Decoded before the status is judged, because the error body carries
	// a sentence the person at the panel can act on — "invalid x-api-key"
	// is worth more than "401".
	_ = json.Unmarshal(raw, &reply)

	if resp.StatusCode != http.StatusOK {
		if reply.Error != nil && reply.Error.Message != "" {
			return nil, fmt.Errorf("organize: the model answered %s: %s",
				resp.Status, reply.Error.Message)
		}
		return nil, fmt.Errorf("organize: the model answered %s: %s",
			resp.Status, firstLine(raw))
	}

	if reply.StopReason == "max_tokens" {
		return nil, fmt.Errorf("organize: the model ran out of room (stop_reason %q); fewer notes per run",
			reply.StopReason)
	}

	// The text lives in the blocks whose type is "text"; the others are
	// thinking, tool use and the rest, and none of them is the answer.
	var text strings.Builder
	for _, block := range reply.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return nil, ErrNoAnswer
	}
	return parseGroups(text.String())
}
