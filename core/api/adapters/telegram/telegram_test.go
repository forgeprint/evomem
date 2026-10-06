package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

// fakeStore records what was written, and can be made to fail.
type fakeStore struct {
	notes []*models.Note
	err   error
}

func (f *fakeStore) Create(_ context.Context, n *models.Note) error {
	if f.err != nil {
		return f.err
	}
	n.ID = models.NewULID()
	f.notes = append(f.notes, n)
	return nil
}

const secret = "a-secret-token"

func newHandler(t *testing.T) (*Handler, *fakeStore) {
	t.Helper()
	store := &fakeStore{}
	h, err := New(store, secret, "evomem")
	if err != nil {
		t.Fatal(err)
	}
	return h, store
}

// post sends a body with the right secret unless withSecret says otherwise.
func post(t *testing.T, h *Handler, body string, header string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	if header != "" {
		r.Header.Set(SecretHeader, header)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// A webhook endpoint is reachable by anyone who learns the URL, so refusing
// to start without a secret is the difference between an ingest endpoint and
// an open one.
func TestNewRefusesWithoutSecret(t *testing.T) {
	if _, err := New(&fakeStore{}, "", "evomem"); err == nil {
		t.Error("a handler was built with no secret")
	}
	if _, err := New(&fakeStore{}, "   ", "evomem"); err == nil {
		t.Error("a handler was built with a blank secret")
	}
	if _, err := New(&fakeStore{}, secret, ""); err == nil {
		t.Error("a handler was built with no project")
	}
	if _, err := New(nil, secret, "evomem"); err == nil {
		t.Error("a handler was built with no store")
	}
}

func TestRejectsWrongSecret(t *testing.T) {
	h, store := newHandler(t)

	for _, given := range []string{"", "wrong", secret + "x", strings.ToUpper(secret)} {
		w := post(t, h, `{"update_id":1,"message":{"message_id":1,"text":"hi","chat":{"id":5}}}`, given)
		if w.Code != http.StatusForbidden {
			t.Errorf("secret %q got %d, want 403", given, w.Code)
		}
	}
	if len(store.notes) != 0 {
		t.Errorf("%d notes were written despite a wrong secret", len(store.notes))
	}
}

func TestRejectsNonPost(t *testing.T) {
	h, _ := newHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/telegram/webhook", nil)
	r.Header.Set(SecretHeader, secret)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", w.Code)
	}
}

func TestTextMessage(t *testing.T) {
	h, store := newHandler(t)

	body := `{"update_id":42,"message":{"message_id":7,"date":1760000000,
		"from":{"id":99,"is_bot":false,"first_name":"Ali","username":"ali"},
		"chat":{"id":-100123,"type":"group","title":"notes"},
		"text":"  the gateway timed out  "}}`
	w := post(t, h, body, secret)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if len(store.notes) != 1 {
		t.Fatalf("%d notes were written, want 1", len(store.notes))
	}

	n := store.notes[0]
	if n.Content != "the gateway timed out" {
		t.Errorf("content is %q; surrounding whitespace was kept", n.Content)
	}
	if n.ProjectID != "evomem" {
		t.Errorf("project is %q, want evomem", n.ProjectID)
	}
	if n.SourceType != models.SourceTelegram {
		t.Errorf("source is %q, want telegram", n.SourceType)
	}

	// A chat message is written by someone else and will later be read by
	// a model. This flag is how the reader knows it is data.
	if !n.Tainted() {
		t.Error("a Telegram message was not marked tainted")
	}
	if origin, _ := n.MetaString(models.MetaOrigin); origin != Origin {
		t.Errorf("origin is %q, want %s", origin, Origin)
	}

	// The chat is recorded so a later version can route on it without a
	// migration.
	if n.Metadata["telegram_chat_id"] != float64(-100123) && n.Metadata["telegram_chat_id"] != int64(-100123) {
		t.Errorf("chat id is %v", n.Metadata["telegram_chat_id"])
	}
	if got, _ := n.MetaString("telegram_from_username"); got != "ali" {
		t.Errorf("username is %q", got)
	}
}

// A voice message has no text, and this phase does not transcribe. What it
// can keep is that one arrived and the handle to fetch it later.
func TestVoiceMessage(t *testing.T) {
	h, store := newHandler(t)

	body := `{"update_id":43,"message":{"message_id":8,"chat":{"id":5,"type":"private"},
		"voice":{"file_id":"AwACAgQ","file_unique_id":"AgADxQ","duration":12,"mime_type":"audio/ogg","file_size":4096}}}`
	if w := post(t, h, body, secret); w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if len(store.notes) != 1 {
		t.Fatalf("%d notes were written, want 1", len(store.notes))
	}

	n := store.notes[0]
	// The content is what the full-text index holds, so it has to say in
	// words that a recording arrived.
	if !strings.Contains(n.Content, "Voice message") || !strings.Contains(n.Content, "12s") {
		t.Errorf("content is %q", n.Content)
	}
	if got, _ := n.MetaString("telegram_file_id"); got != "AwACAgQ" {
		t.Errorf("file_id is %q; without it the recording cannot be fetched later", got)
	}
	if n.Metadata["awaiting_transcription"] != true {
		t.Error("the note is not marked as awaiting transcription")
	}
}

func TestVoiceMessageWithCaption(t *testing.T) {
	h, store := newHandler(t)

	body := `{"update_id":44,"message":{"message_id":9,"chat":{"id":5},
		"voice":{"file_id":"x","duration":3},"caption":"about the migration"}}`
	post(t, h, body, secret)

	if !strings.Contains(store.notes[0].Content, "about the migration") {
		t.Errorf("the caption was dropped: %q", store.notes[0].Content)
	}
}

func TestAudioMessage(t *testing.T) {
	h, store := newHandler(t)

	body := `{"update_id":45,"message":{"message_id":10,"chat":{"id":5},
		"audio":{"file_id":"y","duration":90,"title":"standup"}}}`
	post(t, h, body, secret)

	n := store.notes[0]
	if !strings.Contains(n.Content, "standup") {
		t.Errorf("content is %q", n.Content)
	}
	if got, _ := n.MetaString("telegram_file_id"); got != "y" {
		t.Errorf("file_id is %q", got)
	}
}

func TestCaptionOnlyMessage(t *testing.T) {
	h, store := newHandler(t)

	// A photo with something written under it: the caption is the part
	// worth remembering.
	body := `{"update_id":46,"message":{"message_id":11,"chat":{"id":5},"caption":"the whiteboard"}}`
	post(t, h, body, secret)

	if len(store.notes) != 1 || store.notes[0].Content != "the whiteboard" {
		t.Errorf("notes are %v", store.notes)
	}
}

func TestEditedMessage(t *testing.T) {
	h, store := newHandler(t)

	body := `{"update_id":47,"edited_message":{"message_id":12,"chat":{"id":5},"text":"corrected"}}`
	post(t, h, body, secret)

	if len(store.notes) != 1 {
		t.Fatalf("%d notes were written, want 1", len(store.notes))
	}
	if store.notes[0].Metadata["telegram_edited"] != true {
		t.Error("an edit was not recorded as one")
	}
}

// Telegram retries an update it did not get a 2xx for, and retries everything
// behind it. So nothing worth storing has to be a 200, not a 4xx.
func TestNothingToStoreIsStillOK(t *testing.T) {
	h, store := newHandler(t)

	cases := map[string]string{
		"a sticker":         `{"update_id":1,"message":{"message_id":1,"chat":{"id":5}}}`,
		"a bot's own words": `{"update_id":2,"message":{"message_id":2,"chat":{"id":5},"from":{"id":1,"is_bot":true},"text":"beep"}}`,
		"an unread type":    `{"update_id":3,"poll_answer":{"poll_id":"1"}}`,
		"an empty update":   `{"update_id":4}`,
		"whitespace only":   `{"update_id":5,"message":{"message_id":5,"chat":{"id":5},"text":"   "}}`,
	}
	for name, body := range cases {
		w := post(t, h, body, secret)
		if w.Code != http.StatusOK {
			t.Errorf("%s got %d, want 200 so Telegram does not retry it", name, w.Code)
		}
	}
	if len(store.notes) != 0 {
		t.Errorf("%d notes were written, want none", len(store.notes))
	}
}

func TestMalformedBody(t *testing.T) {
	h, _ := newHandler(t)

	if w := post(t, h, `{not json`, secret); w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}

// A store that failed may succeed on the retry, so this is the one case that
// should ask for one.
func TestStoreFailureAsksForARetry(t *testing.T) {
	store := &fakeStore{err: context.DeadlineExceeded}
	h, err := New(store, secret, "evomem")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"update_id":1,"message":{"message_id":1,"chat":{"id":5},"text":"hi"}}`
	if w := post(t, h, body, secret); w.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", w.Code)
	}
}

// The header name is not a guess: it is what Telegram sends for setWebhook's
// secret_token, verified against the API documentation.
func TestSecretHeaderName(t *testing.T) {
	if SecretHeader != "X-Telegram-Bot-Api-Secret-Token" {
		t.Errorf("SecretHeader is %q", SecretHeader)
	}
}
