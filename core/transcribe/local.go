package transcribe

import (
	"context"
	"fmt"
	"io"
)

// Recordings is the part of shared/audio this package needs.
type Recordings interface {
	// Open returns a note's recording and the name it is stored under.
	Open(noteID string) (io.ReadCloser, string, error)
}

// LocalFiles reads recordings evomem already holds: the ones a phone uploaded
// to POST /ingest/audio (ADR-0018).
//
// Unlike TelegramFiles this needs no network and no credential — the file is
// on the same disk as the database. It is a Fetcher so that an uploaded
// recording and a Telegram voice message go through one transcription path.
type LocalFiles struct {
	store Recordings
}

// NewLocalFiles reads from a store of recordings.
func NewLocalFiles(store Recordings) (*LocalFiles, error) {
	if store == nil {
		return nil, ErrNotConfigured
	}
	return &LocalFiles{store: store}, nil
}

// Fetch reads the recording stored for a note.
//
// The ref is the note's own identifier: a recording uploaded against a note
// is named by it, so there is no separate reference to keep.
func (l *LocalFiles) Fetch(_ context.Context, ref string) ([]byte, string, error) {
	f, name, err := l.store.Open(ref)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	// Bounded like every other read of a recording: the whole thing is
	// held in memory before it is posted.
	recording, err := io.ReadAll(io.LimitReader(f, TelegramMaxDownloadBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("transcribe: reading %s: %w", name, err)
	}
	if len(recording) > TelegramMaxDownloadBytes {
		return nil, "", fmt.Errorf("%w: over %d bytes", ErrTooLarge, TelegramMaxDownloadBytes)
	}
	if len(recording) == 0 {
		return nil, "", fmt.Errorf("transcribe: %s is empty", name)
	}
	return recording, name, nil
}
