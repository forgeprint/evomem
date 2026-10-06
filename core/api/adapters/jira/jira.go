// Package jira ingests notes from Jira Cloud webhooks.
//
// The payload shape and the signature scheme below were verified against the
// official documentation on 2026-10-06:
// https://developer.atlassian.com/cloud/jira/platform/webhooks/
//
// Jira signs the body rather than sending a shared secret in a header, so
// verification here is an HMAC over the bytes as they arrived. That is the
// difference from the Telegram adapter and the reason this one buffers the
// request.
package jira

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/forgeprint/evomem/shared/models"
)

// SignatureHeader carries the HMAC of the request body, formatted
// "method=signature" as WebSub defines — in practice "sha256=" and hex.
const SignatureHeader = "X-Hub-Signature"

// Origin is what a note from here is marked with.
const Origin = "jira"

// maxBody bounds a payload before it is hashed. A signature cannot be checked
// without the whole body in hand, so this is what stops an endpoint on the
// open internet being a way to exhaust memory.
const maxBody = 4 << 20

// Store is the part of the database this adapter needs.
type Store interface {
	Create(ctx context.Context, n *models.Note) error
}

// Handler answers Jira's webhook POSTs.
type Handler struct {
	store  Store
	secret string

	// project overrides the issue's own project key when set. Empty is the
	// normal case: an issue already says which project it belongs to, and
	// that is a better answer than anything configured here.
	project string
}

// New returns a handler. It refuses to start without a secret, for the same
// reason the Telegram adapter does.
func New(store Store, secret, project string) (*Handler, error) {
	if store == nil {
		return nil, errors.New("jira: no store")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("jira: no secret token; set one on the webhook and pass it here")
	}
	return &Handler{store: store, secret: secret, project: project}, nil
}

// The payload, as much of it as this adapter reads.
type event struct {
	WebhookEvent string     `json:"webhookEvent"`
	EventType    string     `json:"issue_event_type_name"`
	Timestamp    int64      `json:"timestamp"`
	User         *actor     `json:"user"`
	Issue        *issue     `json:"issue"`
	Comment      *comment   `json:"comment"`
	Changelog    *changelog `json:"changelog"`
}

type actor struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

type issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Fields issueFields `json:"fields"`
}

type issueFields struct {
	Summary     string   `json:"summary"`
	Description any      `json:"description"`
	Status      *named   `json:"status"`
	Priority    *named   `json:"priority"`
	IssueType   *named   `json:"issuetype"`
	Project     *project `json:"project"`
	Assignee    *actor   `json:"assignee"`
	Labels      []string `json:"labels"`
}

type named struct {
	Name string `json:"name"`
}

type project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type comment struct {
	ID     string `json:"id"`
	Body   any    `json:"body"`
	Author *actor `json:"author"`
}

type changelog struct {
	Items []changeItem `json:"items"`
}

type changeItem struct {
	Field      string `json:"field"`
	FromString string `json:"fromString"`
	ToString   string `json:"toString"`
}

// ServeHTTP answers one webhook delivery.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "payload too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	if !h.verify(r.Header.Get(SignatureHeader), body) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var ev event
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "malformed event", http.StatusBadRequest)
		return
	}

	note, ok := h.noteFrom(&ev)
	if !ok {
		writeOK(w, "ignored")
		return
	}
	if err := h.store.Create(r.Context(), note); err != nil {
		http.Error(w, "could not store the note", http.StatusInternalServerError)
		return
	}
	writeOK(w, note.ID)
}

func writeOK(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": id})
}

// verify checks the signature over the body.
//
// Only sha256 is accepted. The header's format allows a method prefix, and
// honouring a weaker one the sender chose would let an attacker pick the
// algorithm.
func (h *Handler) verify(header string, body []byte) bool {
	method, signature, found := strings.Cut(strings.TrimSpace(header), "=")
	if !found || !strings.EqualFold(method, "sha256") {
		return false
	}
	given, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	return hmac.Equal(given, mac.Sum(nil))
}

