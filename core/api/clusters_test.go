package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

func put(t *testing.T, h http.Handler, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

// groupedServer is a server with two notes already grouped.
func groupedServer(t *testing.T) (http.Handler, *database.DB, string, []string) {
	t.Helper()
	h, db := newServer(t, fullConfig())
	ctx := context.Background()

	var ids []string
	for _, content := range []string{"first", "second"} {
		note := &models.Note{ProjectID: "evomem", Content: content, SourceType: models.SourceManual}
		if err := db.Create(ctx, note); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, note.ID)
	}

	cluster := &database.Cluster{ProjectID: "evomem", Name: "before", Summary: "old"}
	if err := db.CreateCluster(ctx, cluster, ids); err != nil {
		t.Fatal(err)
	}
	return h, db, cluster.ID, ids
}

func TestEditingAGroupRenamesItAndGivesItBack(t *testing.T) {
	h, _, id, _ := groupedServer(t)

	w := put(t, h, "/clusters/"+id, `{"name":"after","summary":"new"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	var got struct {
		Cluster database.Cluster `json:"cluster"`
		Notes   []*models.Note   `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Cluster.Name != "after" || got.Cluster.Summary != "new" {
		t.Fatalf("cluster: %+v", got.Cluster)
	}
	// Handed back so the caller does not have to guess what it now is.
	if len(got.Notes) != 2 {
		t.Fatalf("%d note(s) came back", len(got.Notes))
	}
}

// "Not given" and "set it to empty" are different, which is why the fields
// are pointers: renaming a group must not wipe its summary.
func TestEditingOnlyWhatWasGiven(t *testing.T) {
	h, _, id, _ := groupedServer(t)

	w := put(t, h, "/clusters/"+id, `{"name":"renamed"}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct {
		Cluster database.Cluster `json:"cluster"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Cluster.Summary != "old" {
		t.Fatalf("renaming wiped the summary: %q", got.Cluster.Summary)
	}

	// And clearing it deliberately has to work. A fresh value, because
	// Cluster.Summary is omitempty: unmarshalling an absent field into a
	// struct that already holds "old" leaves "old" there, which is a way
	// to write a test that passes whatever the code does.
	var cleared struct {
		Cluster database.Cluster `json:"cluster"`
	}
	w = put(t, h, "/clusters/"+id, `{"summary":""}`, token)
	if err := json.Unmarshal(w.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Cluster.Summary != "" {
		t.Fatalf("the summary could not be cleared: %q", cleared.Cluster.Summary)
	}
}

// Taking a note out of a group must leave the note alone. That line is
// older than this endpoint (ADR-0023) and is the reason it is safe to
// expose at all.
func TestRemovingANoteFromAGroupKeepsTheNote(t *testing.T) {
	h, db, id, ids := groupedServer(t)

	w := put(t, h, "/clusters/"+id, `{"remove":["`+ids[0]+`"]}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct {
		Notes []*models.Note `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 1 || got.Notes[0].ID != ids[1] {
		t.Fatalf("notes: %+v", got.Notes)
	}

	if _, err := db.Get(context.Background(), ids[0]); err != nil {
		t.Fatal("removing a note from a group deleted the note")
	}
}

func TestDeletingAGroupLeavesTheNotes(t *testing.T) {
	h, db, id, ids := groupedServer(t)

	r := httptest.NewRequest(http.MethodDelete, "/clusters/"+id, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if w := do(h, r); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	ctx := context.Background()
	clusters, err := db.Clusters(ctx, "evomem")
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 0 {
		t.Fatalf("%d grouping(s) survived", len(clusters))
	}
	for _, id := range ids {
		if _, err := db.Get(ctx, id); err != nil {
			t.Fatalf("deleting a grouping took note %s with it", id)
		}
	}
}

func TestEditingNeedsTheToken(t *testing.T) {
	h, _, id, _ := groupedServer(t)

	for _, bearer := range []string{"", "not-the-token"} {
		if w := put(t, h, "/clusters/"+id, `{"name":"x"}`, bearer); w.Code != http.StatusUnauthorized {
			t.Errorf("editing with %q answered %d", bearer, w.Code)
		}
	}
}

func TestEditingRefusesWhatItCannotDo(t *testing.T) {
	h, _, id, _ := groupedServer(t)

	for name, c := range map[string]struct {
		path, body string
		want       int
	}{
		"nothing to change": {"/clusters/" + id, `{}`, http.StatusBadRequest},
		"an empty name":     {"/clusters/" + id, `{"name":"  "}`, http.StatusBadRequest},
		"not a cluster id":  {"/clusters/not-a-ulid", `{"name":"x"}`, http.StatusBadRequest},
		"no such grouping": {
			"/clusters/01M4D3H3HNMFM69N4MHNAYBZ1X",
			`{"name":"x"}`, http.StatusNotFound,
		},
		"not json": {"/clusters/" + id, `nope`, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			if w := put(t, h, c.path, c.body, token); w.Code != c.want {
				t.Fatalf("got %d, want %d: %s", w.Code, c.want, w.Body)
			}
		})
	}
}
