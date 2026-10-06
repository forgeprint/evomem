package models

import (
	"errors"
	"strings"
	"testing"
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
