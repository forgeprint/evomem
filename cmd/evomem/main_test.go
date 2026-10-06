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
