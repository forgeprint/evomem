package organize

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The shape asserted here is Anthropic's own, from
// platform.claude.com/docs/en/api/messages and .../api/versioning, read
// 2026-10-10. Every line of it is something the docs state and the code
// would otherwise be guessing at.
func TestTheRequestMatchesTheDocumentedShape(t *testing.T) {
	var got messagesRequest
	var headers http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		headers = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"type":"message","role":"assistant","stop_reason":"end_turn",
			"content":[{"type":"text","text":"{\"groups\":[{\"name\":\"n\",\"summary\":\"s\",\"note_ids\":[\"a\"]}]}"}]}`)
	}))
	defer server.Close()

	svc, err := NewAnthropic(Config{
		BaseURL: server.URL,
		Token:   "sk-ant-test",
		Model:   "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}

	// x-api-key, not a bearer token.
	if headers.Get("x-api-key") != "sk-ant-test" {
		t.Errorf("x-api-key: %q", headers.Get("x-api-key"))
	}
	if headers.Get("Authorization") != "" {
		t.Error("a bearer token was sent; this API does not take one")
	}
	// Required, and the versioning page gives this value.
	if headers.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version: %q", headers.Get("anthropic-version"))
	}

	if got.Model != "claude-sonnet-4-6" {
		t.Errorf("model: %q", got.Model)
	}
	// Required here, unlike the OpenAI-compatible side where it may be
	// left out entirely.
	if got.MaxTokens == 0 {
		t.Error("max_tokens is required and was not sent")
	}
	// The system prompt is a top-level field. There is no system role for
	// input messages, and sending one as a message is the mistake this
	// pins down.
	if !strings.Contains(got.System, "JSON") {
		t.Errorf("the system prompt is not in the system field: %q", got.System)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("messages: %+v", got.Messages)
	}
	for _, m := range got.Messages {
		if m.Role == "system" {
			t.Error("a system message was sent; there is no system role for input")
		}
	}
	if !strings.Contains(got.Messages[0].Content, "a: hello") {
		t.Errorf("the notes are not in the message: %q", got.Messages[0].Content)
	}

	if len(groups) != 1 || groups[0].Name != "n" {
		t.Fatalf("groups: %+v", groups)
	}
}

// The text is in the blocks whose type is "text". The others — thinking,
// tool use — are not the answer, and concatenating them would feed the
// parser something that is not JSON.
func TestOnlyTextBlocksAreRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"stop_reason":"end_turn","content":[
			{"type":"thinking","thinking":"let me see"},
			{"type":"text","text":"{\"groups\":[{\"name\":\"n\",\"summary\":\"s\",\"note_ids\":[\"a\"]}]}"}
		]}`)
	}))
	defer server.Close()

	svc, _ := NewAnthropic(Config{BaseURL: server.URL, Token: "k"})
	groups, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups: %+v", groups)
	}
}

func TestNoTextBlockIsNotAnEmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"stop_reason":"end_turn","content":[]}`)
	}))
	defer server.Close()

	svc, _ := NewAnthropic(Config{BaseURL: server.URL, Token: "k"})
	_, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("expected ErrNoAnswer, got %v", err)
	}
}

// stop_reason max_tokens means the JSON is cut off mid-object, and the
// parse below would fail with something unhelpful.
func TestATruncatedAnswerSaysSoOnAnthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"stop_reason":"max_tokens","content":[{"type":"text","text":"{\"groups\":[{\"name\":\"half"}]}`)
	}))
	defer server.Close()

	svc, _ := NewAnthropic(Config{BaseURL: server.URL, Token: "k"})
	_, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "ran out of room") {
		t.Fatalf("a truncated answer should say so, got %v", err)
	}
}

// The error body carries a sentence the person at the panel can act on;
// "401" on its own is not one.
func TestTheApiErrorMessageReachesTheCaller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer server.Close()

	svc, _ := NewAnthropic(Config{BaseURL: server.URL, Token: "k"})
	_, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("expected the server's reason, got %v", err)
	}
}

// Unlike the OpenAI-compatible side, where a self-hosted server often wants
// no key, this API always does.
func TestNoKeyIsTheFeatureBeingOff(t *testing.T) {
	if _, err := NewAnthropic(Config{BaseURL: "https://api.anthropic.com"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestItDefaultsToAnthropicsOwnAddress(t *testing.T) {
	svc, err := NewAnthropic(Config{Token: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if svc.cfg.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("base url: %q", svc.cfg.BaseURL)
	}
	if svc.cfg.Model == "" {
		t.Fatal("a model is required by the API, so there has to be a default")
	}
}
