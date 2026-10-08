package transcribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// What the Bot API will and will not do, from the reference read 2026-10-08
// against Bot API 10.3 (24 August 2026), getFile and the File type:
//
//   - "For the moment, bots can download files of up to 20MB in size."
//   - getFile takes one parameter, file_id, and returns a File whose
//     file_path is optional.
//   - The file is fetched from
//     https://api.telegram.org/file/bot<token>/<file_path>, and that link is
//     "guaranteed to be valid for at least 1 hour". When it expires another
//     getFile gives a new one — which is why the fetch happens now, at
//     transcription time, and not when the webhook arrived.
//   - Every reply is an envelope with a boolean `ok`, the payload in
//     `result`, and a human-readable `description` when ok is false.
//   - "This function may not preserve the original file name and MIME type."
//
// https://core.telegram.org/bots/api#getfile
const (
	// TelegramMaxDownloadBytes is the 20 MB the API will hand over, and
	// also the bound on what this will hold in memory. A longer recording
	// is refused here rather than part-way through a download, so the note
	// says why.
	TelegramMaxDownloadBytes = 20 << 20

	telegramAPIBase  = "https://api.telegram.org"
	telegramTimeout  = 2 * time.Minute
	telegramMetaBody = 64 << 10
)

// ErrTooLarge is a recording the sender will not hand over. Trying again will
// not help, and the note records that.
var ErrTooLarge = errors.New("transcribe: recording is larger than the sender will download")

// TelegramFiles fetches recordings Telegram is holding.
//
// It needs the bot token, which the webhook side of evomem does not have:
// `evomem serve` knows only the webhook secret. Downloading is the first
// thing evomem does *to* Telegram rather than from it.
type TelegramFiles struct {
	token   string
	baseURL string
	client  *http.Client
}

// NewTelegramFiles prepares a downloader. An empty token means Telegram
// recordings cannot be fetched, reported as ErrNotConfigured.
//
// baseURL is empty for Telegram's own servers. Telegram also documents a
// self-hosted Bot API server, which is the other thing it can point at; the
// size bound below still applies either way, because this reads the whole
// recording into memory and that bound is evomem's as much as the API's.
func NewTelegramFiles(token, baseURL string) (*TelegramFiles, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrNotConfigured
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = telegramAPIBase
	} else {
		u, err := url.Parse(base)
		if err != nil {
			return nil, fmt.Errorf("transcribe: telegram api url: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("transcribe: telegram api url needs http or https, got %q", baseURL)
		}
	}
	return &TelegramFiles{
		token:   token,
		baseURL: base,
		client:  &http.Client{Timeout: telegramTimeout},
	}, nil
}

// Fetch downloads the file behind a file_id and returns its bytes and the
// name Telegram gave it.
//
// The name matters: the transcription service decides how to decode by
// extension, and the API warns that it "may not preserve the original file
// name". What comes back is the path's base, which carries the extension even
// when the rest is a generated name.
func (t *TelegramFiles) Fetch(ctx context.Context, fileID string) ([]byte, string, error) {
	if strings.TrimSpace(fileID) == "" {
		return nil, "", errors.New("transcribe: no telegram file_id on the note")
	}

	filePath, size, err := t.describe(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	// Refused on the stated size before the download, when the API gave
	// one. file_size is optional, so a missing one is not a reason to
	// skip: the read below is bounded either way.
	if size > TelegramMaxDownloadBytes {
		return nil, "", fmt.Errorf("%w: %d bytes, limit is %d",
			ErrTooLarge, size, TelegramMaxDownloadBytes)
	}

	audio, err := t.download(ctx, filePath)
	if err != nil {
		return nil, "", err
	}
	return audio, path.Base(filePath), nil
}

// describe calls getFile for a path to download from.
func (t *TelegramFiles) describe(ctx context.Context, fileID string) (string, int64, error) {
	endpoint := t.baseURL + "/bot" + t.token + "/getFile?file_id=" + url.QueryEscape(fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", 0, fmt.Errorf("transcribe: building getFile request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		// The token is in the URL, so the error is reported without it.
		return "", 0, fmt.Errorf("transcribe: calling getFile: %w", redactToken(err, t.token))
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, telegramMetaBody))
	if err != nil {
		return "", 0, fmt.Errorf("transcribe: reading getFile reply: %w", err)
	}

	var envelope struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			FilePath string `json:"file_path"`
			FileSize int64  `json:"file_size"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", 0, fmt.Errorf("transcribe: getFile reply was not the expected JSON: %w", err)
	}
	if !envelope.OK {
		// The description is what the API says went wrong; the status
		// line alone does not distinguish a stale file_id from a bad
		// token.
		reason := envelope.Description
		if reason == "" {
			reason = resp.Status
		}
		return "", 0, fmt.Errorf("transcribe: getFile refused: %s", reason)
	}
	if envelope.Result.FilePath == "" {
		// file_path is optional in the type, and a File without one
		// cannot be downloaded at all.
		return "", 0, errors.New("transcribe: getFile returned no file_path")
	}
	return envelope.Result.FilePath, envelope.Result.FileSize, nil
}

func (t *TelegramFiles) download(ctx context.Context, filePath string) ([]byte, error) {
	endpoint := t.baseURL + "/file/bot" + t.token + "/" + filePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("transcribe: building download request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcribe: downloading recording: %w", redactToken(err, t.token))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transcribe: downloading recording: %s", resp.Status)
	}

	// One byte past the limit, so a body that lies about its length — or
	// arrives without one — is caught rather than truncated into audio the
	// service would transcribe as half a sentence.
	audio, err := io.ReadAll(io.LimitReader(resp.Body, TelegramMaxDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("transcribe: downloading recording: %w", err)
	}
	if len(audio) > TelegramMaxDownloadBytes {
		return nil, fmt.Errorf("%w: over %d bytes", ErrTooLarge, TelegramMaxDownloadBytes)
	}
	if len(audio) == 0 {
		return nil, errors.New("transcribe: downloaded recording is empty")
	}
	return audio, nil
}

// redactToken keeps the bot token out of an error that quotes the URL.
// net/http errors carry the request URL, and these errors end up on a note
// and in a log.
func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), token, "<token>")
	return errors.New(msg)
}
