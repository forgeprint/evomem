// Package audio keeps the recordings a note stands for.
//
// One file per note, named by the note's identifier, in a directory beside
// the database. Three callers share it: the HTTP endpoint writes, the
// transcription run reads, and deleting a note removes. See ADR-0018.
//
// It is a directory of files rather than rows in SQLite on purpose. A note is
// kilobytes and a recording is megabytes; putting the second in the same file
// as the first would make every backup, every vacuum and every sync carry
// audio nobody asked for.
package audio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/forgeprint/evomem/shared/models"
)

// MaxBytes bounds one recording, and is why the HTTP endpoint and the
// Telegram download agree on a figure: `core/transcribe` reads a whole
// recording into memory before posting it, so the bound is evomem's own and
// not only the sender's.
const MaxBytes = 20 << 20

// ErrNoRecording is returned for a note that has no audio on disk. It is an
// ordinary outcome: a note may be marked as awaiting a transcript before its
// upload arrives, or the upload may never have arrived at all.
var ErrNoRecording = errors.New("audio: no recording for this note")

// extensions maps what a sender says it is posting to what the file is
// called.
//
// The extension matters: a transcription service decides how to decode by it,
// so a recording saved under the wrong name is rejected by some servers and
// misread by others. Only formats the OpenAI speech-to-text interface
// documents as supported are here — mp3, mp4, mpeg, mpga, m4a, wav and webm —
// plus ogg, which is what Telegram sends and what every whisper server reads.
var extensions = map[string]string{
	"audio/mpeg":  ".mp3",
	"audio/mp3":   ".mp3",
	"audio/mp4":   ".m4a",
	"audio/m4a":   ".m4a",
	"audio/x-m4a": ".m4a",
	"audio/aac":   ".m4a",
	"audio/wav":   ".wav",
	"audio/x-wav": ".wav",
	"audio/wave":  ".wav",
	"audio/webm":  ".webm",
	"video/webm":  ".webm",
	"audio/ogg":   ".ogg",
	"video/ogg":   ".ogg",
	"audio/opus":  ".ogg",
	"audio/oga":   ".ogg",
}

// ExtensionFor says what a recording of this media type is called, reporting
// whether the type is one this accepts.
//
// Anything else is refused rather than stored under a guessed name. A file
// the transcription service cannot decode is worse than an upload that was
// turned down, because the first fails later, on someone else's machine.
func ExtensionFor(mediaType string) (string, bool) {
	mediaType, _, _ = strings.Cut(mediaType, ";")
	ext, ok := extensions[strings.ToLower(strings.TrimSpace(mediaType))]
	return ext, ok
}

// A Store is a directory of recordings.
type Store struct {
	dir string
}

// StoreBeside opens the store that belongs to a database file: the `audio`
// directory next to it. The directory is created when the first recording is
// saved, so a store nobody uses leaves nothing behind.
func StoreBeside(databasePath string) *Store {
	return &Store{dir: filepath.Join(filepath.Dir(databasePath), "audio")}
}

// Dir is where recordings are kept, for a command that wants to say so.
func (s *Store) Dir() string { return s.dir }

// path is the only place a note identifier becomes a filename.
//
// It goes through NormalizeULID first, so the name is 26 characters of
// Crockford base32 and nothing else. That is what keeps a sender from
// choosing a path: there is no spelling of "../" that survives it.
func (s *Store) path(noteID, ext string) (string, error) {
	id, err := models.NormalizeULID(noteID)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, id+ext), nil
}

// Save writes a recording for a note, replacing one already there.
//
// It refuses anything past MaxBytes, having read one byte more than the
// bound: a sender that understated its length is caught here rather than
// filling the disk.
func (s *Store) Save(noteID, mediaType string, r io.Reader) (string, error) {
	ext, ok := ExtensionFor(mediaType)
	if !ok {
		return "", fmt.Errorf("audio: %q is not a recording format this accepts", mediaType)
	}
	name, err := s.path(noteID, ext)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("audio: making %s: %w", s.dir, err)
	}

	// Written beside and renamed, so a failed upload never leaves a
	// half-written recording that the transcription run would post as if
	// it were whole.
	tmp, err := os.CreateTemp(s.dir, ".partial-*")
	if err != nil {
		return "", fmt.Errorf("audio: %w", err)
	}
	defer os.Remove(tmp.Name())

	written, err := io.Copy(tmp, io.LimitReader(r, MaxBytes+1))
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("audio: writing the recording: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("audio: writing the recording: %w", err)
	}
	if written > MaxBytes {
		return "", fmt.Errorf("audio: recording is over %d bytes", MaxBytes)
	}
	if written == 0 {
		return "", errors.New("audio: the recording is empty")
	}

	// Any other extension for the same note goes, so a re-upload in a
	// different format does not leave two recordings of one note with
	// only one of them reachable.
	if err := s.Remove(noteID); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return "", fmt.Errorf("audio: %w", err)
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return "", fmt.Errorf("audio: storing the recording: %w", err)
	}
	return name, nil
}

// Open returns a note's recording and the name it is stored under, which is
// what tells a transcription service how to decode it.
//
// A note with no recording is ErrNoRecording, which a caller is expected to
// report rather than treat as a failure of the store.
func (s *Store) Open(noteID string) (io.ReadCloser, string, error) {
	name, err := s.find(noteID)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, "", fmt.Errorf("audio: %w", err)
	}
	return f, filepath.Base(name), nil
}

// Remove deletes a note's recording, in whichever format it was stored.
//
// A note with no recording is not an error: this is called on every delete,
// and most notes are text.
func (s *Store) Remove(noteID string) error {
	id, err := models.NormalizeULID(noteID)
	if err != nil {
		return err
	}
	for _, ext := range knownExtensions() {
		name := filepath.Join(s.dir, id+ext)
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("audio: removing %s: %w", name, err)
		}
	}
	return nil
}

// RemoveAll deletes every recording. It is what a restore needs: the notes it
// is about to replace are gone, and so are their recordings.
func (s *Store) RemoveAll() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("audio: removing %s: %w", s.dir, err)
	}
	return nil
}

// find locates a note's recording whatever format it was stored in.
func (s *Store) find(noteID string) (string, error) {
	id, err := models.NormalizeULID(noteID)
	if err != nil {
		return "", err
	}
	for _, ext := range knownExtensions() {
		name := filepath.Join(s.dir, id+ext)
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrNoRecording, noteID)
}

// knownExtensions is every distinct extension a recording can be stored
// under, which is what Remove and find have to look through. Sorted, so two
// runs behave identically.
func knownExtensions() []string {
	seen := map[string]bool{}
	var out []string
	for _, ext := range extensions {
		if !seen[ext] {
			seen[ext] = true
			out = append(out, ext)
		}
	}
	// Few enough that an insertion sort is the whole of it, and it keeps
	// the package on the standard library alone.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
