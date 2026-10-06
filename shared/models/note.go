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
)

// KnownSourceTypes is what this build ships adapters for.
var KnownSourceTypes = []SourceType{
	SourceManual, SourceTelegram, SourceJira, SourceShortcut, SourceMCP, SourceAudio,
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
