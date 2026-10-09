package connect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
)

// seenRequest is what the fake Jira saw, so the tests assert on the wire
// format the vendor documents rather than on this package's idea of it.
type seenRequest struct {
	path  string
	query map[string]string
	auth  string
}

func fakeJira(t *testing.T, pages []jiraPage) (*httptest.Server, *[]seenRequest) {
	t.Helper()
	var seen []seenRequest
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		one := seenRequest{path: r.URL.Path, query: map[string]string{}, auth: r.Header.Get("Authorization")}
		for key, values := range r.URL.Query() {
			one.query[key] = values[0]
		}
		seen = append(seen, one)

		if page >= len(pages) {
			http.Error(w, "no more pages", http.StatusInternalServerError)
			return
		}
		body, _ := json.Marshal(pages[page])
		page++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func connectionTo(url string) *database.Connection {
	return &database.Connection{
		SourceType: "jira",
		ProjectID:  "evomem",
		BaseURL:    url,
		Account:    "someone@example.com",
		Query:      "project = EVO ORDER BY updated DESC",
	}
}

func issue(key, summary string) jiraIssue {
	var one jiraIssue
	one.ID = "10001"
	one.Key = key
	one.Fields.Summary = summary
	one.Fields.Status.Name = "In Progress"
	return one
}

func TestJiraSendsWhatTheVendorDocuments(t *testing.T) {
	srv, seen := fakeJira(t, []jiraPage{{Issues: []jiraIssue{issue("EVO-12", "sync worker")}, IsLast: true}})

	notes, cursor, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "the-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("%d notes", len(notes))
	}
	if cursor != "" {
		t.Errorf("cursor = %q after the last page", cursor)
	}

	request := (*seen)[0]
	if request.path != "/rest/api/3/search/jql" {
		t.Errorf("path = %q", request.path)
	}
	if request.query["jql"] != "project = EVO ORDER BY updated DESC" {
		t.Errorf("jql = %q", request.query["jql"])
	}
	if request.query["fields"] == "" || request.query["maxResults"] == "" {
		t.Errorf("query = %v", request.query)
	}
	// email:token, base64, as the basic-auth page documents.
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("someone@example.com:the-token"))
	if request.auth != want {
		t.Errorf("authorization = %q", request.auth)
	}
}

// Sending it on the first call has been refused as invalid, so it is omitted.
func TestJiraOmitsThePageTokenOnTheFirstRequest(t *testing.T) {
	srv, seen := fakeJira(t, []jiraPage{
		{Issues: []jiraIssue{issue("EVO-1", "one")}, NextPageToken: "page-2"},
		{Issues: []jiraIssue{issue("EVO-2", "two")}, IsLast: true},
	})

	if _, _, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "t"); err != nil {
		t.Fatal(err)
	}
	if _, sent := (*seen)[0].query["nextPageToken"]; sent {
		t.Error("the first request carried a page token")
	}
	if (*seen)[1].query["nextPageToken"] != "page-2" {
		t.Errorf("the second request carried %q", (*seen)[1].query["nextPageToken"])
	}
}

// isLast is reported unreliable, so an absent token ends the loop too.
func TestJiraStopsWhenTheTokenRunsOut(t *testing.T) {
	srv, seen := fakeJira(t, []jiraPage{
		{Issues: []jiraIssue{issue("EVO-1", "one")}, NextPageToken: "page-2"},
		{Issues: []jiraIssue{issue("EVO-2", "two")}}, // no token, isLast false
	})

	notes, _, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Errorf("%d notes", len(notes))
	}
	if len(*seen) != 2 {
		t.Errorf("%d requests", len(*seen))
	}
}

