package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

// waitingVoiceNote puts one recording in the store, the way the Telegram
// adapter would.
func waitingVoiceNote(t *testing.T) {
	t.Helper()
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	n := &models.Note{
		ProjectID:  "evomem",
		Content:    "Voice message, 0:14",
		SourceType: models.SourceTelegram,
	}
	n.MarkTainted("telegram")
	n.SetMeta(models.MetaAwaitingTranscription, true)
	n.SetMeta("telegram_file_id", "AwACAgQ")
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
}

func TestTranscribeSaysNothingIsWaiting(t *testing.T) {
	withStore(t)
	if got := exec(t, "", "transcribe"); !strings.Contains(got, "nothing is waiting") {
		t.Errorf("got %q", got)
	}
}

// An unconfigured service is the feature being off, not an error: the command
// says what would happen and exits zero.
func TestTranscribeIsOffWithoutAService(t *testing.T) {
	withStore(t)
	t.Setenv("EVOMEM_TRANSCRIPTION_URL", "")
	waitingVoiceNote(t)

	got := exec(t, "", "transcribe")
	if !strings.Contains(got, "transcription is off") {
		t.Errorf("got %q", got)
	}
	if !strings.Contains(got, "EVOMEM_TRANSCRIPTION_URL") {
		t.Errorf("it does not say how to turn it on: %q", got)
	}
	if !strings.Contains(got, "1 recording waiting") {
		t.Errorf("it does not say how much is waiting: %q", got)
	}
}

func TestTranscribeRefusesANonsenseService(t *testing.T) {
	withStore(t)
	waitingVoiceNote(t)
	t.Setenv("EVOMEM_TRANSCRIPTION_URL", "whisper.example:8001")

	if err := run([]string{"transcribe"}, os.Stdout, strings.NewReader("")); err == nil {
		t.Error("a url with no scheme was accepted")
	}
}

// Nothing is sent anywhere by a dry run, so this needs no service to answer.
func TestTranscribeDryRunChangesNothing(t *testing.T) {
	withStore(t)
	waitingVoiceNote(t)
	t.Setenv("EVOMEM_TRANSCRIPTION_URL", "http://127.0.0.1:1")
	t.Setenv("EVOMEM_TELEGRAM_BOT_TOKEN", "123:ABC")

	got := exec(t, "", "transcribe", "-dry-run")
	if !strings.Contains(got, "would transcribe") {
		t.Errorf("got %q", got)
	}

	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	waiting, err := db.AwaitingTranscriptionCount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if waiting != 1 {
		t.Errorf("%d still waiting after a dry run, want 1", waiting)
	}
}

// Without the bot token there is no way to reach a Telegram recording, which
// the command says rather than failing.
func TestTranscribeSaysWhenItCannotDownload(t *testing.T) {
	withStore(t)
	waitingVoiceNote(t)
	t.Setenv("EVOMEM_TRANSCRIPTION_URL", "http://127.0.0.1:1")
	t.Setenv("EVOMEM_TELEGRAM_BOT_TOKEN", "")

	got := exec(t, "", "transcribe")
	if !strings.Contains(got, "EVOMEM_TELEGRAM_BOT_TOKEN") {
		t.Errorf("got %q", got)
	}
	if !strings.Contains(got, "skipped") {
		t.Errorf("the note was not reported as skipped: %q", got)
	}
}

// The queue is worth announcing where someone is already looking.
func TestSyncStatusMentionsWaitingRecordings(t *testing.T) {
	withStore(t)
	if strings.Contains(exec(t, "", "sync-status"), "to be transcribed") {
		t.Error("an empty queue was announced")
	}

	waitingVoiceNote(t)
	if !strings.Contains(exec(t, "", "sync-status"), "1 waiting to be transcribed") {
		t.Errorf("got %s", exec(t, "", "sync-status"))
	}
}

func TestUsageMentionsTranscribe(t *testing.T) {
	withStore(t)
	got := exec(t, "", "help")
	for _, want := range []string{"evomem transcribe", "EVOMEM_TRANSCRIPTION_URL", "EVOMEM_TELEGRAM_BOT_TOKEN"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage does not mention %s", want)
		}
	}
}
