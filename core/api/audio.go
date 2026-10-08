package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/forgeprint/evomem/shared/audio"
	"github.com/forgeprint/evomem/shared/models"
)

// Recordings is where this package puts an uploaded recording.
// shared/audio.Store implements it.
type Recordings interface {
	// Save stores one recording for a note, returning where it went.
	Save(noteID, mediaType string, r io.Reader) (string, error)
}

// handleAudio attaches a recording to a note evomem already stored.
//
// Two requests rather than one, because the identifier comes from the first:
// /ingest mints the ULID and returns it, and this is posted against that
// identifier. See ADR-0018 for why it is not one multipart request.
//
// The body is the recording itself, not a multipart form. The sender is the
// mobile app and a Shortcut, both of which can post a file as a body with a
// content type; wrapping one file in a form would buy nothing and cost both
// of them a construction step.
func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	// Checked before anything is read, so a wrong identifier costs no
	// upload. A name on disk is derived from this, which is why it goes
	// through NormalizeULID rather than being trusted.
	noteID, err := models.NormalizeULID(r.URL.Query().Get("note"))
	if err != nil {
		http.Error(w, "the note parameter has to be a note id, as /ingest returned it",
			http.StatusBadRequest)
		return
	}

	// The note has to exist. Without this, a recording could be stored
	// under an identifier nothing points at, and nothing would ever
	// delete it.
	if _, err := s.store.Get(r.Context(), noteID); err != nil {
		http.Error(w, "there is no such note", http.StatusBadRequest)
		return
	}

	mediaType := r.Header.Get("Content-Type")
	if _, ok := audio.ExtensionFor(mediaType); !ok {
		// Named, because the sender can fix it: a recording stored
		// under a guessed extension fails later, on the transcription
		// service, where the message is someone else's.
		http.Error(w, fmt.Sprintf("%q is not a recording format this accepts", mediaType),
			http.StatusUnsupportedMediaType)
		return
	}

	// 20 MiB here rather than this package's usual 1 MiB, which is the
	// deviation ADR-0018 records. MaxBytesReader so the connection is cut
	// rather than read to the end; the store bounds it a second time,
	// because it is also called by things that are not this handler.
	r.Body = http.MaxBytesReader(w, r.Body, audio.MaxBytes)

	if _, err := s.recordings.Save(noteID, mediaType, r.Body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, fmt.Sprintf("a recording has to be under %d bytes", audio.MaxBytes),
				http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "could not store the recording", http.StatusInternalServerError)
		return
	}

	// Nothing to say: the note already has its identifier and the sender
	// chose it.
	w.WriteHeader(http.StatusNoContent)
}
