package transcribe

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// request is what a fake service saw, so a test can assert on the wire format
// rather than on this package's own idea of it.
type request struct {
	path     string
	auth     string
	fields   map[string]string
	filename string
	audio    string
}

func fakeService(t *testing.T, status int, body string) (*httptest.Server, *request) {
	t.Helper()
	seen := &request{fields: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")

		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Errorf("content type: %v", err)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("reading part: %v", err)
				return
			}
			value, err := io.ReadAll(part)
			if err != nil {
				t.Errorf("reading part: %v", err)
				return
			}
			if part.FormName() == "file" {
				seen.filename = part.FileName()
				seen.audio = string(value)
				continue
			}
			seen.fields[part.FormName()] = string(value)
		}

		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func TestNotConfiguredIsNotAnError(t *testing.T) {
	for _, url := range []string{"", "   "} {
		_, err := NewService(Config{BaseURL: url})
		if !errors.Is(err, ErrNotConfigured) {
			t.Errorf("NewService(%q) = %v, want ErrNotConfigured", url, err)
		}
	}
}

func TestServiceRefusesANonHTTPURL(t *testing.T) {
	// A path or an ftp:// would otherwise be posted to by net/http in
	// surprising ways, or silently never reach anything.
	for _, url := range []string{"ftp://host/x", "/var/run/whisper.sock", "localhost:8001"} {
		if _, err := NewService(Config{BaseURL: url}); err == nil {
			t.Errorf("NewService(%q) was accepted", url)
		}
	}
}

// The wire format is the contract with every OpenAI-compatible server, so it
// is asserted field by field.
func TestTranscribePostsTheDocumentedForm(t *testing.T) {
	srv, seen := fakeService(t, http.StatusOK, `{"text":"tünel önce ayakta olmalı"}`)

	svc, err := NewService(Config{
		BaseURL:  srv.URL,
		Token:    "sk-test",
		Language: "tr",
	})
	if err != nil {
		t.Fatal(err)
	}

	text, err := svc.Transcribe(context.Background(), "voice.oga", strings.NewReader("OggS-pretend"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "tünel önce ayakta olmalı" {
		t.Errorf("text = %q", text)
	}

	if seen.path != "/v1/audio/transcriptions" {
		t.Errorf("path = %q", seen.path)
	}
	if seen.auth != "Bearer sk-test" {
		t.Errorf("authorization = %q", seen.auth)
	}
	if seen.filename != "voice.oga" {
		t.Errorf("filename = %q; the service decides how to decode by extension", seen.filename)
	}
	if seen.audio != "OggS-pretend" {
		t.Errorf("audio = %q", seen.audio)
	}
	// model is required by the interface, so there is always one.
	if seen.fields["model"] != DefaultModel {
		t.Errorf("model = %q, want %q", seen.fields["model"], DefaultModel)
	}
	if seen.fields["language"] != "tr" {
		t.Errorf("language = %q", seen.fields["language"])
	}
}

func TestTranscribeOmitsWhatWasNotConfigured(t *testing.T) {
	srv, seen := fakeService(t, http.StatusOK, `{"text":"ok"}`)
	svc, err := NewService(Config{BaseURL: srv.URL, Model: "whisper-large-v3"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transcribe(context.Background(), "a.wav", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}

	// A self-hosted service usually wants no token, and sending an empty
	// bearer header is worse than sending none.
	if seen.auth != "" {
		t.Errorf("authorization = %q, want none", seen.auth)
	}
	if _, ok := seen.fields["language"]; ok {
		t.Error("language was sent although none was configured")
	}
	if seen.fields["model"] != "whisper-large-v3" {
		t.Errorf("model = %q", seen.fields["model"])
	}
}

func TestTranscribeReportsWhatTheServiceSaid(t *testing.T) {
	srv, _ := fakeService(t, http.StatusBadRequest, `{"error":{"message":"audio too short"}}`)
	svc, err := NewService(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Transcribe(context.Background(), "a.wav", strings.NewReader("x"))
	if err == nil {
		t.Fatal("a 400 was accepted")
	}
	// The body, not just the status: "400 Bad Request" alone does not say
	// what to change.
	if !strings.Contains(err.Error(), "audio too short") {
		t.Errorf("error = %v; it should quote what the service said", err)
	}
}

func TestTranscribeRejectsAnEmptyTranscript(t *testing.T) {
	// Silence comes back as a 200 with no text. A note whose content
	// became "" would fail validation, so this is a failure here.
	for _, body := range []string{`{"text":""}`, `{"text":"   "}`, `{}`} {
		srv, _ := fakeService(t, http.StatusOK, body)
		svc, err := NewService(Config{BaseURL: srv.URL})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Transcribe(context.Background(), "a.wav", strings.NewReader("x")); err == nil {
			t.Errorf("%s was accepted as a transcript", body)
		}
	}
}

func TestTranscribeRejectsSomethingThatIsNotTheService(t *testing.T) {
	srv, _ := fakeService(t, http.StatusOK, "<html>login</html>")
	svc, err := NewService(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transcribe(context.Background(), "a.wav", strings.NewReader("x")); err == nil {
		t.Error("an HTML page was accepted as a transcript")
	}
}

func TestEndpointIsWhatGetsRecorded(t *testing.T) {
	svc, err := NewService(Config{BaseURL: "https://whisper.example/"})
	if err != nil {
		t.Fatal(err)
	}
	// The trailing slash is dropped so the value written to a note's
	// metadata is the same whichever way it was configured.
	if got := svc.Endpoint(); got != "https://whisper.example/v1/audio/transcriptions" {
		t.Errorf("Endpoint() = %q", got)
	}
}
