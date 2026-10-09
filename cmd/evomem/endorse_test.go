package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// taintedNote stores one note the way an adapter would.
func taintedNote(t *testing.T, origin string, transcribed bool) string {
	t.Helper()
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	n := &models.Note{
		ProjectID:  "evomem",
		Content:    "the tunnel has to be running first",
		SourceType: models.SourceType(origin),
	}
	n.MarkTainted(origin)
	if transcribed {
		n.SetMeta(models.MetaTranscribed, true)
	}
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return n.ID
}

func storedNote(t *testing.T, id string) *models.Note {
	t.Helper()
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	n, err := db.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEndorseClearsTheMark(t *testing.T) {
	withStore(t)
	id := taintedNote(t, "telegram", false)

	out := exec(t, "", "endorse", id)
	// The content is shown before the decision: endorsing without reading
	// is the one thing the command cannot prevent.
	if !strings.Contains(out, "the tunnel has to be running first") {
		t.Errorf("the note was not shown: %q", out)
	}
	if !strings.Contains(out, "came from telegram") {
		t.Errorf("the origin was not shown: %q", out)
	}
	if !strings.Contains(out, "endorsed") {
		t.Errorf("got %q", out)
	}
	// And it says the one-way door is one-way.
	if !strings.Contains(out, "no way to undo") {
		t.Errorf("it does not say this cannot be undone: %q", out)
	}

	note := storedNote(t, id)
	if note.Tainted() {
		t.Error("still tainted")
	}
	if !note.Endorsed() {
		t.Error("not recorded as endorsed")
	}
	if note.Metadata[models.MetaWasTainted] != true {
		t.Error("the note does not record that it was tainted")
	}
}

func TestEndorseSaysATranscriptStaysMarked(t *testing.T) {
	withStore(t)
	id := taintedNote(t, "telegram", true)

	out := exec(t, "", "endorse", id)
	if !strings.Contains(out, "machine transcription") {
		t.Errorf("got %q", out)
	}
	if !storedNote(t, id).Transcribed() {
		t.Error("the transcription mark was cleared")
	}
}

func TestEndorseDryRunChangesNothing(t *testing.T) {
	withStore(t)
	id := taintedNote(t, "jira", false)

	out := exec(t, "", "endorse", "-dry-run", id)
	if !strings.Contains(out, "would clear") {
		t.Errorf("got %q", out)
	}
	if !storedNote(t, id).Tainted() {
		t.Error("a dry run cleared the mark")
	}
}

// Not an error: the person asked for a state the note is already in.
func TestEndorseAnUntaintedNoteSaysSo(t *testing.T) {
	withStore(t)
	exec(t, "", "add", "-project", "evomem", "typed by hand")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	notes, err := db.List(context.Background(), database.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if len(notes) != 1 {
		t.Fatalf("%d notes", len(notes))
	}

	out := exec(t, "", "endorse", notes[0].ID)
	if !strings.Contains(out, "nothing to endorse") {
		t.Errorf("got %q", out)
	}
}

func TestEndorseArguments(t *testing.T) {
	withStore(t)
	id := taintedNote(t, "telegram", false)

	for _, args := range [][]string{
		{"endorse"},
		{"endorse", id, "extra"},
		{"endorse", "not-a-ulid"},
		{"endorse", "01M4D3H3HNMFM69N4MHNAYBZ1Z"},
	} {
		var out bytes.Buffer
		if err := run(args, &out, strings.NewReader("")); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

func TestUsageMentionsEndorse(t *testing.T) {
	withStore(t)
	out := exec(t, "", "help")
	if !strings.Contains(out, "evomem endorse") {
		t.Error("usage does not mention endorse")
	}
}
