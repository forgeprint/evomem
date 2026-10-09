package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/core/connect"
	"github.com/forgeprint/evomem/shared/database"
)

var testSecretKey = strings.Repeat("evomem-test-key-", 2)

func panelConfig() Config {
	cfg := fullConfig()
	cfg.SecretKey = testSecretKey
	return cfg
}

func post(t *testing.T, h http.Handler, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

func forget(t *testing.T, h http.Handler, id, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodDelete, "/connections/"+id, nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

const jiraConnection = `{"source_type":"jira","project_id":"evomem",
	"base_url":"https://example.atlassian.net","account":"someone@example.com",
	"query":"project = EVO","secret":"the-api-token"}`

func TestConnectStoresTheSourceAndSealsTheToken(t *testing.T) {
	h, db := newServer(t, panelConfig())

	w := post(t, h, "/connections", jiraConnection, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	stored, err := db.Connections(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("%d connections", len(stored))
	}
	key, err := database.ParseSecretKey(testSecretKey)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := stored[0].Secret(key)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "the-api-token" {
		t.Errorf("secret = %q", secret)
	}
}

// There is no response shape that could carry a token out, and this is the
// test that keeps it that way.
func TestAConnectionNeverCarriesItsSecretOut(t *testing.T) {
	h, _ := newServer(t, panelConfig())

	created := post(t, h, "/connections", jiraConnection, token)
	if strings.Contains(created.Body.String(), "the-api-token") {
		t.Errorf("the create answer carried the token: %s", created.Body)
	}

	listed := get(t, h, "/connections", token)
	if listed.Code != http.StatusOK {
		t.Fatalf("%d %s", listed.Code, listed.Body)
	}
	body := listed.Body.String()
	if strings.Contains(body, "the-api-token") || strings.Contains(body, "secret") {
		t.Errorf("the listing carried the token or a field for it: %s", body)
	}
	// And it does carry what the panel has to show.
	for _, want := range []string{"jira", "example.atlassian.net", "someone@example.com"} {
		if !strings.Contains(body, want) {
			t.Errorf("the listing does not show %q: %s", want, body)
		}
	}
}

// A server with no key cannot store a token. That is a configuration problem
// the person at the panel cannot fix from there, so it is not a 400.
func TestConnectSaysWhenTheServerHasNoKey(t *testing.T) {
	h, _ := newServer(t, fullConfig()) // no SecretKey

	w := post(t, h, "/connections", jiraConnection, token)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("%d, want 503: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "EVOMEM_SECRET_KEY") {
		t.Errorf("it does not name what is missing: %s", w.Body)
	}
}

func TestConnectRefusesWhatItCannotUse(t *testing.T) {
	h, _ := newServer(t, panelConfig())

	for name, body := range map[string]string{
		"no source":  `{"project_id":"evomem","base_url":"https://x","secret":"t"}`,
		"no project": `{"source_type":"jira","base_url":"https://x","secret":"t"}`,
		"no url":     `{"source_type":"jira","project_id":"evomem","secret":"t"}`,
		"no token":   `{"source_type":"jira","project_id":"evomem","base_url":"https://x"}`,
		"not json":   `nonsense`,
	} {
		if w := post(t, h, "/connections", body, token); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
}

func TestForgetRemovesTheConnection(t *testing.T) {
	h, db := newServer(t, panelConfig())
	created := post(t, h, "/connections", jiraConnection, token)
	var reply struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if w := forget(t, h, reply.Connection.ID, token); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	stored, _ := db.Connections(t.Context())
	if len(stored) != 0 {
		t.Errorf("%d connections survived", len(stored))
	}

	if w := forget(t, h, reply.Connection.ID, token); w.Code != http.StatusNotFound {
		t.Errorf("a second forget: %d, want 404", w.Code)
	}
	if w := forget(t, h, "not-a-ulid", token); w.Code != http.StatusBadRequest {
		t.Errorf("a malformed id: %d, want 400", w.Code)
	}
}

func TestPullRunsAndReportsWhatItDid(t *testing.T) {
	cfg := panelConfig()
	var ran bool
	cfg.Pull = func(ctx context.Context, key database.SecretKey) (connect.Result, error) {
		ran = true
		return connect.Result{Considered: 1, Pulled: 3}, nil
	}
	h, _ := newServer(t, cfg)

	w := post(t, h, "/connections/pull", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !ran {
		t.Error("the pull was not run")
	}
	if !strings.Contains(w.Body.String(), `"pulled":3`) {
		t.Errorf("body = %s", w.Body)
	}
}

func TestPullSaysWhenTheServerCannotRunIt(t *testing.T) {
	// No key: the stored tokens cannot be read.
	h, _ := newServer(t, fullConfig())
	if w := post(t, h, "/connections/pull", "", token); w.Code != http.StatusServiceUnavailable {
		t.Errorf("without a key: %d, want 503", w.Code)
	}

	// A key but no connector wired in.
	h2, _ := newServer(t, panelConfig())
	if w := post(t, h2, "/connections/pull", "", token); w.Code != http.StatusServiceUnavailable {
		t.Errorf("without a puller: %d, want 503", w.Code)
	}
}

// The route set that holds other people's credentials is behind the same
// token as everything else, and absent without one.
func TestThePanelNeedsTheToken(t *testing.T) {
	h, _ := newServer(t, panelConfig())
	for _, check := range []func() *httptest.ResponseRecorder{
		func() *httptest.ResponseRecorder { return get(t, h, "/connections", "") },
		func() *httptest.ResponseRecorder { return post(t, h, "/connections", jiraConnection, "wrong") },
		func() *httptest.ResponseRecorder { return forget(t, h, "01M4D3H3HNMFM69N4MHNAYBZ1Z", "") },
		func() *httptest.ResponseRecorder { return post(t, h, "/connections/pull", "", "wrong") },
	} {
		if w := check(); w.Code != http.StatusUnauthorized {
			t.Errorf("%d, want 401", w.Code)
		}
	}

	noToken, _ := newServer(t, Config{TelegramSecret: telegramSecret, TelegramProject: "evomem"})
	if w := get(t, noToken, "/connections", token); w.Code != http.StatusNotFound {
		t.Errorf("without a configured token: %d, want 404", w.Code)
	}
}
