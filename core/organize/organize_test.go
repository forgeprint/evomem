package organize

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// testKey is the 32 bytes a key has to be, spelled so that it reads as a
// placeholder rather than as somebody's key.
var testKey = strings.Repeat("evomem-test-key-", 2)

func store(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "evomem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func add(t *testing.T, db *database.DB, project, content string) *models.Note {
	t.Helper()
	note := &models.Note{ProjectID: project, Content: content, SourceType: models.SourceManual}
	if err := db.Create(context.Background(), note); err != nil {
		t.Fatal(err)
	}
	return note
}

// fakeModel answers with whatever the test put in it.
type fakeModel struct {
	groups []Group
	err    error
	saw    []Item
}

func (f *fakeModel) Group(_ context.Context, items []Item) ([]Group, error) {
	f.saw = items
	return f.groups, f.err
}

func TestUngroupedLeavesClusteredNotesAlone(t *testing.T) {
	db := store(t)
	ctx := context.Background()

	alone := add(t, db, "p", "ungrouped")
	member := add(t, db, "p", "already in a group")
	elsewhere := add(t, db, "other", "another project")

	if err := db.CreateCluster(ctx, &database.Cluster{ProjectID: "p", Name: "existing"},
		[]string{member.ID}); err != nil {
		t.Fatal(err)
	}

	got, err := db.Ungrouped(ctx, "p", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != alone.ID {
		t.Fatalf("expected only the ungrouped note of project p, got %d", len(got))
	}
	if got[0].ID == elsewhere.ID {
		t.Fatal("another project's note leaked into the run")
	}
}

// A second press must add groups, never dissolve one. The notes placed by
// the first run are no longer offered, so the model cannot move them.
func TestRunTwiceDoesNotRegroup(t *testing.T) {
	db := store(t)
	ctx := context.Background()
	a, b := add(t, db, "p", "one"), add(t, db, "p", "two")

	model := &fakeModel{groups: []Group{{Name: "first", NoteIDs: []string{a.ID, b.ID}}}}
	if _, err := Run(ctx, db, model, "p", false, io.Discard); err != nil {
		t.Fatal(err)
	}

	second, err := Run(ctx, db, model, "p", false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if second.Offered != 0 || second.Created != 0 {
		t.Fatalf("a second run touched grouped notes: %+v", second)
	}
	clusters, err := db.Clusters(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected the one cluster to survive, got %d", len(clusters))
	}
}

func TestRunDropsIdsTheModelInvented(t *testing.T) {
	db := store(t)
	ctx := context.Background()
	real := add(t, db, "p", "a real note")

	model := &fakeModel{groups: []Group{
		{Name: "mixed", NoteIDs: []string{real.ID, models.NewULID()}},
		{Name: "entirely made up", NoteIDs: []string{models.NewULID()}},
	}}
	result, err := Run(ctx, db, model, "p", false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Invented != 2 {
		t.Fatalf("expected 2 invented ids, got %d", result.Invented)
	}
	if result.Dropped != 1 {
		t.Fatalf("expected the empty group to be dropped, got %d", result.Dropped)
	}
	if result.Created != 1 || result.Grouped != 1 {
		t.Fatalf("expected one cluster with one note, got %+v", result)
	}

	_, notes, err := db.Cluster(ctx, mustOneCluster(t, db).ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].ID != real.ID {
		t.Fatal("the cluster names a note that was never sent to the model")
	}
}

func TestRunPlacesANoteOnlyOnce(t *testing.T) {
	db := store(t)
	ctx := context.Background()
	a := add(t, db, "p", "one")

	model := &fakeModel{groups: []Group{
		{Name: "first", NoteIDs: []string{a.ID}},
		{Name: "second", NoteIDs: []string{a.ID}},
	}}
	result, err := Run(ctx, db, model, "p", false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Dropped != 1 {
		t.Fatalf("the same note was placed twice: %+v", result)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	db := store(t)
	ctx := context.Background()
	a := add(t, db, "p", "one")

	model := &fakeModel{groups: []Group{{Name: "would be", NoteIDs: []string{a.ID}}}}
	var log strings.Builder
	result, err := Run(ctx, db, model, "p", true, &log)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 {
		t.Fatalf("a dry run should still report what it would do, got %+v", result)
	}
	clusters, err := db.Clusters(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Fatal("a dry run wrote a cluster")
	}
	if !strings.Contains(log.String(), "would create") {
		t.Fatalf("a dry run said nothing about what it would do: %q", log.String())
	}
}

func mustOneCluster(t *testing.T, db *database.DB) database.Cluster {
	t.Helper()
	clusters, err := db.Clusters(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected one cluster, got %d", len(clusters))
	}
	return clusters[0]
}

// --- the wire format ---

// The shape asserted here is OpenAI's own, from openai/openai-openapi
// openapi.yaml (info.version 2.3.0, read 2026-10-09): `model` and `messages`
// are required, a message is {role, content}, and the reply carries the text
// at choices[].message.content.
func TestRequestMatchesTheDocumentedShape(t *testing.T) {
	var got chatRequest
	var auth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"groups\":[{\"name\":\"n\",\"summary\":\"s\",\"note_ids\":[\"a\"]}]}"}}]}`)
	}))
	defer server.Close()

	svc, err := New(Config{BaseURL: server.URL, Token: "the-token", Model: "a-model"})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}

	if auth != "Bearer the-token" {
		t.Fatalf("the spec's security scheme is http bearer, sent %q", auth)
	}
	if got.Model != "a-model" {
		t.Fatalf("model: %q", got.Model)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Fatalf("messages: %+v", got.Messages)
	}
	if !strings.Contains(got.Messages[0].Content, "JSON") {
		t.Fatal("json_object mode requires the prompt to ask for JSON; it does not")
	}
	if !strings.Contains(got.Messages[1].Content, "a: hello") {
		t.Fatalf("the notes are not in the user message: %q", got.Messages[1].Content)
	}
	if got.ResponseFormat["type"] != "json_object" {
		t.Fatalf("response_format: %v", got.ResponseFormat)
	}
	if got.MaxCompletionTokens == 0 {
		t.Fatal("max_tokens is deprecated; max_completion_tokens should be sent")
	}
	if len(groups) != 1 || groups[0].Name != "n" {
		t.Fatalf("groups: %+v", groups)
	}
}

func TestNullContentIsNotAnEmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":null}}]}`)
	}))
	defer server.Close()

	svc, _ := New(Config{BaseURL: server.URL})
	_, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("expected ErrNoAnswer, got %v", err)
	}
}

