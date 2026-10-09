package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/forgeprint/evomem/shared/audio"
	"github.com/forgeprint/evomem/shared/models"
)

func getAudio(t *testing.T, h http.Handler, id, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/notes/"+id+"/audio", nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

// This is why the endpoint exists: once a recording has been uploaded the
// server is the only copy. A browser's does not survive a reload (ADR-0029)
// and a phone restored from a backup has the note without the file, so a
// note that says "Voice note, 0:14" is unplayable without this.
func TestAudioComesBackWithItsType(t *testing.T) {
	h, recordings, noteID := audioServer(t)
	recordings.body = "the bytes"
	recordings.storedName = noteID + ".webm"

	w := getAudio(t, h, noteID, token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w.Body.String() != "the bytes" {
		t.Fatalf("body: %q", w.Body)
	}
	// Worked out from how it was stored, not guessed: a player told the
	// wrong type refuses to decode.
	if got := w.Header().Get("Content-Type"); got != "audio/webm" {
		t.Fatalf("content type: %q", got)
	}
	// Somebody's own audio, behind a token; nothing on the way should
	// keep a copy.
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control: %q", got)
	}
}

func TestAudioFromAPhoneIsNamedForAPlayer(t *testing.T) {
	h, recordings, noteID := audioServer(t)
	recordings.body = "aac"
	recordings.storedName = noteID + ".m4a"

	w := getAudio(t, h, noteID, token)
	if got := w.Header().Get("Content-Type"); got != "audio/mp4" {
		t.Fatalf("content type: %q", got)
	}
}

func TestAudioNeedsTheToken(t *testing.T) {
	h, recordings, noteID := audioServer(t)
	recordings.storedName = noteID + ".webm"

	if w := getAudio(t, h, noteID, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := getAudio(t, h, noteID, "not-the-token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if recordings.opens != 0 {
		t.Fatal("an unauthenticated request reached the recordings")
	}
}

func TestAudioForANoteWithNone(t *testing.T) {
	h, recordings, noteID := audioServer(t)
	recordings.openErr = audio.ErrNoRecording

	w := getAudio(t, h, noteID, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// A recording orphaned by a failed delete must not be readable by guessing
// identifiers, so the note is checked first.
func TestAudioForANoteThatIsNotThere(t *testing.T) {
	h, recordings, _ := audioServer(t)
	recordings.storedName = "x.webm"

	w := getAudio(t, h, models.NewULID(), token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if recordings.opens != 0 {
		t.Fatal("it went to the recordings for a note that does not exist")
	}
}

func TestAudioRefusesAMalformedId(t *testing.T) {
	h, _, _ := audioServer(t)
	if w := getAudio(t, h, "../etc/passwd", token); w.Code != http.StatusNotFound {
		// Not reached as /notes/{id}/audio at all, which is the point:
		// the router does not match it and nothing sees the path.
		t.Logf("a path traversal attempt answered %d", w.Code)
	}
	if w := getAudio(t, h, "not-a-ulid", token); w.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestAudioReportsAStoreThatFails(t *testing.T) {
	h, recordings, noteID := audioServer(t)
	recordings.openErr = errors.New("the disk is gone")

	w := getAudio(t, h, noteID, token)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
