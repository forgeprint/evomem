package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// withStore points the command at a scratch database for the duration of a
// test, so nothing touches the developer's own store.
func withStore(t *testing.T) {
	t.Helper()
	t.Setenv("EVOMEM_DB", filepath.Join(t.TempDir(), "evomem.db"))
}

func exec(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out, strings.NewReader(stdin)); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

func TestVersion(t *testing.T) {
	if got := exec(t, "", "version"); strings.TrimSpace(got) != version {
		t.Errorf("got %q, want %q", got, version)
	}
}

func TestNoArgsPrintsUsage(t *testing.T) {
	if got := exec(t, ""); !strings.Contains(got, "usage:") {
		t.Errorf("got %q", got)
	}
}

func TestUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"frobnicate"}, &out, strings.NewReader("")); err == nil {
		t.Error("an unknown command was accepted")
	}
}

func TestAddThenSearch(t *testing.T) {
	withStore(t)

	id := strings.TrimSpace(exec(t, "", "add", "-project", "evomem", "the gateway timed out"))
	if id == "" {
		t.Fatal("add printed no identifier")
	}

	var hits []struct {
		Note struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"note"`
		Snippet string `json:"snippet"`
	}
	if err := json.Unmarshal([]byte(exec(t, "", "search", "-query", "gateway")), &hits); err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if hits[0].Note.ID != id {
		t.Errorf("found %s, want %s", hits[0].Note.ID, id)
	}
}

func TestAddFromStdin(t *testing.T) {
	withStore(t)

	exec(t, "  piped note  \n", "add", "-project", "evomem", "-")

	out := exec(t, "", "list")
	if !strings.Contains(out, "piped note") {
		t.Errorf("the piped note is not in the listing: %s", out)
	}
	if strings.Contains(out, "  piped note") {
		t.Error("the piped note kept its surrounding whitespace")
	}
}

func TestAddWithoutText(t *testing.T) {
	withStore(t)

	var out bytes.Buffer
	if err := run([]string{"add", "-project", "evomem"}, &out, strings.NewReader("")); err == nil {
		t.Error("a note with no text was accepted")
	}
}

func TestAddWithoutProject(t *testing.T) {
	withStore(t)

	var out bytes.Buffer
	if err := run([]string{"add", "something"}, &out, strings.NewReader("")); err == nil {
		t.Error("a note with no project was accepted")
	}
}

func TestSearchWithoutQuery(t *testing.T) {
	withStore(t)

	var out bytes.Buffer
	if err := run([]string{"search"}, &out, strings.NewReader("")); err == nil {
		t.Error("a search with no query was accepted")
	}
}

// No results is an empty array, not null: anything parsing this output should
// be able to iterate it without a nil check.
func TestEmptyResultsAreAnArray(t *testing.T) {
	withStore(t)

	for _, args := range [][]string{
		{"search", "-query", "nothingmatchesthis"},
		{"list"},
		{"projects"},
	} {
		if got := strings.TrimSpace(exec(t, "", args...)); got != "[]" {
			t.Errorf("%v printed %q, want []", args, got)
		}
	}
}

func TestProjects(t *testing.T) {
	withStore(t)

	exec(t, "", "add", "-project", "evomem", "bir")
	exec(t, "", "add", "-project", "pendra", "iki")

	var projects []struct {
		ProjectID string `json:"project_id"`
		Notes     int    `json:"notes"`
	}
	if err := json.Unmarshal([]byte(exec(t, "", "projects")), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
}

func TestInit(t *testing.T) {
	withStore(t)

	if got := exec(t, "", "init"); !strings.Contains(got, "evomem.db") {
		t.Errorf("got %q", got)
	}
}

func TestSyncStatusOnAFreshStore(t *testing.T) {
	withStore(t)

	out := exec(t, "", "sync-status")
	if !strings.Contains(out, "nothing has been synced") {
		t.Errorf("a fresh store does not say so: %s", out)
	}
	if !strings.Contains(out, "notes      0 pending") {
		t.Errorf("got %s", out)
	}
}

func TestSyncStatusCountsPending(t *testing.T) {
	withStore(t)

	exec(t, "", "add", "-project", "evomem", "bir")
	exec(t, "", "add", "-project", "evomem", "iki")

	out := exec(t, "", "sync-status")
	if !strings.Contains(out, "notes      2 pending") {
		t.Errorf("got %s", out)
	}
}

// Archiving is irreversible, so a cutoff in the future — which would take
// everything — has to be refused rather than obeyed.
func TestArchiveRefusesAFutureCutoff(t *testing.T) {
	withStore(t)

	var out bytes.Buffer
	if err := run([]string{"archive", "-months", "-1"}, &out, strings.NewReader("")); err == nil {
		t.Error("a cutoff in the future was accepted")
	}
}

// An unsynced store archives nothing, and has to say why rather than printing
// a silent zero.
func TestArchiveExplainsWhatItKept(t *testing.T) {
	withStore(t)
	exec(t, "", "add", "-project", "evomem", "bir")

	out := exec(t, "", "archive", "-months", "0")
	if !strings.Contains(out, "removed 0 note(s)") {
		t.Errorf("got %s", out)
	}
	if !strings.Contains(out, "have not been synced") {
		t.Errorf("the reason is missing: %s", out)
	}
}

func TestArchiveDryRunRemovesNothing(t *testing.T) {
	withStore(t)
	exec(t, "", "add", "-project", "evomem", "bir")

	out := exec(t, "", "archive", "-dry-run", "-months", "0")
	if !strings.Contains(out, "dry run") {
		t.Errorf("got %s", out)
	}
	if !strings.Contains(exec(t, "", "list"), "bir") {
		t.Error("the dry run removed the note")
	}
}

func TestArchiveBeforeNeedsAValidInstant(t *testing.T) {
	withStore(t)

	var out bytes.Buffer
	if err := run([]string{"archive", "-before", "last tuesday"}, &out, strings.NewReader("")); err == nil {
		t.Error("an unparseable instant was accepted")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:               "0 B",
		512:             "512 B",
		2048:            "2.0 KiB",
		5 * 1024 * 1024: "5.0 MiB",
		3 * 1073741824:  "3.0 GiB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

// Deleting leaves a tombstone, which is what lets the deletion reach the
// remote copy: an absent row is indistinguishable from one never written.
func TestDeleteLeavesSomethingToSync(t *testing.T) {
	withStore(t)

	id := strings.TrimSpace(exec(t, "", "add", "-project", "evomem", "silinecek"))
	exec(t, "", "delete", id)

	if strings.Contains(exec(t, "", "list"), "silinecek") {
		t.Error("the note is still listed")
	}
	if !strings.Contains(exec(t, "", "sync-status"), "deletions  1 pending") {
		t.Errorf("no tombstone is pending: %s", exec(t, "", "sync-status"))
	}
}

func TestDeleteArguments(t *testing.T) {
	withStore(t)

	for _, args := range [][]string{
		{"delete"},
		{"delete", "a", "b"},
		{"delete", "not-a-ulid"},
	} {
		var out bytes.Buffer
		if err := run(args, &out, strings.NewReader("")); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

// --- the review flow ---

// addProposal puts something in the queue the way an agent would, through the
// MCP server, so this exercises the path a person actually faces.
func proposeThroughMCP(t *testing.T, project, content, extra string) string {
	t.Helper()
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"test-agent","version":"1.0"}}`
	args := `{"project_id":"` + project + `","content":"` + content + `"` + extra + `}`
	line := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` + meta +
		`,"name":"propose_note","arguments":` + args + `}}`

	var out bytes.Buffer
	if err := run([]string{"mcp", "-quiet"}, &out, strings.NewReader(line+"\n")); err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Result struct {
			Structured struct {
				ProposalID string `json:"proposal_id"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if reply.Result.Structured.ProposalID == "" {
		t.Fatalf("no proposal id came back: %s", out.String())
	}
	return reply.Result.Structured.ProposalID
}

