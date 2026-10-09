package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A Note is one remembered thing. The fields here are the atomic pillars of
// the schema and are the same whatever produced the note; everything that
// belongs to one source and not the others — a Jira issue key, a Telegram
// message id, an Apple Shortcut variable — goes in Metadata rather than
// becoming a column, so adding a source never means a migration.
type Note struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	Content    string         `json:"content"`
	SourceType SourceType     `json:"source_type"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// SourceType says what produced a note. It is an open set on purpose: an
// adapter this build has never heard of still has somewhere to put its name,
// and KnownSourceTypes is a list of what ships rather than a constraint.
type SourceType string

const (
	SourceManual   SourceType = "manual"
	SourceTelegram SourceType = "telegram"
	SourceJira     SourceType = "jira"
	SourceShortcut SourceType = "shortcut"
	SourceMCP      SourceType = "mcp"
	SourceAudio    SourceType = "audio"

	// SourceMobile is the phone app. It is a source like any other — Jira
	// and Telegram are the neighbours, not the exception — so a note that
	// came off a phone says so and can be filtered by it. Before this it
	// arrived as "manual" and was indistinguishable from one typed into
	// the command line.
	SourceMobile SourceType = "mobile"
)

// KnownSourceTypes is what this build ships adapters for.
var KnownSourceTypes = []SourceType{
	SourceManual, SourceTelegram, SourceJira, SourceShortcut, SourceMCP, SourceAudio,
	SourceMobile,
}

// Validation failures a caller is expected to handle and report.
var (
	ErrEmptyContent    = errors.New("models: content is empty")
	ErrEmptyProjectID  = errors.New("models: project_id is empty")
	ErrEmptySourceType = errors.New("models: source_type is empty")
)

// maxProjectIDLen bounds a project identifier. It is an index key that arrives
// from webhooks, so it needs a limit that is not "whatever was sent".
const maxProjectIDLen = 256

// Validate checks what the database cannot. The schema rejects a null
// project_id but not one made of spaces, and an empty note is worse than no
// note at all: it occupies a search result and says nothing.
func (n *Note) Validate() error {
	if strings.TrimSpace(n.ProjectID) == "" {
		return ErrEmptyProjectID
	}
	if len(n.ProjectID) > maxProjectIDLen {
		return fmt.Errorf("models: project_id is longer than %d characters", maxProjectIDLen)
	}
	if strings.TrimSpace(n.Content) == "" {
		return ErrEmptyContent
	}
	if strings.TrimSpace(string(n.SourceType)) == "" {
		return ErrEmptySourceType
	}
	if n.ID != "" && !ValidULID(n.ID) {
		return ErrInvalidULID
	}
	return nil
}

// MarshalMetadata renders Metadata for the metadata column. An absent map is
// stored as an empty object rather than NULL, so a reader never has to decide
// which of the two it is looking at.
func (n *Note) MarshalMetadata() (string, error) {
	if len(n.Metadata) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(n.Metadata)
	if err != nil {
		return "", fmt.Errorf("models: encoding metadata: %w", err)
	}
	return string(data), nil
}

// UnmarshalMetadata reads the metadata column into Metadata.
func (n *Note) UnmarshalMetadata(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		n.Metadata = nil
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return fmt.Errorf("models: decoding metadata: %w", err)
	}
	n.Metadata = m
	return nil
}

// MetaString reads one metadata value as a string, reporting whether it was
// there and was one. Adapters reach for a single key far more often than for
// the whole map.
func (n *Note) MetaString(key string) (string, bool) {
	v, ok := n.Metadata[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetMeta sets one metadata value, creating the map when it is the first.
func (n *Note) SetMeta(key string, value any) {
	if n.Metadata == nil {
		n.Metadata = make(map[string]any, 1)
	}
	n.Metadata[key] = value
}

// Metadata keys this project assigns a meaning to. Everything else in the map
// belongs to whichever adapter wrote it.
const (
	// MetaTainted marks a note whose content came from outside the
	// project: a chat message, a Jira field, a web page. See Tainted.
	MetaTainted = "tainted"

	// MetaOrigin names where a tainted note came from, for a person
	// deciding whether to trust it.
	MetaOrigin = "origin"

	// MetaWasTainted records that a note was tainted before a person
	// endorsed it. The warning was a state; where the content came from is
	// a fact, and MetaOrigin keeps saying it. See Endorse.
	MetaWasTainted = "was_tainted"

	// MetaEndorsedAt is when a person said they had read the content and
	// stood behind it, RFC 3339.
	MetaEndorsedAt = "endorsed_at"

	// MetaAwaitingTranscription marks a note whose content describes a
	// recording rather than saying what is in it. An adapter that stores
	// audio sets it; `evomem transcribe` looks for it and clears it only
	// once a transcript has been written. See ADR-0016.
	MetaAwaitingTranscription = "awaiting_transcription"

	// MetaTranscribed marks a note whose content a machine derived from
	// audio. See Transcribed.
	MetaTranscribed = "transcribed"

	// MetaTranscribedAt is when the transcript was written, RFC 3339.
	MetaTranscribedAt = "transcribed_at"

	// MetaTranscriptionSource names the service that produced the
	// transcript, so a bad batch can be traced to what made it.
	MetaTranscriptionSource = "transcription_source"

	// MetaTranscriptionReplaced keeps the description the note was created
	// with, which the transcript overwrote.
	MetaTranscriptionReplaced = "transcription_replaced"

	// MetaTranscriptionError says why the last attempt did not produce a
	// transcript. MetaAwaitingTranscription stays set alongside it.
	MetaTranscriptionError = "transcription_error"

	// MetaTranscriptionAttemptedAt is when that attempt was made, RFC 3339.
	MetaTranscriptionAttemptedAt = "transcription_attempted_at"
)

// Tainted reports whether this note's content came from outside the project.
//
// It matters because of what reads these notes. A note goes into a model's
// context through the MCP tools, and a Telegram message or a Jira description
// is text a third party wrote: it can contain instructions aimed at whatever
// reads it next. Nothing here can stop that text being stored — storing it is
// the point — but a reader that is told where it came from can treat it as
// data rather than as something the user said.
//
// Anything ingested by an adapter is tainted. A note the user typed into
// evomem add is not.
func (n *Note) Tainted() bool {
	v, ok := n.Metadata[MetaTainted]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// MarkTainted records that this note's content came from outside, and from
// where. Every adapter calls it.
func (n *Note) MarkTainted(origin string) {
	n.SetMeta(MetaTainted, true)
	if origin != "" {
		n.SetMeta(MetaOrigin, origin)
	}
}

// Endorse records that a person has read this content and stands behind it,
// which clears the tainted mark.
//
// Only the tainted mark. It says nobody has read this and a third party wrote
// it, and a person reading it and saying yes is the thing it exists to be
// absent for — the same reasoning AcceptProposal already applies to a
// proposal a person accepts. Transcribed is a different claim and stays:
// after the reading the content is still something a model guessed at from
// audio, and a reader two months later deserves to know.
//
// What was claimed is kept rather than erased: MetaWasTainted, and MetaOrigin
// goes on saying where the content came from. See ADR-0021.
//
// Reports whether anything changed, so a caller can tell a person who asked
// for a state the note was already in.
func (n *Note) Endorse(at time.Time) bool {
	if !n.Tainted() {
		return false
	}
	delete(n.Metadata, MetaTainted)
	n.SetMeta(MetaWasTainted, true)
	n.SetMeta(MetaEndorsedAt, at.UTC().Format(time.RFC3339))
	return true
}

// Endorsed reports whether a person has said they read this content and stood
// behind it.
func (n *Note) Endorsed() bool {
	_, ok := n.MetaString(MetaEndorsedAt)
	return ok
}

// AwaitingTranscription reports whether this note stands for a recording
// nobody has transcribed yet. Its content describes the recording — how long
// it was, what the caption said — rather than what was in it.
func (n *Note) AwaitingTranscription() bool {
	b, _ := n.Metadata[MetaAwaitingTranscription].(bool)
	return b
}

// Transcribed reports whether a machine derived this note's content from
// audio.
//
// It is a different claim from Tainted, and both can be true. Tainted says a
// third party wrote the text. This says no one wrote it: a model heard the
// audio and guessed at the words, so a name may be the wrong name and a
// negation may have been dropped. A reader told this can weigh the text
// accordingly; one that is not reads a mishearing as something the user said.
func (n *Note) Transcribed() bool {
	b, _ := n.Metadata[MetaTranscribed].(bool)
	return b
}

// ApplyTranscript replaces the content with what came back from [source],
// keeping the description it replaced, and clears the awaiting mark so the
// next run passes this note by.
//
// The tainted mark, if the adapter set one, is left alone: where the audio
// came from did not change.
func (n *Note) ApplyTranscript(text, source string, at time.Time) {
	n.SetMeta(MetaTranscriptionReplaced, n.Content)
	n.Content = text
	n.SetMeta(MetaTranscribed, true)
	n.SetMeta(MetaTranscribedAt, at.UTC().Format(time.RFC3339))
	if source != "" {
		n.SetMeta(MetaTranscriptionSource, source)
	}
	delete(n.Metadata, MetaAwaitingTranscription)
	delete(n.Metadata, MetaTranscriptionError)
	delete(n.Metadata, MetaTranscriptionAttemptedAt)
}

// MarkTranscriptionFailed records why there is still no transcript.
//
// The awaiting mark stays, so the next run tries again. A recording larger
// than the sender will hand over never succeeds, and the reason sits on the
// note where someone can read it rather than only in a log that has rotated.
func (n *Note) MarkTranscriptionFailed(reason string, at time.Time) {
	n.SetMeta(MetaTranscriptionError, reason)
	n.SetMeta(MetaTranscriptionAttemptedAt, at.UTC().Format(time.RFC3339))
	n.SetMeta(MetaAwaitingTranscription, true)
}
