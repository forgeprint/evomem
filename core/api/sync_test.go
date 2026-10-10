package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
)

type pulled struct {
	Notes []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"notes"`
	Deletions []struct {
		ID string `json:"id"`
	} `json:"deletions"`
	Cursor struct {
		Since   string `json:"since"`
		SinceID string `json:"since_id"`
	} `json:"cursor"`
}

func decodePull(t *testing.T, w *httptest.ResponseRecorder) pulled {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var p pulled
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// syncServer is a server with the pull endpoints and some notes on it.
func syncServer(t *testing.T) (http.Handler, *database.DB, []string) {
	t.Helper()
	h, db := newServer(t, fullConfig())

	var ids []string
	for _, content := range []string{"first", "second", "third"} {
		w := ingest(t, h, `{"project":"evomem","content":"`+content+`"}`,
			"application/json", token, "")
		if w.Code != http.StatusCreated {
			t.Fatalf("storing %q: %d %s", content, w.Code, w.Body)
		}
		var reply struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, reply.ID)
	}
	return h, db, ids
}

func TestPullWalksNotesOldestFirstAndStops(t *testing.T) {
	h, _, ids := syncServer(t)

	first := decodePull(t, get(t, h, "/sync/notes?limit=2", token))
	if len(first.Notes) != 2 {
		t.Fatalf("asked for 2, got %d", len(first.Notes))
	}
	// Oldest first, so a client can stop anywhere and resume.
	if first.Notes[0].Content != "first" || first.Notes[1].Content != "second" {
		t.Errorf("out of order: %q then %q", first.Notes[0].Content, first.Notes[1].Content)
	}
	if first.Cursor.SinceID != ids[1] {
		t.Errorf("cursor is %s, want the last note of the page (%s)", first.Cursor.SinceID, ids[1])
	}

	second := decodePull(t, get(t,
		h, "/sync/notes?limit=2&since="+first.Cursor.Since+"&since_id="+first.Cursor.SinceID, token))
	if len(second.Notes) != 1 || second.Notes[0].Content != "third" {
		t.Fatalf("the second page is %+v", second.Notes)
	}

	// Caught up. The cursor must come back, not come back empty: a client
	// that stored an empty string would start again from the beginning,
	// which is how the mirror's own pull broke (ADR-0032).
	third := decodePull(t, get(t,
		h, "/sync/notes?since="+second.Cursor.Since+"&since_id="+second.Cursor.SinceID, token))
	if len(third.Notes) != 0 {
		t.Errorf("a caught-up pull returned %d note(s)", len(third.Notes))
	}
	if third.Cursor.SinceID != second.Cursor.SinceID {
		t.Errorf("a caught-up pull moved the cursor: %q then %q",
			second.Cursor.SinceID, third.Cursor.SinceID)
	}
}

// The whole reason this exists: a note deleted here has to stop existing on
// the device.
func TestPullReportsADeletion(t *testing.T) {
	h, db, ids := syncServer(t)

	// Nothing deleted yet.
	before := decodePull(t, get(t, h, "/sync/deletions", token))
	if len(before.Deletions) != 0 {
		t.Fatalf("%d tombstone(s) before anything was deleted", len(before.Deletions))
	}

	if err := db.Delete(t.Context(), ids[1]); err != nil {
		t.Fatal(err)
	}

	after := decodePull(t, get(t, h, "/sync/deletions", token))
	if len(after.Deletions) != 1 || after.Deletions[0].ID != ids[1] {
		t.Fatalf("tombstones: %+v", after.Deletions)
	}
	if after.Cursor.SinceID != ids[1] {
		t.Errorf("cursor is %q", after.Cursor.SinceID)
	}

	// And it settles.
	again := decodePull(t, get(t,
		h, "/sync/deletions?since="+after.Cursor.Since+"&since_id="+after.Cursor.SinceID, token))
	if len(again.Deletions) != 0 {
		t.Errorf("the same tombstone came back: %+v", again.Deletions)
	}
}

// Half a cursor is refused rather than guessed at: a time with no identifier
// can only say "this instant", which has to either repeat a row or skip one.
func TestPullRefusesHalfACursor(t *testing.T) {
	h, _, _ := syncServer(t)

	for _, url := range []string{
		"/sync/notes?since=2026-10-10T00:00:00Z",
		"/sync/notes?since_id=01M4D3H3HNMFM69N4MHNAYBZ1X",
		"/sync/notes?since=yesterday&since_id=01M4D3H3HNMFM69N4MHNAYBZ1X",
		"/sync/deletions?since=2026-10-10T00:00:00Z",
	} {
		if w := get(t, h, url, token); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", url, w.Code)
		}
	}
}

func TestPullNeedsTheToken(t *testing.T) {
	h, _, _ := syncServer(t)

	for _, url := range []string{"/sync/notes", "/sync/deletions"} {
		for _, bearer := range []string{"", "wrong-token"} {
			if w := get(t, h, url, bearer); w.Code != http.StatusUnauthorized {
				t.Errorf("%s with %q: %d, want 401", url, bearer, w.Code)
			}
		}
	}
}

// A page is bounded whatever the client asks for, so one request cannot be
// made to read the whole store into memory.
func TestPullBoundsThePage(t *testing.T) {
	h, _, _ := syncServer(t)

	r := httptest.NewRequest(http.MethodGet, "/sync/notes?limit=100000", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if w := do(h, r); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if pullLimit(r) != maxPullBatch {
		t.Errorf("limit %d, want it bounded to %d", pullLimit(r), maxPullBatch)
	}
}

func TestRoutesNameThePullEndpoints(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := New(db, fullConfig())
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{"GET /sync/notes": false, "GET /sync/deletions": false}
	for _, r := range s.Routes() {
		if _, ok := want[r]; ok {
			want[r] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("%s is not served; routes = %v", route, s.Routes())
		}
	}
}