func TestATruncatedAnswerSaysSo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{\"groups\":[{\"name\":\"half"}}]}`)
	}))
	defer server.Close()

	svc, _ := New(Config{BaseURL: server.URL})
	_, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "ran out of room") {
		t.Fatalf("a truncated answer should say so, got %v", err)
	}
}

func TestAnErrorStatusCarriesTheReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"incorrect api key"}}`)
	}))
	defer server.Close()

	svc, _ := New(Config{BaseURL: server.URL})
	_, err := svc.Group(context.Background(), []Item{{ID: "a", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "incorrect api key") {
		t.Fatalf("expected the server's reason, got %v", err)
	}
}

func TestNoBaseURLIsTheFeatureBeingOff(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

// --- parsing what a model actually sends back ---

func TestParseGroupsSurvivesAServerThatIgnoredResponseFormat(t *testing.T) {
	for name, answer := range map[string]string{
		"bare":        `{"groups":[{"name":"n","summary":"s","note_ids":["a"]}]}`,
		"code fence":  "Here you go:\n```json\n{\"groups\":[{\"name\":\"n\",\"summary\":\"s\",\"note_ids\":[\"a\"]}]}\n```",
		"with prose":  `Sure. {"groups":[{"name":"n","summary":"s","note_ids":["a"]}]} Hope that helps.`,
		"brace in it": `{"groups":[{"name":"the {weird} one","summary":"s","note_ids":["a"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			groups, err := parseGroups(answer)
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || len(groups[0].NoteIDs) != 1 {
				t.Fatalf("groups: %+v", groups)
			}
		})
	}
}

func TestParseGroupsRefusesWhatItCannotRead(t *testing.T) {
	for name, answer := range map[string]string{
		"no json":   "I'd rather not.",
		"truncated": `{"groups":[{"name":"half`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseGroups(answer); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRenderItemsFoldsNewlinesAndCutsOnRunes(t *testing.T) {
	long := strings.Repeat("ş", MaxContent+50)
	out := renderItems([]Item{{ID: "a", Content: "two\nlines"}, {ID: "b", Content: long}})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("a note with a newline became two lines: %q", out)
	}
	if !strings.Contains(lines[0], "a: two lines") {
		t.Fatalf("newline not folded: %q", lines[0])
	}
	if strings.Contains(lines[1], "�") {
		t.Fatal("the cut landed mid-rune and produced a replacement character")
	}
}

// --- the keyring ---

func TestFromConnectionFindsTheModelAndNotASource(t *testing.T) {
	db := store(t)
	ctx := context.Background()
	key, err := database.ParseSecretKey(testKey)
	if err != nil {
		t.Fatal(err)
	}

	jira := &database.Connection{SourceType: "jira", ProjectID: "p", BaseURL: "https://jira.invalid"}
	if err := db.AddConnection(ctx, jira, "a-source-token", key); err != nil {
		t.Fatal(err)
	}
	model := &database.Connection{
		SourceType: database.ModelSourceType,
		ProjectID:  "p",
		BaseURL:    "https://model.invalid",
		Query:      "a-model",
	}
	if err := db.AddConnection(ctx, model, "a-model-key", key); err != nil {
		t.Fatal(err)
	}

	connections, err := db.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := FromConnection(connections, key)
	if err != nil {
		t.Fatal(err)
	}
	if svc.cfg.BaseURL != "https://model.invalid" || svc.cfg.Model != "a-model" {
		t.Fatalf("picked the wrong row: %+v", svc.cfg)
	}
	if svc.cfg.Token != "a-model-key" {
		t.Fatal("the model's own key was not the one unsealed")
	}
}

func TestFromConnectionWithNoModelIsTheFeatureBeingOff(t *testing.T) {
	db := store(t)
	ctx := context.Background()
	key, _ := database.ParseSecretKey(testKey)
	jira := &database.Connection{SourceType: "jira", ProjectID: "p", BaseURL: "https://jira.invalid"}
	if err := db.AddConnection(ctx, jira, "a-source-token", key); err != nil {
		t.Fatal(err)
	}
	connections, _ := db.Connections(ctx)
	if _, err := FromConnection(connections, key); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
