package jira

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

type fakeStore struct {
	notes []*models.Note
	err   error
}

func (f *fakeStore) Create(_ context.Context, n *models.Note) error {
	if f.err != nil {
		return f.err
	}
	n.ID = models.NewULID()
	f.notes = append(f.notes, n)
	return nil
}

const secret = "jira-shared-secret"

func newHandler(t *testing.T) (*Handler, *fakeStore) {
	t.Helper()
	store := &fakeStore{}
	h, err := New(store, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	return h, store
}

// sign is what Jira does: HMAC-SHA256 over the body, hex, with the method as
// a prefix.
func sign(body, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func post(t *testing.T, h *Handler, body, signature string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/jira/webhook", strings.NewReader(body))
	if signature != "" {
		r.Header.Set(SignatureHeader, signature)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func send(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h, body, sign(body, secret))
}

func TestNewRefusesWithoutSecret(t *testing.T) {
	if _, err := New(&fakeStore{}, "", ""); err == nil {
		t.Error("a handler was built with no secret")
	}
	if _, err := New(nil, secret, ""); err == nil {
		t.Error("a handler was built with no store")
	}
}

const issueCreated = `{"webhookEvent":"jira:issue_created","issue_event_type_name":"issue_created",
	"timestamp":1760000000000,
	"user":{"accountId":"abc","displayName":"Ali"},
	"issue":{"id":"10001","key":"EVO-12","fields":{
		"summary":"sync worker must check connectivity first",
		"description":"It currently writes before checking.",
		"status":{"name":"To Do"},
		"priority":{"name":"High"},
		"issuetype":{"name":"Task"},
		"project":{"key":"EVO","name":"Evomem"},
		"assignee":{"accountId":"def","displayName":"Ali Osman"},
		"labels":["sync","phase5"]}}}`

// The signature is the only thing between this endpoint and anyone who learns
// the URL, so every way of getting it wrong has to be a 403.
func TestRejectsBadSignature(t *testing.T) {
	h, store := newHandler(t)

	cases := map[string]string{
		"absent":           "",
		"empty":            " ",
		"no method prefix": hex.EncodeToString([]byte("whatever")),
		"wrong key":        sign(issueCreated, "not-the-secret"),
		"not hex":          "sha256=zzzz",
		"truncated":        sign(issueCreated, secret)[:20],
		// A sender must not get to choose a weaker algorithm.
		"md5 method":  strings.Replace(sign(issueCreated, secret), "sha256=", "md5=", 1),
		"sha1 method": strings.Replace(sign(issueCreated, secret), "sha256=", "sha1=", 1),
	}
	for name, signature := range cases {
		w := post(t, h, issueCreated, signature)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s signature got %d, want 403", name, w.Code)
		}
	}
	if len(store.notes) != 0 {
		t.Errorf("%d notes were written despite a bad signature", len(store.notes))
	}
}

// The signature covers the body, so changing one byte of it has to invalidate
// the whole delivery.
func TestRejectsTamperedBody(t *testing.T) {
	h, _ := newHandler(t)

	good := sign(issueCreated, secret)
	tampered := strings.Replace(issueCreated, "EVO-12", "EVO-99", 1)
	if w := post(t, h, tampered, good); w.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", w.Code)
	}
}

// The method prefix is case-insensitive in the header's own format.
func TestAcceptsUppercaseMethod(t *testing.T) {
	h, _ := newHandler(t)

	signature := strings.Replace(sign(issueCreated, secret), "sha256=", "SHA256=", 1)
	if w := post(t, h, issueCreated, signature); w.Code != http.StatusOK {
		t.Errorf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestRejectsNonPost(t *testing.T) {
	h, _ := newHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/jira/webhook", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", w.Code)
	}
}

func TestIssueCreated(t *testing.T) {
	h, store := newHandler(t)

	if w := send(t, h, issueCreated); w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if len(store.notes) != 1 {
		t.Fatalf("%d notes were written, want 1", len(store.notes))
	}

	n := store.notes[0]
	// The issue already says which project it belongs to, and that is a
	// better answer than anything configured here.
	if n.ProjectID != "EVO" {
		t.Errorf("project is %q, want EVO", n.ProjectID)
	}
	if n.SourceType != models.SourceJira {
		t.Errorf("source is %q, want jira", n.SourceType)
	}
	// A note that reads "status changed" and nothing else is unsearchable,
	// so the key and the summary are always in the content.
	if !strings.Contains(n.Content, "EVO-12") {
		t.Errorf("the issue key is not in the content: %q", n.Content)
	}
	if !strings.Contains(n.Content, "sync worker must check connectivity") {
		t.Errorf("the summary is not in the content: %q", n.Content)
	}
	if !strings.Contains(n.Content, "writes before checking") {
		t.Errorf("the description is not in the content: %q", n.Content)
	}

	if !n.Tainted() {
		t.Error("a Jira field was not marked tainted")
	}
	if got, _ := n.MetaString("jira_issue_key"); got != "EVO-12" {
		t.Errorf("jira_issue_key is %q", got)
	}
	if got, _ := n.MetaString("status"); got != "To Do" {
		t.Errorf("status is %q", got)
	}
	if got, _ := n.MetaString("jira_priority"); got != "High" {
		t.Errorf("priority is %q", got)
	}
	if got, _ := n.MetaString("jira_assignee"); got != "Ali Osman" {
		t.Errorf("assignee is %q", got)
	}
	if got, _ := n.MetaString("jira_event"); got != "jira:issue_created" {
		t.Errorf("event is %q", got)
	}
}

// One note per event, not one issue kept up to date: the history is the part
// that answers "why is it like this".
func TestIssueUpdatedRecordsTheChange(t *testing.T) {
	h, store := newHandler(t)

	body := `{"webhookEvent":"jira:issue_updated",
		"issue":{"key":"EVO-12","fields":{"summary":"the sync worker","project":{"key":"EVO"},
			"status":{"name":"In Progress"}}},
		"changelog":{"items":[
			{"field":"status","fromString":"To Do","toString":"In Progress"},
			{"field":"assignee","fromString":"","toString":"Ali Osman"}]}}`
	send(t, h, body)

	n := store.notes[0]
	if !strings.Contains(n.Content, "status: To Do -> In Progress") {
		t.Errorf("the change is not in the content: %q", n.Content)
	}
	// An empty side of a change has to read as something, not as nothing.
	if !strings.Contains(n.Content, "assignee: (empty) -> Ali Osman") {
		t.Errorf("an empty from side is unreadable: %q", n.Content)
	}
}

func TestComment(t *testing.T) {
	h, store := newHandler(t)

	body := `{"webhookEvent":"comment_created",
		"issue":{"key":"EVO-12","fields":{"summary":"the sync worker","project":{"key":"EVO"}}},
		"comment":{"id":"5","author":{"displayName":"Ali"},"body":"this is blocked on the tunnel"}}`
	send(t, h, body)

	n := store.notes[0]
	if !strings.Contains(n.Content, "Comment by Ali") {
		t.Errorf("the author is not in the content: %q", n.Content)
	}
	if !strings.Contains(n.Content, "blocked on the tunnel") {
		t.Errorf("the comment body is not in the content: %q", n.Content)
	}
}

// A text field is a string on some instances and an Atlassian Document Format
// tree on others, depending on the API version the webhook was made under.
func TestAtlassianDocumentFormat(t *testing.T) {
	h, store := newHandler(t)

	body := `{"webhookEvent":"jira:issue_created",
		"issue":{"key":"EVO-13","fields":{"summary":"adf description","project":{"key":"EVO"},
			"description":{"type":"doc","version":1,"content":[
				{"type":"paragraph","content":[{"type":"text","text":"first paragraph"}]},
				{"type":"paragraph","content":[{"type":"text","text":"second paragraph"}]}]}}}}`
	send(t, h, body)

	n := store.notes[0]
	if !strings.Contains(n.Content, "first paragraph") || !strings.Contains(n.Content, "second paragraph") {
		t.Errorf("the document was not flattened: %q", n.Content)
	}
	// Without a break between blocks every paragraph runs into the next
	// one word-first.
	if strings.Contains(n.Content, "paragraphsecond") {
		t.Errorf("the paragraphs ran together: %q", n.Content)
	}
}

// Jira sends shapes this adapter does not read, and a project event has no
// issue to file anything under.
func TestEventsWithNothingToStore(t *testing.T) {
	h, store := newHandler(t)

	for name, body := range map[string]string{
		"a project event": `{"webhookEvent":"project_created","project":{"key":"EVO"}}`,
		"no issue key":    `{"webhookEvent":"jira:issue_created","issue":{"id":"1","fields":{"summary":"x"}}}`,
		"empty":           `{}`,
	} {
		if w := send(t, h, body); w.Code != http.StatusOK {
			t.Errorf("%s got %d, want 200", name, w.Code)
		}
	}
	if len(store.notes) != 0 {
		t.Errorf("%d notes were written, want none", len(store.notes))
	}
}

// Without a project object, the key's own prefix is the project.
func TestProjectFromIssueKey(t *testing.T) {
	h, store := newHandler(t)

	body := `{"webhookEvent":"jira:issue_created","issue":{"key":"PEND-3","fields":{"summary":"x"}}}`
	send(t, h, body)

	if store.notes[0].ProjectID != "PEND" {
		t.Errorf("project is %q, want PEND", store.notes[0].ProjectID)
	}
}

// A configured project overrides the issue's own, for an instance whose keys
// are not what this store files notes under.
func TestConfiguredProjectWins(t *testing.T) {
	store := &fakeStore{}
	h, err := New(store, secret, "evomem")
	if err != nil {
		t.Fatal(err)
	}
	send(t, h, issueCreated)

	if store.notes[0].ProjectID != "evomem" {
		t.Errorf("project is %q, want evomem", store.notes[0].ProjectID)
	}
}

func TestMalformedBody(t *testing.T) {
	h, _ := newHandler(t)

	body := `{not json`
	if w := post(t, h, body, sign(body, secret)); w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}

func TestStoreFailureAsksForARetry(t *testing.T) {
	store := &fakeStore{err: context.DeadlineExceeded}
	h, err := New(store, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	if w := send(t, h, issueCreated); w.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", w.Code)
	}
}

// A signature cannot be checked without the whole body in hand, so the bound
// on it is what stops this endpoint being a way to exhaust memory.
func TestOversizedBodyIsRejected(t *testing.T) {
	h, store := newHandler(t)

	body := `{"webhookEvent":"x","padding":"` + strings.Repeat("a", maxBody+1) + `"}`
	w := post(t, h, body, sign(body, secret))
	if w.Code == http.StatusOK {
		t.Error("an oversized payload was accepted")
	}
	if len(store.notes) != 0 {
		t.Error("an oversized payload was stored")
	}
}

// The header name and the signature scheme are not guesses: they are what
// Jira Cloud sends, verified against the platform documentation.
func TestSignatureHeaderName(t *testing.T) {
	if SignatureHeader != "X-Hub-Signature" {
		t.Errorf("SignatureHeader is %q", SignatureHeader)
	}
}
