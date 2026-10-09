package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
)

// storedNote puts one note in the store through /ingest and returns the
// identifier the server minted, which is what an edit addresses.
func storedNote(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	w := ingest(t, h, body, "application/json", token, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("storing the note: %d %s", w.Code, w.Body)
	}
	var reply struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	return reply.ID
}

func edit(t *testing.T, h http.Handler, id, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPut, "/notes/"+id, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

func TestEditReplacesWhatTheNoteSays(t *testing.T) {
	h, db := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"first wording"}`)

	w := edit(t, h, id, `{"content":"  second wording  "}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	note, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	// Trimmed, like every other way content enters the store.
	if note.Content != "second wording" {
		t.Errorf("content = %q", note.Content)
	}
}

// The identifier, the project and the source are what a note is.
func TestEditDoesNotChangeWhatTheNoteIs(t *testing.T) {
	h, db := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"first","source":"shortcut"}`)

	before, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	w := edit(t, h, id,
		`{"content":"second","project":"somewhere-else","source":"telegram","id":"01M4D3H3HNMFM69N4MHNAYBZ1Z"}`,
		token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	after, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Errorf("id changed to %s", after.ID)
	}
	if after.ProjectID != before.ProjectID {
		t.Errorf("project changed to %s", after.ProjectID)
	}
	if after.SourceType != before.SourceType {
		t.Errorf("source changed to %s", after.SourceType)
	}
	if after.CreatedAt != before.CreatedAt {
		t.Errorf("created_at changed")
	}
}

// updated_at is half of the sync cursor, so the server sets it rather than
// taking a clock it does not own from the body.
func TestEditMovesUpdatedAtForwardItself(t *testing.T) {
	h, db := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"first"}`)
	before, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	w := edit(t, h, id, `{"content":"second","updated_at":"1999-01-01T00:00:00Z"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	after, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("updated_at went from %s to %s", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestEditReplacesMetadataAndCanLeaveContentAlone(t *testing.T) {
	h, db := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"the words","metadata":{"pinned":true}}`)

	w := edit(t, h, id, `{"metadata":{"pinned":false,"colour":"red"}}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	note, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if note.Content != "the words" {
		t.Errorf("content = %q; metadata-only edit changed it", note.Content)
	}
	if note.Metadata["pinned"] != false || note.Metadata["colour"] != "red" {
		t.Errorf("metadata = %v", note.Metadata)
	}
}

// A note the mirror no longer has is reported, never recreated: a delete that
// undoes itself on the next sync is the worst outcome available (ADR-0019).
func TestEditOfAMissingNoteIs404AndWritesNothing(t *testing.T) {
	h, db := newServer(t, fullConfig())

	before, err := db.Count(t.Context(), database.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	w := edit(t, h, "01M4D3H3HNMFM69N4MHNAYBZ1Z", `{"content":"resurrect me"}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	after, err := db.Count(t.Context(), database.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("a 404 created a note: %d then %d", before, after)
	}
}

func TestEditRefusesWhatItCannotActOn(t *testing.T) {
	h, _ := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"first"}`)

	for name, body := range map[string]string{
		"neither field": `{}`,
		"empty content": `{"content":"   "}`,
		"not an object": `[]`,
		"not json":      `nonsense`,
		"null content":  `{"content":null}`,
	} {
		w := edit(t, h, id, body, token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
}

func TestEditRefusesAnIdentifierItCannotTrust(t *testing.T) {
	h, _ := newServer(t, fullConfig())
	for _, id := range []string{"not-a-ulid", "..%2F..%2Fetc", "01M4D3H3HNMFM69N4MHNAYBZ"} {
		w := edit(t, h, id, `{"content":"x"}`, token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("id %q: %d, want 400", id, w.Code)
		}
	}
}

func TestEditRejectsBadCredentials(t *testing.T) {
	h, db := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"first"}`)

	for _, bearer := range []string{"", "wrong-token", token + "x"} {
		w := edit(t, h, id, `{"content":"second"}`, bearer)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("bearer %q: %d, want 401", bearer, w.Code)
		}
	}

	note, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if note.Content != "first" {
		t.Errorf("an unauthorized request changed the note to %q", note.Content)
	}
}

// No token, no endpoint — the posture every endpoint in this package shares.
func TestEditIsNotServedWithoutAToken(t *testing.T) {
	h, _ := newServer(t, Config{TelegramSecret: telegramSecret, TelegramProject: "evomem"})
	if w := edit(t, h, "01M4D3H3HNMFM69N4MHNAYBZ1Z", `{"content":"x"}`, token); w.Code != http.StatusNotFound {
		t.Errorf("%d, want 404", w.Code)
	}
}

// An edit arrives under the user's own token, like /ingest, so it is not
// third-party text and carries no taint.
func TestEditDoesNotTaintTheNote(t *testing.T) {
	h, db := newServer(t, fullConfig())
	id := storedNote(t, h, `{"project":"evomem","content":"first"}`)

	if w := edit(t, h, id, `{"content":"second"}`, token); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	note, err := db.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if note.Tainted() {
		t.Error("an edit under the user's own token was marked tainted")
	}
}

func TestRoutesNameTheEditEndpoint(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := New(db, fullConfig())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, route := range s.Routes() {
		if route == "PUT /notes/{id}" {
			found = true
		}
	}
	if !found {
		t.Errorf("routes = %v", s.Routes())
	}
}
