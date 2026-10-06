// Package telegram ingests notes from Telegram Bot API webhook updates.
//
// The payload shapes and the secret header below were verified against the
// official documentation on 2026-10-06: https://core.telegram.org/bots/api
// Nothing here is written from memory, because a field name guessed wrong
// fails silently — an absent field decodes as a zero value, not an error.
package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/forgeprint/evomem/shared/models"
)

// SecretHeader is the header Telegram sends when a webhook was registered
// with setWebhook's secret_token.
const SecretHeader = "X-Telegram-Bot-Api-Secret-Token"

// Origin is what a note from here is marked with.
const Origin = "telegram"

// Store is the part of the database this adapter needs.
type Store interface {
	Create(ctx context.Context, n *models.Note) error
}

// Handler answers Telegram's webhook POSTs.
type Handler struct {
	store Store

	// secret is the token given to setWebhook. A webhook endpoint is
	// reachable by anyone who learns the URL, so this is the only thing
	// between the store and the open internet.
	secret string

	// project is the project every note from this bot belongs to.
	//
	// One bot, one project, deliberately. Routing by chat, by hashtag or
	// by command are all defensible and they contradict each other; until
	// there is a reason to pick one, the chat is recorded in the metadata
	// so a later version can route on it without a migration.
	project string
}

// New returns a handler. It fails rather than start without a secret: an
// unauthenticated ingest endpoint is a way for anyone to write to the user's
// memory, and a note once written is read by a model later.
func New(store Store, secret, project string) (*Handler, error) {
	if store == nil {
		return nil, errors.New("telegram: no store")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("telegram: no secret token; set one with setWebhook and pass it here")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("telegram: no project to file notes under")
	}
	return &Handler{store: store, secret: secret, project: project}, nil
}

// The payload, as much of it as this adapter reads. Every field is optional in
// the API, so a missing one has to be ordinary rather than an error.
type update struct {
	UpdateID      int64    `json:"update_id"`
	Message       *message `json:"message"`
	EditedMessage *message `json:"edited_message"`
	ChannelPost   *message `json:"channel_post"`
}

type message struct {
	MessageID int64  `json:"message_id"`
	Date      int64  `json:"date"`
	From      *user  `json:"from"`
	Chat      *chat  `json:"chat"`
	Text      string `json:"text"`
	Caption   string `json:"caption"`
	Voice     *voice `json:"voice"`
	Audio     *audio `json:"audio"`
}

type user struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MIMEType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MIMEType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	Title        string `json:"title"`
}

// ServeHTTP answers one update.
//
// Telegram retries an update it did not get a 2xx for, and retries the whole
// backlog behind it. So anything that is not worth storing — a sticker, a
// bot's own message, an update type this adapter does not read — is answered
// 200 with nothing written, rather than 4xx. An error status is reserved for
// what a retry could actually fix.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Constant time, so the comparison does not leak the secret one byte
	// at a time to someone who can measure the response.
	given := r.Header.Get(SecretHeader)
	if subtle.ConstantTimeCompare([]byte(given), []byte(h.secret)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var u update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "malformed update", http.StatusBadRequest)
		return
	}

	note, ok := h.noteFrom(&u)
	if !ok {
		writeOK(w, "ignored")
		return
	}
	if err := h.store.Create(r.Context(), note); err != nil {
		// Worth a retry: the store was busy or the disk was full.
		http.Error(w, "could not store the note", http.StatusInternalServerError)
		return
	}
	writeOK(w, note.ID)
}

func writeOK(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": id})
}

// noteFrom turns an update into a note, reporting false when there is nothing
// worth storing.
func (h *Handler) noteFrom(u *update) (*models.Note, bool) {
	msg := u.Message
	edited := false
	switch {
	case msg != nil:
	case u.EditedMessage != nil:
		msg, edited = u.EditedMessage, true
	case u.ChannelPost != nil:
		msg = u.ChannelPost
	default:
		return nil, false
	}

	// A bot's own messages would otherwise come back as notes, including
	// this project's own replies.
	if msg.From != nil && msg.From.IsBot {
		return nil, false
	}

	content, meta, ok := describe(msg)
	if !ok {
		return nil, false
	}

	n := &models.Note{
		ProjectID:  h.project,
		Content:    content,
		SourceType: models.SourceTelegram,
	}
	// Every field below is third-party text. The flag is what tells a
	// model reading this later that it is data, not instruction.
	n.MarkTainted(Origin)
	n.SetMeta("telegram_update_id", u.UpdateID)
	n.SetMeta("telegram_message_id", msg.MessageID)
	if edited {
		n.SetMeta("telegram_edited", true)
	}
	if msg.Chat != nil {
		n.SetMeta("telegram_chat_id", msg.Chat.ID)
		if msg.Chat.Type != "" {
			n.SetMeta("telegram_chat_type", msg.Chat.Type)
		}
	}
	if msg.From != nil {
		n.SetMeta("telegram_from_id", msg.From.ID)
		if msg.From.Username != "" {
			n.SetMeta("telegram_from_username", msg.From.Username)
		}
	}
	for k, v := range meta {
		n.SetMeta(k, v)
	}
	return n, true
}

// describe gives the note's content and any extra metadata.
//
// A voice message has no text at all, and this phase does not transcribe it.
// What it can record is that one arrived and how to fetch it: the file_id is
// what getFile takes, so a later phase can download and transcribe without
// having lost the message. The content says so in words, because the content
// is what the full-text index holds and what a model is shown.
func describe(msg *message) (string, map[string]any, bool) {
	if text := strings.TrimSpace(msg.Text); text != "" {
		return text, nil, true
	}

	meta := map[string]any{}
	switch {
	case msg.Voice != nil:
		meta["telegram_file_id"] = msg.Voice.FileID
		meta["telegram_file_unique_id"] = msg.Voice.FileUniqueID
		meta["telegram_duration_seconds"] = msg.Voice.Duration
		if msg.Voice.MIMEType != "" {
			meta["telegram_mime_type"] = msg.Voice.MIMEType
		}
		meta["awaiting_transcription"] = true
		return voiceContent("Voice message", msg.Voice.Duration, msg.Caption), meta, true

	case msg.Audio != nil:
		meta["telegram_file_id"] = msg.Audio.FileID
		meta["telegram_file_unique_id"] = msg.Audio.FileUniqueID
		meta["telegram_duration_seconds"] = msg.Audio.Duration
		if msg.Audio.MIMEType != "" {
			meta["telegram_mime_type"] = msg.Audio.MIMEType
		}
		meta["awaiting_transcription"] = true
		title := "Audio"
		if msg.Audio.Title != "" {
			title = "Audio: " + msg.Audio.Title
		}
		return voiceContent(title, msg.Audio.Duration, msg.Caption), meta, true

	case strings.TrimSpace(msg.Caption) != "":
		// A photo or a document with something written under it. The
		// caption is the part worth remembering.
		return strings.TrimSpace(msg.Caption), nil, true
	}

	// A sticker, a location, someone joining a group: nothing to remember.
	return "", nil, false
}

func voiceContent(kind string, seconds int, caption string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s, %ds, not yet transcribed.", kind, seconds)
	if c := strings.TrimSpace(caption); c != "" {
		fmt.Fprintf(&b, "\n%s", c)
	}
	return b.String()
}
