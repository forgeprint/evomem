package transcribe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeTelegram answers getFile and the download that follows it, the way the
// Bot API documents: an envelope with ok and result for the first, the raw
// bytes under /file/bot<token>/<file_path> for the second.
func fakeTelegram(t *testing.T, getFile string, status int, audio string) *TelegramFiles {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			_, _ = io.WriteString(w, getFile)
		case strings.Contains(r.URL.Path, "/file/bot"):
			w.WriteHeader(status)
			_, _ = io.WriteString(w, audio)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	files, err := NewTelegramFiles("123:ABC", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestTelegramNeedsAToken(t *testing.T) {
	// serve knows the webhook secret and not the bot token, so an unset
	// token is an ordinary configuration, not a mistake.
	if _, err := NewTelegramFiles("", ""); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestTelegramFetchesAndKeepsTheExtension(t *testing.T) {
	files := fakeTelegram(t,
		`{"ok":true,"result":{"file_id":"AwACAgQ","file_unique_id":"u","file_size":11,"file_path":"voice/file_7.oga"}}`,
		http.StatusOK, "OggS-pretend")

	audio, filename, err := files.Fetch(context.Background(), "AwACAgQ")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "OggS-pretend" {
		t.Errorf("audio = %q", audio)
	}
	// The API warns the original name may not be preserved; what is kept
	// is the extension, which is what the transcription service needs.
	if filename != "file_7.oga" {
		t.Errorf("filename = %q", filename)
	}
}

func TestTelegramReportsWhyGetFileRefused(t *testing.T) {
	// The description is the only thing that distinguishes a stale file_id
	// from a wrong token, and it ends up on the note.
	files := fakeTelegram(t,
		`{"ok":false,"error_code":400,"description":"Bad Request: file is temporarily unavailable"}`,
		http.StatusOK, "")

	_, _, err := files.Fetch(context.Background(), "AwACAgQ")
	if err == nil {
		t.Fatal("a refusal was accepted")
	}
	if !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Errorf("err = %v", err)
	}
}

func TestTelegramRefusesAFileWithoutAPath(t *testing.T) {
	// file_path is optional in the File type, and one without it cannot be
	// downloaded at all.
	files := fakeTelegram(t, `{"ok":true,"result":{"file_id":"x","file_unique_id":"u"}}`,
		http.StatusOK, "")

	if _, _, err := files.Fetch(context.Background(), "x"); err == nil {
		t.Fatal("a File with no file_path was accepted")
	}
}

func TestTelegramRefusesWhatItCannotDownload(t *testing.T) {
	// 20 MB is the API's limit. Refused on the stated size, before the
	// download, so the note says so rather than timing out.
	files := fakeTelegram(t,
		fmt.Sprintf(`{"ok":true,"result":{"file_id":"x","file_unique_id":"u","file_size":%d,"file_path":"a.oga"}}`,
			TelegramMaxDownloadBytes+1),
		http.StatusOK, "")

	_, _, err := files.Fetch(context.Background(), "x")
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestTelegramRefusesABodyThatLiedAboutItsSize(t *testing.T) {
	// file_size is optional, so the read is bounded independently of it.
	files := fakeTelegram(t,
		`{"ok":true,"result":{"file_id":"x","file_unique_id":"u","file_path":"a.oga"}}`,
		http.StatusOK, strings.Repeat("a", TelegramMaxDownloadBytes+1))

	_, _, err := files.Fetch(context.Background(), "x")
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestTelegramRefusesAnEmptyDownload(t *testing.T) {
	files := fakeTelegram(t,
		`{"ok":true,"result":{"file_id":"x","file_unique_id":"u","file_path":"a.oga"}}`,
		http.StatusOK, "")

	if _, _, err := files.Fetch(context.Background(), "x"); err == nil {
		t.Fatal("an empty recording was accepted")
	}
}

func TestTelegramNeedsAFileID(t *testing.T) {
	files := fakeTelegram(t, `{"ok":true}`, http.StatusOK, "")
	if _, _, err := files.Fetch(context.Background(), "  "); err == nil {
		t.Fatal("an empty file_id was accepted")
	}
}

// The token is in the URL of every one of these requests, and these errors
// are written to a note and to a log.
func TestTheTokenIsKeptOutOfErrors(t *testing.T) {
	const token = "123:SECRET-TOKEN"
	// A host nothing is listening on, so net/http reports the URL it tried.
	files, err := NewTelegramFiles(token, "http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = files.Fetch(context.Background(), "x")
	if err == nil {
		t.Fatal("expected a failure")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("the bot token is in the error: %v", err)
	}
	if !strings.Contains(err.Error(), "<token>") {
		t.Errorf("err = %v; the redaction should be visible", err)
	}
}

func TestTelegramRefusesANonsenseAPIURL(t *testing.T) {
	// A typo here would otherwise send the bot token somewhere unintended.
	for _, base := range []string{"api.telegram.org", "ftp://host"} {
		if _, err := NewTelegramFiles("123:ABC", base); err == nil {
			t.Errorf("NewTelegramFiles(_, %q) was accepted", base)
		}
	}
}

func TestTelegramDefaultsToTelegram(t *testing.T) {
	files, err := NewTelegramFiles("123:ABC", "")
	if err != nil {
		t.Fatal(err)
	}
	if files.baseURL != telegramAPIBase {
		t.Errorf("baseURL = %q, want %q", files.baseURL, telegramAPIBase)
	}
}