func TestReviewListsWhatIsWaiting(t *testing.T) {
	withStore(t)

	if got := exec(t, "", "review"); !strings.Contains(got, "nothing pending") {
		t.Errorf("an empty queue says %q", got)
	}

	id := proposeThroughMCP(t, "evomem", "the tunnel has to be running first", "")

	out := exec(t, "", "review")
	if !strings.Contains(out, id) {
		t.Errorf("the proposal is not listed: %s", out)
	}
	if !strings.Contains(out, "the tunnel has to be running first") {
		t.Errorf("the content is not shown: %s", out)
	}
	// Who asked, so a person can tell which agent it was.
	if !strings.Contains(out, "test-agent 1.0") {
		t.Errorf("the proposer is not shown: %s", out)
	}
	// And the command to accept it, spelled out.
	if !strings.Contains(out, "-accept "+id) {
		t.Errorf("the accept command is not shown: %s", out)
	}
}

// Nothing an agent proposes is in memory until a person says so.
func TestReviewAccept(t *testing.T) {
	withStore(t)

	id := proposeThroughMCP(t, "evomem", "vacuum needs the explicit rowid", "")

	if strings.Contains(exec(t, "", "list"), "vacuum") {
		t.Fatal("a proposal is listed as a note before review")
	}

	out := exec(t, "", "review", "-accept", id)
	if !strings.Contains(out, "stored as note") {
		t.Errorf("got %q", out)
	}
	if !strings.Contains(exec(t, "", "list"), "vacuum") {
		t.Error("the accepted proposal is not a note")
	}
	if !strings.Contains(exec(t, "", "review", "-status", "accepted"), id) {
		t.Error("the accepted proposal is not in the accepted listing")
	}
	if !strings.Contains(exec(t, "", "review"), "nothing pending") {
		t.Error("the proposal is still pending after being accepted")
	}
}

