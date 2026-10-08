package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/audio"
	"github.com/forgeprint/evomem/shared/database"
)

// fakeRecordings records what the handler asked it to store, so the tests can
// assert on the handler rather than on the filesystem.
type fakeRecordings struct {
	noteID    string
	mediaType string
	body      string
	err       error
	calls     int
}

func (f *fakeRecordings) Save(noteID, mediaType string, r io.Reader) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	f.noteID, f.mediaType, f.body = noteID, mediaType, string(body)
	return "/tmp/" + noteID, nil
}

// audioServer is a server with the audio endpoint served, plus a note to
// attach a recording to.
func audioServer(t *testing.T) (http.Handler, *fakeRecordings, string) {
	t.Helper()
	recordings := &fakeRecordings{}
	cfg := fullConfig()
	cfg.Recordings = recordings
	h, _ := newServer(t, cfg)

	// The identifier comes from /ingest, which is the whole reason the
	// upload is a second request (ADR-0018).
	w := ingest(t, h, `{"project":"evomem","content":"Voice message, 0:14"}`,
		"application/json", token, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("storing the note: %d %s", w.Code, w.Body)
	}
	var reply struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	return h, recordings, reply.ID
}

func postAudio(t *testing.T, h http.Handler, noteID, contentType, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/ingest/audio"
	if noteID != "" {
		url += "?note=" + noteID
	}
	r := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return do(h, r)
}

func TestAudioUploadStoresTheRecording(t *testing.T) {
	h, recordings, id := audioServer(t)

	w := postAudio(t, h, id, "audio/ogg", "OggS-pretend", token)
	if w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if recordings.noteID != id {
		t.Errorf("stored against %q, want %q", recordings.noteID, id)
	}
	if recordings.mediaType != "audio/ogg" {
		t.Errorf("media type %q", recordings.mediaType)
	}
	if recordings.body != "OggS-pretend" {
		t.Errorf("body %q", recordings.body)
	}
}

// Without a token there is no /ingest/audio at all, the way every other
// endpoint here behaves (ADR-0010).
func TestAudioIsNotServedWithoutATokenOrAStore(t *testing.T) {
	// No token: no endpoint, even with somewhere to put a recording.
	noToken := Config{TelegramSecret: telegramSecret, TelegramProject: "evomem"}
	noToken.Recordings = &fakeRecordings{}
	h, _ := newServer(t, noToken)
	if w := postAudio(t, h, "01M4D3H3HNMFM69N4MHNAYBZ1X", "audio/ogg", "x", token); w.Code != http.StatusNotFound {
		t.Errorf("without a token: %d, want 404", w.Code)
	}

	// A token but nowhere to put a recording: also not served, because
	// accepting an upload with nothing to write it to would lose it.
	h2, _ := newServer(t, fullConfig())
	if w := postAudio(t, h2, "01M4D3H3HNMFM69N4MHNAYBZ1X", "audio/ogg", "x", token); w.Code != http.StatusNotFound {
		t.Errorf("without a store: %d, want 404", w.Code)
	}
}

func TestAudioRejectsBadCredentials(t *testing.T) {
	h, recordings, id := audioServer(t)

	for _, bearer := range []string{"", "wrong-token", token + "x"} {
		w := postAudio(t, h, id, "audio/ogg", "x", bearer)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("bearer %q: %d, want 401", bearer, w.Code)
		}
	}
	if recordings.calls != 0 {
		t.Errorf("the store was reached %d times by an unauthorized request", recordings.calls)
	}
}

// The note parameter becomes a filename, so it is checked before anything is
// read and nothing is stored for a note that does not exist.
func TestAudioRefusesAnIdentifierItCannotTrust(t *testing.T) {
	h, recordings, _ := audioServer(t)

	for _, note := range []string{
		"",
		"not-a-ulid",
		"../../etc/passwd",
		"01M4D3H3HNMFM69N4MHNAYBZ1Z", // a well-formed id for no note
	} {
		w := postAudio(t, h, note, "audio/ogg", "x", token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("note %q: %d, want 400", note, w.Code)
		}
	}
	if recordings.calls != 0 {
		t.Errorf("the store was reached %d times for a note it should not have been", recordings.calls)
	}
}

func TestAudioRefusesAFormatNothingCanDecode(t *testing.T) {
	h, recordings, id := audioServer(t)

	for _, contentType := range []string{"", "text/plain", "application/json", "audio/flac"} {
		w := postAudio(t, h, id, contentType, "x", token)
		if w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("content type %q: %d, want 415", contentType, w.Code)
		}
		// Named, so the sender can fix it.
		if !strings.Contains(w.Body.String(), "not a recording format") {
			t.Errorf("content type %q: body %q", contentType, w.Body)
		}
	}
	if recordings.calls != 0 {
		t.Errorf("the store was reached %d times with a format it cannot decode", recordings.calls)
	}
}

// 20 MiB here, against 1 MiB everywhere else in this package: the deviation
// ADR-0018 records.
func TestAudioTakesMoreThanTheOtherEndpointsAndStillHasABound(t *testing.T) {
	h, recordings, id := audioServer(t)

	// Comfortably past the 1 MiB the rest of this package allows.
	w := postAudio(t, h, id, "audio/ogg", strings.Repeat("a", 2<<20), token)
	if w.Code != http.StatusNoContent {
		t.Fatalf("2 MiB: %d %s", w.Code, w.Body)
	}
	if len(recordings.body) != 2<<20 {
		t.Errorf("stored %d bytes", len(recordings.body))
	}

	// And past its own bound it is refused rather than read.
	w = postAudio(t, h, id, "audio/ogg", strings.Repeat("a", audio.MaxBytes+1), token)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("over the bound: %d, want 413", w.Code)
	}
}

func TestAudioReportsAStoreThatFailed(t *testing.T) {
	recordings := &fakeRecordings{err: errors.New("no space left on device")}
	cfg := fullConfig()
	cfg.Recordings = recordings
	h, db := newServer(t, cfg)
	_ = db

	w := ingest(t, h, `{"project":"evomem","content":"Voice message"}`, "application/json", token, "")
	var reply struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if got := postAudio(t, h, reply.ID, "audio/ogg", "x", token); got.Code != http.StatusInternalServerError {
		t.Errorf("%d, want 500", got.Code)
	}
}

func TestRoutesNameTheAudioEndpoint(t *testing.T) {
	cfg := fullConfig()
	cfg.Recordings = &fakeRecordings{}
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := New(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range s.Routes() {
		if r == "POST /ingest/audio" {
			found = true
		}
	}
	if !found {
		t.Errorf("routes = %v", s.Routes())
	}
}
