package connect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// What Atlassian documents, read 2026-10-09:
//
//   - GET /rest/api/3/search/jql, with `jql`, `nextPageToken`, `maxResults`
//     and `fields` as query parameters. The older /rest/api/3/search is still
//     listed and is not what this calls.
//     https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/
//   - Basic authentication with the account's **email** in the user position
//     and an **API token** in the password position; that page states
//     password authentication is deprecated.
//     https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/
//
// Two things developers report and the reference does not, which this guards
// against rather than trusting:
//
//   - Sending nextPageToken on the first request can be refused as invalid,
//     so the first call omits it.
//   - isLast is not always true when it should be, and a token can repeat,
//     so the loop stops on an absent token, a repeated one, or a page limit.
const (
	jiraSearchPath  = "/rest/api/3/search/jql"
	jiraPageSize    = 50
	jiraMaxPages    = 20
	jiraTimeout     = 30 * time.Second
	jiraMaxBodySize = 8 << 20
)

// JiraPuller reads issues out of a Jira Cloud site.
type JiraPuller struct {
	client *http.Client
}

// NewJiraPuller prepares a puller.
func NewJiraPuller() *JiraPuller {
	return &JiraPuller{client: &http.Client{Timeout: jiraTimeout}}
}

// jiraIssue is the part of an issue this reads. Everything else Jira sends is
// ignored rather than stored: a note is what somebody wrote, and the rest is
// Jira's bookkeeping.
type jiraIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary     string `json:"summary"`
		Description any    `json:"description"`
		Updated     string `json:"updated"`
		Status      struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

type jiraPage struct {
	Issues        []jiraIssue `json:"issues"`
	IsLast        bool        `json:"isLast"`
	NextPageToken string      `json:"nextPageToken"`
}

// Pull reads every issue the query matches, following the vendor's cursor.
func (p *JiraPuller) Pull(
	ctx context.Context,
	conn *database.Connection,
	secret string,
) ([]*models.Note, string, error) {
	query := strings.TrimSpace(conn.Query)
	if query == "" {
		// Without a query this would ask for every issue the token can
		// see, which is rarely what somebody meant and always slow.
		return nil, conn.Cursor, fmt.Errorf("jira: this connection has no JQL query")
	}

	var (
		notes []*models.Note
		token = conn.Cursor
		seen  = map[string]bool{}
	)

	for page := 0; page < jiraMaxPages; page++ {
		result, err := p.fetch(ctx, conn, secret, query, token)
		if err != nil {
			return notes, conn.Cursor, err
		}

		for _, issue := range result.Issues {
			notes = append(notes, noteFromIssue(conn, issue))
		}

		next := strings.TrimSpace(result.NextPageToken)
		if next == "" || result.IsLast {
			// The end, as the vendor reports it.
			return notes, "", nil
		}
		if seen[next] {
			// The same token twice is a loop, which has been
			// reported. Stop and keep what arrived.
			return notes, "", nil
		}
		seen[next] = true
		token = next
	}

	// Out of pages rather than out of issues: the cursor is kept so the
	// next run carries on instead of starting over.
	return notes, token, nil
}

func (p *JiraPuller) fetch(
	ctx context.Context,
	conn *database.Connection,
	secret, query, token string,
) (*jiraPage, error) {
	params := url.Values{}
	params.Set("jql", query)
	params.Set("maxResults", fmt.Sprint(jiraPageSize))
	params.Set("fields", "summary,description,updated,status")
	// Omitted on the first request on purpose: sending it has been
	// refused as invalid.
	if token != "" {
		params.Set("nextPageToken", token)
	}

	endpoint := strings.TrimRight(conn.BaseURL, "/") + jiraSearchPath + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jira: building the request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// email:token, base64, as the vendor documents.
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(conn.Account+":"+secret)))

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira: calling %s: %w", conn.BaseURL, redactSecret(err, secret))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, jiraMaxBodySize))
	if err != nil {
		return nil, fmt.Errorf("jira: reading the answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira: answered %s: %s", resp.Status, firstLine(body))
	}

	var page jiraPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("jira: the answer was not the expected JSON: %w", err)
	}
	return &page, nil
}

// noteFromIssue turns an issue into a note.
//
// The summary is the content, because that is the sentence somebody wrote and
// what a search should match. The key, the status and the description's
// plain text go in the metadata, where they can be read without becoming the
// note's words.
func noteFromIssue(conn *database.Connection, issue jiraIssue) *models.Note {
	metadata := map[string]any{
		"jira_issue_key": issue.Key,
		"jira_issue_id":  issue.ID,
		"jira_base_url":  strings.TrimRight(conn.BaseURL, "/"),
	}
	if status := issue.Fields.Status.Name; status != "" {
		metadata["jira_status"] = status
	}
	if text := plainText(issue.Fields.Description); text != "" {
		metadata["jira_description"] = text
	}

	content := strings.TrimSpace(issue.Fields.Summary)
	if content == "" {
		content = issue.Key
	} else {
		content = issue.Key + ": " + content
	}

	note := NoteFrom(string(models.SourceJira), content, metadata, time.Time{})
	if updated, err := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Updated); err == nil {
		note.CreatedAt = updated.UTC()
	}
	return note
}

// plainText pulls readable text out of a description.
//
// Jira's v3 API sends Atlassian Document Format, a tree of nodes rather than
// a string. Only the text leaves are taken: rebuilding the formatting would
// mean implementing somebody else's document model, and what is wanted here
// is something searchable.
func plainText(node any) string {
	var b strings.Builder
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case string:
			b.WriteString(typed)
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			if text, ok := typed["text"].(string); ok {
				b.WriteString(text)
				b.WriteString(" ")
			}
			if content, ok := typed["content"]; ok {
				walk(content)
			}
		}
	}
	walk(node)
	return strings.TrimSpace(b.String())
}

// redactSecret keeps a token out of an error that quotes a URL or a header.
func redactSecret(err error, secret string) error {
	if secret == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), secret, "<token>"))
}

// firstLine keeps an error page to one line.
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
