package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
)

// loggingServer is a server that writes its access log where a test can read
// it.
func loggingServer(t *testing.T) (http.Handler, *bytes.Buffer, *database.DB) {
	t.Helper()
	log := &bytes.Buffer{}
	cfg := fullConfig()
	cfg.AccessLog = log
	h, db := newServer(t, cfg)
	return h, log, db
}

// The day this exists for: ten notes deleted through the API, and nothing
// anywhere able to say by whom.
func TestADeleteIsRecordedWithWhoMadeIt(t *testing.T) {
	h, log, db := loggingServer(t)

	w := ingest(t, h, `{"project":"evomem","content":"something"}`,
		"application/json", token, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var reply struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	id := reply.ID

	r := httptest.NewRequest(http.MethodDelete, "/notes/"+id, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if got := do(h, r); got.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", got.Code, got.Body)
	}
	_ = db

	line := findLine(t, log, "DELETE /notes/")
	for _, want := range []string{"DELETE", "/notes/" + id, "204", "as=environment"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line does not say %q: %s", want, line)
		}
	}
}

// A refused request is the other half of the question. It never reaches a
// handler, so the log has to sit outside them.
func TestARefusedRequestIsRecordedToo(t *testing.T) {
	h, log, _ := loggingServer(t)

	r := httptest.NewRequest(http.MethodDelete, "/notes/01M4D3H3HNMFM69N4MHNAYBZ1X", nil)
	r.Header.Set("Authorization", "Bearer not-the-token")
	if got := do(h, r); got.Code != http.StatusUnauthorized {
		t.Fatalf("%d", got.Code)
	}

	line := findLine(t, log, "DELETE /notes/")
	if !strings.Contains(line, "401") || !strings.Contains(line, "as="+Anonymous) {
		t.Errorf("a refused delete was not recorded as refused: %s", line)
	}
}

// A log that leaks the credential is worse than no log.
func TestTheLogNeverCarriesTheToken(t *testing.T) {
	h, log, _ := loggingServer(t)

	w := ingest(t, h, `{"project":"evomem","content":"x"}`, "application/json", token, "")
	if w.Code != http.StatusCreated {
		t.Fatal(w.Code)
	}
	if strings.Contains(log.String(), token) {
		t.Error("the access log contains the API token")
	}
	// Not a prefix of it either: a few characters of a secret are still
	// characters of a secret.
	if len(token) >= 8 && strings.Contains(log.String(), token[:8]) {
		t.Error("the access log contains the start of the API token")
	}
}

// A query string here carries the user's own content — a search term is the
// note they are looking for — so the path is logged without it.
func TestTheLogDoesNotCopyTheSearchTerm(t *testing.T) {
	h, log, _ := loggingServer(t)

	if got := get(t, h, "/notes?q=something+private", token); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body)
	}

	if strings.Contains(log.String(), "private") {
		t.Errorf("the search term was copied into the log: %s", log.String())
	}
	if !strings.Contains(log.String(), "GET /notes") {
		t.Errorf("the request was not recorded at all: %s", log.String())
	}
}

// Compose checks this every ten seconds. Eight thousand lines a day that
// answer nothing is a log nobody reads.
func TestHealthChecksAreNotLogged(t *testing.T) {
	h, log, _ := loggingServer(t)

	if got := get(t, h, "/healthz", ""); got.Code != http.StatusOK {
		t.Fatalf("%d", got.Code)
	}
	if strings.Contains(log.String(), "/healthz") {
		t.Errorf("the health check was logged: %s", log.String())
	}
}

// Nil means no log, and the server still serves.
func TestNoLogIsAllowed(t *testing.T) {
	cfg := fullConfig()
	cfg.AccessLog = nil
	h, _ := newServer(t, cfg)

	if got := get(t, h, "/healthz", ""); got.Code != http.StatusOK {
		t.Fatalf("%d", got.Code)
	}
}

func findLine(t *testing.T, log *bytes.Buffer, contains string) string {
	t.Helper()
	for _, line := range strings.Split(log.String(), "\n") {
		if strings.Contains(line, contains) {
			return line
		}
	}
	t.Fatalf("no line contains %q in:\n%s", contains, log.String())
	return ""
}