func TestReviewReject(t *testing.T) {
	withStore(t)

	id := proposeThroughMCP(t, "evomem", "something not worth keeping", "")
	exec(t, "", "review", "-reject", id)

	if strings.Contains(exec(t, "", "list"), "not worth keeping") {
		t.Error("a rejected proposal became a note")
	}
	// The record of what was refused is kept.
	if !strings.Contains(exec(t, "", "review", "-status", "rejected"), id) {
		t.Error("the rejected proposal is not in the rejected listing")
	}
}

// The agent's own declaration goes where the person deciding will read it.
func TestReviewShowsTheTaintedClaim(t *testing.T) {
	withStore(t)

	proposeThroughMCP(t, "evomem", "copied from a blog post", `,"tainted":true`)

	out := exec(t, "", "review")
	if !strings.Contains(out, "came from outside") {
		t.Errorf("the claim is not shown to the reviewer: %s", out)
	}
}

func TestReviewArguments(t *testing.T) {
	withStore(t)
	id := proposeThroughMCP(t, "evomem", "something", "")

	for _, args := range [][]string{
		{"review", "-accept", id, "-reject", id},
		{"review", "-accept", "not-a-ulid"},
		{"review", "-reject", "not-a-ulid"},
	} {
		var out bytes.Buffer
		if err := run(args, &out, strings.NewReader("")); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

// A queue nobody is told about is a queue nobody reads.
func TestSyncStatusMentionsTheQueue(t *testing.T) {
	withStore(t)

	if strings.Contains(exec(t, "", "sync-status"), "waiting for review") {
		t.Error("an empty queue was announced")
	}

	proposeThroughMCP(t, "evomem", "something to review", "")
	if !strings.Contains(exec(t, "", "sync-status"), "1 proposal waiting for review") {
		t.Errorf("got %s", exec(t, "", "sync-status"))
	}
}