// noteFrom turns an event into a note, reporting false when there is nothing
// worth storing.
//
// One note per event, not one per issue kept up to date. A note is a thing
// that was true at a moment, and the history of an issue is what makes it
// worth remembering; collapsing it would throw away the part that answers
// "why is it like this".
func (h *Handler) noteFrom(ev *event) (*models.Note, bool) {
	if ev.Issue == nil || ev.Issue.Key == "" {
		// A project or a version event: no issue to file it under.
		return nil, false
	}

	projectID := h.project
	if projectID == "" {
		if ev.Issue.Fields.Project != nil && ev.Issue.Fields.Project.Key != "" {
			projectID = ev.Issue.Fields.Project.Key
		} else {
			// The key is PROJ-42, so the prefix is the project.
			projectID, _, _ = strings.Cut(ev.Issue.Key, "-")
		}
	}
	if projectID == "" {
		return nil, false
	}

	content := h.describe(ev)
	if strings.TrimSpace(content) == "" {
		return nil, false
	}

	n := &models.Note{
		ProjectID:  projectID,
		Content:    content,
		SourceType: models.SourceJira,
	}
	// A summary, a description and a comment are all written by whoever
	// has access to the Jira project.
	n.MarkTainted(Origin)

	f := ev.Issue.Fields
	n.SetMeta("jira_issue_key", ev.Issue.Key)
	if ev.WebhookEvent != "" {
		n.SetMeta("jira_event", ev.WebhookEvent)
	}
	if ev.EventType != "" {
		n.SetMeta("jira_event_type", ev.EventType)
	}
	if f.Status != nil && f.Status.Name != "" {
		n.SetMeta("status", f.Status.Name)
	}
	if f.IssueType != nil && f.IssueType.Name != "" {
		n.SetMeta("jira_issue_type", f.IssueType.Name)
	}
	if f.Priority != nil && f.Priority.Name != "" {
		n.SetMeta("jira_priority", f.Priority.Name)
	}
	if f.Assignee != nil && f.Assignee.DisplayName != "" {
		n.SetMeta("jira_assignee", f.Assignee.DisplayName)
	}
	if len(f.Labels) > 0 {
		n.SetMeta("jira_labels", f.Labels)
	}
	if ev.User != nil && ev.User.DisplayName != "" {
		n.SetMeta("jira_actor", ev.User.DisplayName)
	}
	return n, true
}

// describe writes what happened, in the words a person would use.
//
// The content is what the full-text index holds and what a model is shown, so
// it says the issue key and the summary even when the event is a one-field
// change: a note that reads "status: To Do -> In Progress" and nothing else
// is unsearchable and means nothing on its own.
func (h *Handler) describe(ev *event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s", ev.Issue.Key, strings.TrimSpace(ev.Issue.Fields.Summary))

	if ev.Comment != nil {
		if body := plainText(ev.Comment.Body); body != "" {
			who := "someone"
			if ev.Comment.Author != nil && ev.Comment.Author.DisplayName != "" {
				who = ev.Comment.Author.DisplayName
			}
			fmt.Fprintf(&b, "\n\nComment by %s:\n%s", who, body)
			return b.String()
		}
	}

	if ev.Changelog != nil && len(ev.Changelog.Items) > 0 {
		b.WriteString("\n")
		for _, item := range ev.Changelog.Items {
			from := item.FromString
			if from == "" {
				from = "(empty)"
			}
			to := item.ToString
			if to == "" {
				to = "(empty)"
			}
			fmt.Fprintf(&b, "\n%s: %s -> %s", item.Field, from, to)
		}
		return b.String()
	}

	if desc := plainText(ev.Issue.Fields.Description); desc != "" {
		fmt.Fprintf(&b, "\n\n%s", desc)
	}
	return b.String()
}

// plainText reads a Jira text field.
//
// It is a string on some instances and an Atlassian Document Format tree on
// others, and which one depends on the API version the webhook was created
// under. Rather than depend on that, a string is taken as it is and a document
// has its text nodes walked out of it.
func plainText(field any) string {
	switch v := field.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		var b strings.Builder
		walkADF(v, &b)
		return strings.TrimSpace(b.String())
	default:
		return ""
	}
}

func walkADF(node map[string]any, b *strings.Builder) {
	if text, ok := node["text"].(string); ok {
		b.WriteString(text)
	}
	content, ok := node["content"].([]any)
	if !ok {
		return
	}
	for _, child := range content {
		if m, ok := child.(map[string]any); ok {
			walkADF(m, b)
			// A paragraph is a block, and without this every block runs
			// into the next one word-first.
			if m["type"] == "paragraph" {
				b.WriteString("\n")
			}
		}
	}
}
