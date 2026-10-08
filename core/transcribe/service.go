// Package transcribe turns the recordings evomem has stored into text.
//
// Nothing here runs on its own. `evomem transcribe` drives it: it asks the
// store what is waiting, fetches each recording from wherever the adapter
// said it lives, posts it to a transcription service, and writes the result
// back. With no service configured the whole feature is off and the notes
// stay as they are, which is the point — see ADR-0016.
//
// Evomem is the client here, never the server. It does not serve an endpoint
// that accepts audio.
package transcribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults for a service that was not told otherwise.
const (
	// DefaultModel is what the request names when nothing else is
	// configured. The field is required by the interface, so there is no
	// "unset": whisper-1 is the model every OpenAI-compatible server
	// implements, including the self-hosted ones this is pointed at.
	DefaultModel = "whisper-1"

	// DefaultTimeout bounds one request. Transcription is slower than a
	// web request and faster than a build; a minute of audio on a CPU is
	// tens of seconds.
	DefaultTimeout = 5 * time.Minute
)

// ErrNotConfigured is what a caller gets when no service URL was given. It is
// not a failure: it is the feature being switched off, and `evomem
// transcribe` reports it as such and exits without touching a note.
var ErrNotConfigured = errors.New("transcribe: no service configured")

// Config is how a service is reached.
type Config struct {
	// BaseURL of an OpenAI-compatible server, without the path: the
	// request goes to BaseURL + "/v1/audio/transcriptions". Empty means
	// transcription is off.
	BaseURL string

	// Token for the Authorization header, when the service wants one. A
	// self-hosted server usually does not.
	Token string

	// Model names the model in the request. Required by the interface,
	// defaulted to DefaultModel.
	Model string

	// Language is an optional ISO-639-1 hint. Worth setting: a model left
	// to guess will read short Turkish audio as some other language and
	// return a confident translation of something nobody said.
	Language string

	// Timeout bounds one request. Zero means DefaultTimeout.
	Timeout time.Duration
}

// A Service posts audio to an OpenAI-compatible transcription endpoint.
//
// The wire format is from the OpenAI speech-to-text guide, read 2026-10-08:
// POST /v1/audio/transcriptions, multipart/form-data with a `file` part and a
// `model` field, `Authorization: Bearer <token>`, and a JSON reply whose
// `text` holds the transcript. Those four are all this depends on, which is
// what makes a self-hosted server a drop-in for the hosted one.
//
//	https://developers.openai.com/api/docs/guides/speech-to-text
type Service struct {
	cfg    Config
	client *http.Client
}

// NewService prepares a client. It returns ErrNotConfigured when there is no
// URL, so a caller can tell "switched off" from "misconfigured".
func NewService(cfg Config) (*Service, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrNotConfigured
	}
	u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("transcribe: service url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("transcribe: service url needs http or https, got %q", cfg.BaseURL)
	}
	cfg.BaseURL = u.String()
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Service{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

// Endpoint is the URL this service posts to, which is also what goes in the
// note's metadata so a transcript can be traced to what produced it.
func (s *Service) Endpoint() string {
	return s.cfg.BaseURL + "/v1/audio/transcriptions"
}

// Transcribe posts one recording and returns what came back.
//
// filename is sent as the part's filename because the service decides how to
// decode by extension; audio with the wrong name is rejected by some servers
// and misread by others.
func (s *Service) Transcribe(ctx context.Context, filename string, audio io.Reader) (string, error) {
	body, contentType, err := s.buildForm(filename, audio)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint(), body)
	if err != nil {
		return "", fmt.Errorf("transcribe: building request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcribe: posting to %s: %w", s.Endpoint(), err)
	}
	defer resp.Body.Close()

	// Bounded: an error page from something that is not the service at all
	// should not be read into memory in full.
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("transcribe: reading reply: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("transcribe: service answered %s: %s",
			resp.Status, firstLine(payload))
	}

	var reply struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &reply); err != nil {
		return "", fmt.Errorf("transcribe: reply was not the expected JSON: %w", err)
	}
	text := strings.TrimSpace(reply.Text)
	if text == "" {
		// Silence, or a model that heard nothing. Not an error the
		// service reported, but no transcript either, and a note whose
		// content became an empty string would fail validation.
		return "", errors.New("transcribe: the service returned no text")
	}
	return text, nil
}

func (s *Service) buildForm(filename string, audio io.Reader) (io.Reader, string, error) {
	var buf strings.Builder
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", fmt.Errorf("transcribe: building request: %w", err)
	}
	if _, err := io.Copy(part, audio); err != nil {
		return nil, "", fmt.Errorf("transcribe: reading audio: %w", err)
	}
	if err := w.WriteField("model", s.cfg.Model); err != nil {
		return nil, "", fmt.Errorf("transcribe: building request: %w", err)
	}
	if s.cfg.Language != "" {
		if err := w.WriteField("language", s.cfg.Language); err != nil {
			return nil, "", fmt.Errorf("transcribe: building request: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("transcribe: building request: %w", err)
	}
	return strings.NewReader(buf.String()), w.FormDataContentType(), nil
}

// firstLine keeps an error message to one line of an error page.
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const maxLen = 300
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	if s == "" {
		return "(no body)"
	}
	return s
}