// A token that repeats is a loop, which has been reported in the wild.
func TestJiraStopsOnARepeatedToken(t *testing.T) {
	pages := make([]jiraPage, 10)
	for i := range pages {
		pages[i] = jiraPage{Issues: []jiraIssue{issue("EVO-1", "one")}, NextPageToken: "same"}
	}
	srv, seen := fakeJira(t, pages)

	notes, _, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "t")
	if err != nil {
		t.Fatal(err)
	}
	// Two requests: the first, and the one that proved the token repeats.
	if len(*seen) != 2 {
		t.Errorf("%d requests, want 2", len(*seen))
	}
	if len(notes) != 2 {
		t.Errorf("%d notes", len(notes))
	}
}

func TestJiraKeepsTheCursorWhenItRunsOutOfPages(t *testing.T) {
	pages := make([]jiraPage, jiraMaxPages+2)
	for i := range pages {
		pages[i] = jiraPage{
			Issues:        []jiraIssue{issue("EVO-1", "one")},
			NextPageToken: "page-" + string(rune('a'+i)),
		}
	}
	srv, _ := fakeJira(t, pages)

	notes, cursor, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != jiraMaxPages {
		t.Errorf("%d notes", len(notes))
	}
	// Kept, so the next run carries on rather than starting over.
	if cursor == "" {
		t.Error("the cursor was dropped when the page limit was reached")
	}
}

func TestJiraRefusesAConnectionWithNoQuery(t *testing.T) {
	srv, seen := fakeJira(t, nil)
	conn := connectionTo(srv.URL)
	conn.Query = "   "

	if _, _, err := NewJiraPuller().Pull(context.Background(), conn, "t"); err == nil {
		t.Fatal("a connection with no query was accepted")
	}
	if len(*seen) != 0 {
		t.Error("it called Jira anyway")
	}
}

func TestJiraReportsWhatTheServerSaid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errorMessages":["Issue does not exist"]}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	_, _, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "t")
	if err == nil {
		t.Fatal("a 400 was accepted")
	}
	if !strings.Contains(err.Error(), "Issue does not exist") {
		t.Errorf("err = %v; it should quote what Jira said", err)
	}
}

// The summary is the sentence somebody wrote; the rest is Jira's bookkeeping
// and belongs in the metadata.
func TestAnIssueBecomesAReadableNote(t *testing.T) {
	one := issue("EVO-12", "sync worker baglanti kontrolu")
	one.Fields.Updated = "2026-10-09T06:00:00.000+0000"
	one.Fields.Description = map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":    "paragraph",
				"content": []any{map[string]any{"type": "text", "text": "the tunnel has to be up"}},
			},
		},
	}
	srv, _ := fakeJira(t, []jiraPage{{Issues: []jiraIssue{one}, IsLast: true}})

	notes, _, err := NewJiraPuller().Pull(context.Background(), connectionTo(srv.URL), "t")
	if err != nil {
		t.Fatal(err)
	}
	note := notes[0]
	if note.Content != "EVO-12: sync worker baglanti kontrolu" {
		t.Errorf("content = %q", note.Content)
	}
	if note.Metadata["jira_issue_key"] != "EVO-12" || note.Metadata["jira_status"] != "In Progress" {
		t.Errorf("metadata = %v", note.Metadata)
	}
	// Atlassian Document Format is a tree; only its text leaves are kept.
	if note.Metadata["jira_description"] != "the tunnel has to be up" {
		t.Errorf("description = %v", note.Metadata["jira_description"])
	}
	if note.CreatedAt.IsZero() {
		t.Error("the issue's updated time was not read")
	}
}

func TestPlainTextWalksWhatItIsGiven(t *testing.T) {
	if got := plainText(nil); got != "" {
		t.Errorf("nil = %q", got)
	}
	if got := plainText("already text"); got != "already text" {
		t.Errorf("string = %q", got)
	}
	nested := map[string]any{
		"content": []any{
			map[string]any{"text": "one"},
			map[string]any{"content": []any{map[string]any{"text": "two"}}},
		},
	}
	if got := plainText(nested); got != "one two" {
		t.Errorf("nested = %q", got)
	}
}
