package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

func get(t *testing.T, h http.Handler, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var payload T
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	return payload
}

func TestListNotesAnswersWhatTheServerHolds(t *testing.T) {
	h, db := newServer(t, fullConfig())
	for _, content := range []string{"the tunnel has to be running", "the webhook needs a secret"} {
		n := &models.Note{ProjectID: "evomem", Content: content, SourceType: models.SourceManual}
		if err := db.Create(t.Context(), n); err != nil {
			t.Fatal(err)
		}
	}

	w := get(t, h, "/notes?project=evomem", token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	payload := decode[struct {
		Notes []models.Note `json:"notes"`
	}](t, w)
	if len(payload.Notes) != 2 {
		t.Errorf("%d notes", len(payload.Notes))
	}
}

// An empty answer is [] rather than null: a client parsing this should not
// have to tell the two apart.
func TestListNotesAnswersAnEmptyListNotNull(t *testing.T) {
	h, _ := newServer(t, fullConfig())
	w := get(t, h, "/notes?project=nothing-here", token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, `"notes":[]`) {
		t.Errorf("body = %s", got)
	}
}

func TestListNotesNarrowsAndSearches(t *testing.T) {
	h, db := newServer(t, fullConfig())
	typed := &models.Note{ProjectID: "evomem", Content: "the tunnel has to be running", SourceType: models.SourceManual}
	fromJira := &models.Note{ProjectID: "evomem", Content: "EVO-12 sync worker", SourceType: models.SourceJira}
	for _, n := range []*models.Note{typed, fromJira} {
		if err := db.Create(t.Context(), n); err != nil {
			t.Fatal(err)
		}
	}

	// A source is a first-class filter: the phone and Jira are both sources.
	w := get(t, h, "/notes?project=evomem&source=jira", token)
	notes := decode[struct {
		Notes []models.Note `json:"notes"`
	}](t, w).Notes
	if len(notes) != 1 || notes[0].SourceType != models.SourceJira {
		t.Errorf("source filter returned %d notes", len(notes))
	}

	w = get(t, h, "/notes?project=evomem&q=tunnel", token)
	hits := decode[struct {
		Hits []database.SearchHit `json:"hits"`
	}](t, w).Hits
	if len(hits) != 1 {
		t.Fatalf("%d hits", len(hits))
	}
	if hits[0].Note.Content != typed.Content {
		t.Errorf("found %q", hits[0].Note.Content)
	}
}

// The read side carries the marks in metadata and no warning prose: a client
// renders them, and the lines exist because a model reads prose (ADR-0024).
func TestListNotesCarriesTheMarksWithoutTheProse(t *testing.T) {
	h, db := newServer(t, fullConfig())
	n := &models.Note{ProjectID: "evomem", Content: "from outside", SourceType: models.SourceTelegram}
	n.MarkTainted("telegram")
	if err := db.Create(t.Context(), n); err != nil {
		t.Fatal(err)
	}

	w := get(t, h, "/notes?project=evomem", token)
	body := w.Body.String()
	if !strings.Contains(body, `"tainted":true`) || !strings.Contains(body, `"origin":"telegram"`) {
		t.Errorf("the marks are missing: %s", body)
	}
	if strings.Contains(body, "untrusted") {
		t.Errorf("the MCP warning prose leaked into the read side: %s", body)
	}
}

func TestReadClusters(t *testing.T) {
	h, db := newServer(t, fullConfig())
	n := &models.Note{ProjectID: "evomem", Content: "the tunnel has to be running", SourceType: models.SourceManual}
	if err := db.Create(t.Context(), n); err != nil {
		t.Fatal(err)
	}
	c := &database.Cluster{ProjectID: "evomem", Name: "Deployment", Summary: "what has to be up"}
	if err := db.CreateCluster(t.Context(), c, []string{n.ID}); err != nil {
		t.Fatal(err)
	}

	w := get(t, h, "/clusters?project=evomem", token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	clusters := decode[struct {
		Clusters []database.Cluster `json:"clusters"`
	}](t, w).Clusters
	if len(clusters) != 1 || clusters[0].Name != "Deployment" || clusters[0].Size != 1 {
		t.Fatalf("clusters = %+v", clusters)
	}

	w = get(t, h, "/clusters/"+c.ID, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	one := decode[struct {
		Cluster database.Cluster `json:"cluster"`
		Notes   []models.Note    `json:"notes"`
	}](t, w)
	if one.Cluster.Name != "Deployment" || len(one.Notes) != 1 {
		t.Errorf("cluster = %+v, %d notes", one.Cluster, len(one.Notes))
	}
}

func TestReadClusterRefusesWhatItCannotFind(t *testing.T) {
	h, _ := newServer(t, fullConfig())
	if w := get(t, h, "/clusters/not-a-ulid", token); w.Code != http.StatusBadRequest {
		t.Errorf("malformed id: %d, want 400", w.Code)
	}
	if w := get(t, h, "/clusters/01M4D3H3HNMFM69N4MHNAYBZ1Z", token); w.Code != http.StatusNotFound {
		t.Errorf("missing cluster: %d, want 404", w.Code)
	}
}

// The read side is the more serious half of a leaked token, so it is behind
// the same posture as every write (ADR-0024).
func TestReadSideNeedsTheToken(t *testing.T) {
	h, _ := newServer(t, fullConfig())
	for _, path := range []string{"/notes", "/clusters", "/clusters/01M4D3H3HNMFM69N4MHNAYBZ1Z"} {
		if w := get(t, h, path, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s with no token: %d, want 401", path, w.Code)
		}
		if w := get(t, h, path, "wrong-token"); w.Code != http.StatusUnauthorized {
			t.Errorf("%s with a wrong token: %d, want 401", path, w.Code)
		}
	}
}

func TestReadSideIsNotServedWithoutAToken(t *testing.T) {
	h, _ := newServer(t, Config{TelegramSecret: telegramSecret, TelegramProject: "evomem"})
	if w := get(t, h, "/notes", token); w.Code != http.StatusNotFound {
		t.Errorf("%d, want 404", w.Code)
	}
}
