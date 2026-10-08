package models

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validNote() Note {
	return Note{ProjectID: "evomem", Content: "bir sey", SourceType: SourceManual}
}

func TestNoteValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Note)
		want error
	}{
		{"valid", func(*Note) {}, nil},
		{"no project", func(n *Note) { n.ProjectID = "" }, ErrEmptyProjectID},
		{"blank project", func(n *Note) { n.ProjectID = "   " }, ErrEmptyProjectID},
		{"no content", func(n *Note) { n.Content = "" }, ErrEmptyContent},
		{"blank content", func(n *Note) { n.Content = "\n\t " }, ErrEmptyContent},
		{"no source", func(n *Note) { n.SourceType = "" }, ErrEmptySourceType},
		{"bad id", func(n *Note) { n.ID = "not-a-ulid" }, ErrInvalidULID},
		{"good id", func(n *Note) { n.ID = NewULID() }, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := validNote()
			c.mut(&n)
			err := n.Validate()
			if !errors.Is(err, c.want) {
				t.Errorf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestNoteValidateProjectIDLength(t *testing.T) {
	n := validNote()
	n.ProjectID = strings.Repeat("a", maxProjectIDLen+1)
	if err := n.Validate(); err == nil {
		t.Error("an over-long project_id was accepted")
	}
}

// An unknown source type is deliberately allowed: an adapter that this build
// does not ship still has to be able to write.
func TestNoteValidateAcceptsUnknownSource(t *testing.T) {
	n := validNote()
	n.SourceType = "some-future-thing"
	if err := n.Validate(); err != nil {
		t.Errorf("an unknown source was rejected: %v", err)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	n := validNote()
	n.SetMeta("jira_issue_key", "EVO-12")
	n.SetMeta("status", "open")

	raw, err := n.MarshalMetadata()
	if err != nil {
		t.Fatal(err)
	}

	var back Note
	if err := back.UnmarshalMetadata(raw); err != nil {
		t.Fatal(err)
	}
	if got, ok := back.MetaString("jira_issue_key"); !ok || got != "EVO-12" {
		t.Errorf("got %q (%v), want EVO-12", got, ok)
	}
}

// NULL, empty and "{}" all have to come back as the same nothing, so no
// reader has to tell them apart.
func TestMetadataEmptyForms(t *testing.T) {
	for _, raw := range []string{"", "{}", "null", "  "} {
		var n Note
		if err := n.UnmarshalMetadata(raw); err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if n.Metadata != nil {
			t.Errorf("%q gave %v, want nil", raw, n.Metadata)
		}
	}
}

func TestMarshalMetadataEmptyIsObject(t *testing.T) {
	n := validNote()
	raw, err := n.MarshalMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if raw != "{}" {
		t.Errorf("got %q, want {}", raw)
	}
}

func TestUnmarshalMetadataRejectsNonsense(t *testing.T) {
	var n Note
	if err := n.UnmarshalMetadata("{not json"); err == nil {
		t.Error("invalid JSON was accepted")
	}
}

func TestMetaStringOnWrongType(t *testing.T) {
	n := validNote()
	n.SetMeta("count", 3)
	if _, ok := n.MetaString("count"); ok {
		t.Error("a number was returned as a string")
	}
	if _, ok := n.MetaString("absent"); ok {
		t.Error("a missing key was reported as present")
	}
}

// A note from an adapter carries text a third party wrote, which will later
// be read by a model. The flag is how a reader knows to treat it as data.
func TestTainted(t *testing.T) {
	n := validNote()
	if n.Tainted() {
		t.Error("a note nobody marked is tainted")
	}

	n.MarkTainted("telegram")
	if !n.Tainted() {
		t.Error("a marked note is not tainted")
	}
	if got, _ := n.MetaString(MetaOrigin); got != "telegram" {
		t.Errorf("origin is %q, want telegram", got)
	}

	// It has to survive the round trip through the metadata column, or the
	// flag is lost the moment the note is read back.
	raw, err := n.MarshalMetadata()
	if err != nil {
		t.Fatal(err)
	}
	var back Note
	if err := back.UnmarshalMetadata(raw); err != nil {
		t.Fatal(err)
	}
	if !back.Tainted() {
		t.Error("the flag did not survive the round trip")
	}
}

// A metadata map that happens to carry a non-boolean under the key is not a
// claim that the note is clean.
func TestTaintedWrongType(t *testing.T) {
	n := validNote()
	n.SetMeta(MetaTainted, "yes")
	if n.Tainted() {
		t.Error("a non-boolean was read as true")
	}
}

func TestApplyTranscriptKeepsWhatItReplaced(t *testing.T) {
	n := &Note{ProjectID: "evomem", Content: "Voice message, 0:14", SourceType: SourceTelegram}
	n.MarkTainted("telegram")
	n.SetMeta(MetaAwaitingTranscription, true)

	at := time.Date(2026, 10, 8, 6, 35, 22, 0, time.UTC)
	n.ApplyTranscript("tünel önce ayakta olmalı", "http://localhost:8001/v1/audio/transcriptions", at)

	if n.Content != "tünel önce ayakta olmalı" {
		t.Errorf("content = %q", n.Content)
	}
	if !n.Transcribed() {
		t.Error("the note does not say it is a transcription")
	}
	// Cleared, which is what makes a second run a no-op.
	if n.AwaitingTranscription() {
		t.Error("still marked as awaiting a transcript")
	}
	if got, _ := n.MetaString(MetaTranscriptionReplaced); got != "Voice message, 0:14" {
		t.Errorf("replaced = %q", got)
	}
	if got, _ := n.MetaString(MetaTranscribedAt); got != "2026-10-08T06:35:22Z" {
		t.Errorf("transcribed_at = %q", got)
	}
	// Where the audio came from did not change, so the taint stays. The
	// two marks are different claims and both are true here.
	if !n.Tainted() {
		t.Error("the taint was dropped")
	}
}

func TestApplyTranscriptClearsAnEarlierFailure(t *testing.T) {
	n := &Note{ProjectID: "evomem", Content: "Voice message, 0:14", SourceType: SourceTelegram}
	n.MarkTranscriptionFailed("the service was down", time.Now())
	n.ApplyTranscript("it worked this time", "svc", time.Now())

	if _, ok := n.MetaString(MetaTranscriptionError); ok {
		t.Error("a stale failure is still on the note")
	}
	if _, ok := n.MetaString(MetaTranscriptionAttemptedAt); ok {
		t.Error("a stale attempt time is still on the note")
	}
}

func TestMarkTranscriptionFailedKeepsItWaiting(t *testing.T) {
	n := &Note{ProjectID: "evomem", Content: "Voice message, 0:14", SourceType: SourceTelegram}
	n.SetMeta(MetaAwaitingTranscription, true)

	at := time.Date(2026, 10, 8, 6, 40, 11, 0, time.UTC)
	n.MarkTranscriptionFailed("recording is larger than the sender will download", at)

	// The point of the mark staying: the next run tries again.
	if !n.AwaitingTranscription() {
		t.Error("a failed note stopped waiting")
	}
	if n.Transcribed() {
		t.Error("a failed note claims to be a transcription")
	}
	if n.Content != "Voice message, 0:14" {
		t.Errorf("a failure changed the content: %q", n.Content)
	}
	if got, _ := n.MetaString(MetaTranscriptionError); got == "" {
		t.Error("no reason was recorded")
	}
	if got, _ := n.MetaString(MetaTranscriptionAttemptedAt); got != "2026-10-08T06:40:11Z" {
		t.Errorf("attempted_at = %q", got)
	}
}

// Metadata arrives from JSON, where a bool may not be one.
func TestTranscriptionMarksWithWrongTypes(t *testing.T) {
	n := &Note{Metadata: map[string]any{
		MetaTranscribed:           "yes",
		MetaAwaitingTranscription: 1,
	}}
	if n.Transcribed() {
		t.Error(`"yes" was read as true`)
	}
	if n.AwaitingTranscription() {
		t.Error("1 was read as true")
	}
}
