package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/forgeprint/evomem/core/api/adapters/jira"
	"github.com/forgeprint/evomem/core/api/adapters/telegram"
	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

const (
	token          = "an-api-token"
	telegramSecret = "a-telegram-secret"
	jiraSecret     = "a-jira-secret"
)

// newServer builds a server over a real store, so these tests exercise the
// ingestion path end to end rather than a fake.
func newServer(t *testing.T, cfg Config) (http.Handler, *database.DB) {
	t.Helper()
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s, err := New(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler(), db
}

func fullConfig() Config {
	return Config{
		Token:           token,
		DefaultProject:  "evomem",
		TelegramSecret:  telegramSecret,
		TelegramProject: "evomem",
		JiraSecret:      jiraSecret,
	}
}

func do(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func ingest(t *testing.T, h http.Handler, body, contentType, bearer, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/ingest"
	if query != "" {
		url += "?" + query
	}
	r := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

func TestNewRefusesAnEmptyConfiguration(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A server with no endpoint is never what the user meant.
	if _, err := New(db, Config{}); err == nil {
		t.Error("a server with nothing configured was built")
	}
	if _, err := New(nil, fullConfig()); err == nil {
		t.Error("a server with no store was built")
	}
}

// Loopback by default, so a laptop on a café network is not running an open
// ingest endpoint by accident.
func TestDefaultAddrIsLoopback(t *testing.T) {
	if !strings.HasPrefix(DefaultAddr, "127.0.0.1:") {
		t.Errorf("DefaultAddr is %q; it must not listen on every interface", DefaultAddr)
	}

	db, _ := database.OpenMemory()
	defer db.Close()
	s, err := New(db, Config{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	if s.Addr() != DefaultAddr {
		t.Errorf("Addr is %q, want %s", s.Addr(), DefaultAddr)
	}
}

// An endpoint whose secret is missing is not registered at all: the failure
// is a 404 at a known address, not an unauthenticated way to write notes.
func TestEndpointsWithoutSecretsAreNotServed(t *testing.T) {
	h, _ := newServer(t, Config{Token: token, DefaultProject: "evomem"})

	for _, path := range []string{"/telegram/webhook", "/jira/webhook"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		if w := do(h, r); w.Code != http.StatusNotFound {
			t.Errorf("%s got %d with no secret configured, want 404", path, w.Code)
		}
	}
}

// A Telegram secret without a project is not enough to know where notes go.
func TestTelegramNeedsAProject(t *testing.T) {
	h, _ := newServer(t, Config{Token: token, TelegramSecret: telegramSecret})

	r := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader("{}"))
	if w := do(h, r); w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", w.Code)
	}
}

func TestRoutesAreReported(t *testing.T) {
	db, _ := database.OpenMemory()
	defer db.Close()

	s, err := New(db, fullConfig())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(s.Routes(), " ")
	for _, want := range []string{"/ingest", "/telegram/webhook", "/jira/webhook"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s is not reported in %q", want, got)
		}
	}
}

func TestHealthzNeedsNoCredential(t *testing.T) {
	h, _ := newServer(t, fullConfig())

	w := do(h, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}
	// It says the process is up and nothing else.
	if strings.Contains(w.Body.String(), "evomem.db") {
		t.Errorf("healthz leaks the store path: %s", w.Body.String())
	}
}

func TestIngestRejectsBadCredentials(t *testing.T) {
	h, db := newServer(t, fullConfig())

	cases := map[string]string{
		"no header":    "",
		"wrong token":  "Bearer not-the-token",
		"longer token": "Bearer " + token + "x",
		"no scheme":    token,
		"wrong scheme": "Basic " + token,
		"empty bearer": "Bearer ",
	}
	for name, header := range cases {
		r := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(`{"content":"x"}`))
		r.Header.Set("Content-Type", "application/json")
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if w := do(h, r); w.Code != http.StatusUnauthorized {
			t.Errorf("%s got %d, want 401", name, w.Code)
		}
	}

	count, err := db.Count(context.Background(), database.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d notes were written without a credential", count)
	}
}

// Bearer is a scheme name and is case-insensitive.
func TestIngestAcceptsAnyCaseOfBearer(t *testing.T) {
	h, _ := newServer(t, fullConfig())

	r := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(`{"content":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "bearer "+token)
	if w := do(h, r); w.Code != http.StatusCreated {
		t.Errorf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngestJSON(t *testing.T) {
	h, db := newServer(t, fullConfig())

	body := `{"project":"pendra","content":"remember the tunnel","source":"shortcut",
		"metadata":{"shortcut_name":"Capture"}}`
	w := ingest(t, h, body, "application/json", token, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	var reply struct {
		Status  string `json:"status"`
		ID      string `json:"id"`
		Project string `json:"project"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Project != "pendra" {
		t.Errorf("project is %q", reply.Project)
	}

	n, err := db.Get(context.Background(), reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Content != "remember the tunnel" {
		t.Errorf("content is %q", n.Content)
	}
	if n.SourceType != models.SourceShortcut {
		t.Errorf("source is %q", n.SourceType)
	}
	if got, _ := n.MetaString("shortcut_name"); got != "Capture" {
		t.Errorf("metadata is %v", n.Metadata)
	}
	// This arrived under the user's own token from the user's own device,
	// carrying what the user dictated. That is not third-party text.
	if n.Tainted() {
		t.Error("a note the user dictated was marked tainted")
	}
}

// A Shortcut can post JSON, and it can just as easily post the text it
// captured with no structure at all. Making the second an error would mean
// every Shortcut starts with a dictionary block for no reason.
func TestIngestPlainText(t *testing.T) {
	h, db := newServer(t, fullConfig())

	w := ingest(t, h, "  dictated into the watch  ", "text/plain", token, "project=pendra")
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	notes, err := db.List(context.Background(), database.ListOptions{ProjectID: "pendra"})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("%d notes were written, want 1", len(notes))
	}
	if notes[0].Content != "dictated into the watch" {
		t.Errorf("content is %q", notes[0].Content)
	}
}

// A body with no content type at all is what a minimal Shortcut sends.
func TestIngestPlainTextWithoutContentType(t *testing.T) {
	h, _ := newServer(t, fullConfig())

	if w := ingest(t, h, "no content type", "", token, ""); w.Code != http.StatusCreated {
		t.Errorf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngestFallsBackToTheDefaultProject(t *testing.T) {
	h, db := newServer(t, fullConfig())

	ingest(t, h, `{"content":"no project given"}`, "application/json", token, "")

	notes, err := db.List(context.Background(), database.ListOptions{ProjectID: "evomem"})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Errorf("%d notes landed in the default project, want 1", len(notes))
	}
}

func TestIngestWithoutAnyProject(t *testing.T) {
	h, _ := newServer(t, Config{Token: token})

	w := ingest(t, h, `{"content":"x"}`, "application/json", token, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "project") {
		t.Errorf("the error does not say what is missing: %s", w.Body.String())
	}
}

// An empty note is the mistake a half-built Shortcut makes, so the reason has
// to come back rather than a bare 400.
func TestIngestRejectsEmptyContent(t *testing.T) {
	h, _ := newServer(t, fullConfig())

	for name, body := range map[string]string{
		"empty json":      `{"content":""}`,
		"whitespace json": `{"content":"   "}`,
		"empty body":      ``,
		"whitespace body": `   `,
	} {
		contentType := "application/json"
		if !strings.HasPrefix(strings.TrimSpace(body), "{") {
			contentType = "text/plain"
		}
		w := ingest(t, h, body, contentType, token, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s got %d, want 400", name, w.Code)
		}
	}
}

func TestIngestRejectsMalformedJSON(t *testing.T) {
	h, _ := newServer(t, fullConfig())

	w := ingest(t, h, `{not json`, "application/json", token, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}

func TestIngestRejectsNonPost(t *testing.T) {
	h, _ := newServer(t, fullConfig())

	r := httptest.NewRequest(http.MethodGet, "/ingest", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if w := do(h, r); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", w.Code)
	}
}

// The bound is what stops an endpoint on the open internet being a way to
// exhaust memory.
func TestIngestRejectsOversizedBody(t *testing.T) {
	h, db := newServer(t, fullConfig())

	w := ingest(t, h, strings.Repeat("a", maxBody+1), "text/plain", token, "")
	if w.Code == http.StatusCreated {
		t.Error("an oversized body was accepted")
	}
	count, _ := db.Count(context.Background(), database.ListOptions{})
	if count != 0 {
		t.Error("an oversized body was stored")
	}
}

// The adapters are wired to the same store, so a webhook delivery has to be
// readable afterwards through the ordinary queries.
func TestTelegramThroughTheServer(t *testing.T) {
	h, db := newServer(t, fullConfig())

	body := `{"update_id":1,"message":{"message_id":1,"chat":{"id":5},"text":"from telegram"}}`
	r := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	r.Header.Set(telegram.SecretHeader, telegramSecret)
	if w := do(h, r); w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	hits, err := db.Search(context.Background(), database.SearchQuery{Text: "telegram"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("%d hits, want 1", len(hits))
	}
	if !hits[0].Note.Tainted() {
		t.Error("the note is not marked tainted")
	}
}

func TestJiraThroughTheServer(t *testing.T) {
	h, db := newServer(t, fullConfig())

	body := `{"webhookEvent":"jira:issue_created","issue":{"key":"EVO-1",
		"fields":{"summary":"from jira","project":{"key":"EVO"}}}}`
	mac := hmac.New(sha256.New, []byte(jiraSecret))
	mac.Write([]byte(body))

	r := httptest.NewRequest(http.MethodPost, "/jira/webhook", strings.NewReader(body))
	r.Header.Set(jira.SignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if w := do(h, r); w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	notes, err := db.List(context.Background(), database.ListOptions{ProjectID: "EVO"})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("%d notes, want 1", len(notes))
	}
}

// The server has to come up on a real port and go down when its context is
// cancelled, or nothing can supervise it.
func TestListenAndServeStopsOnContextCancel(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Port 0: the kernel picks one, and Addr reports which.
	s, err := New(db, Config{Addr: "127.0.0.1:0", Token: token, DefaultProject: "evomem"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx) }()

	// Wait for the listener, then check the reported address works.
	var resp *http.Response
	for i := 0; i < 100; i++ {
		resp, err = http.Get("http://" + s.Addr() + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("the server never answered on %s: %v", s.Addr(), err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz returned %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("shutting down returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("the server did not stop when its context was cancelled")
	}
}
