package database

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/forgeprint/evomem/shared/models"
)

// SearchQuery is one full-text search.
type SearchQuery struct {
	// Text is what a person typed. It is not an FTS5 expression and is
	// not treated as one; see ftsExpression.
	Text string

	// ProjectID narrows the search to one project when set.
	ProjectID string

	// SourceType narrows it to one source when set.
	SourceType models.SourceType

	// Prefix matches on the last word as a prefix, which is what
	// search-as-you-type needs.
	Prefix bool

	Limit  int
	Offset int
}

// SearchHit is a note that matched, with the part that matched.
type SearchHit struct {
	Note *models.Note `json:"note"`

	// Snippet is the matching stretch of the content with the matched
	// terms wrapped in SnippetOpen and SnippetClose.
	Snippet string `json:"snippet"`

	// Score ranks the hit, higher being a better match. It is BM25
	// negated: SQLite returns a smaller-is-better figure, and a caller
	// comparing two hits should not have to remember that.
	Score float64 `json:"score"`
}

// How a snippet marks the matched terms. Square brackets rather than HTML,
// because the first consumer is an LLM reading plain text and the second is a
// terminal.
const (
	SnippetOpen  = "["
	SnippetClose = "]"

	// snippetTokens is how many tokens of context a snippet carries. 32 is
	// about a line and a half: enough to judge a hit, short enough that
	// twenty of them still fit in a prompt.
	snippetTokens = 32
)

// Search runs a full-text query over the notes, best match first.
func (d *DB) Search(ctx context.Context, q SearchQuery) ([]SearchHit, error) {
	expr := ftsExpression(q.Text, q.Prefix)
	if expr == "" {
		// Nothing searchable was given. An empty FTS5 expression is a
		// syntax error, and a search for nothing is not an error.
		return nil, nil
	}

	query := `
		SELECT n.id, n.project_id, n.content, n.source_type, n.created_at, n.updated_at, n.metadata,
		       snippet(notes_fts, 0, ?, ?, '…', ?), bm25(notes_fts)
		FROM notes_fts
		JOIN notes n ON n.rowid = notes_fts.rowid
		WHERE notes_fts MATCH ?`
	args := []any{SnippetOpen, SnippetClose, snippetTokens, expr}

	if q.ProjectID != "" {
		query += ` AND n.project_id = ?`
		args = append(args, q.ProjectID)
	}
	if q.SourceType != "" {
		query += ` AND n.source_type = ?`
		args = append(args, string(q.SourceType))
	}
	query += ` ORDER BY bm25(notes_fts), n.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, clampLimit(q.Limit), max(0, q.Offset))

	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: searching for %q: %w", q.Text, err)
	}
	defer rows.Close()

	var out []SearchHit
	for rows.Next() {
		var (
			n          models.Note
			sourceType string
			created    string
			updated    string
			metadata   string
			snippet    string
			bm25       float64
		)
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Content, &sourceType,
			&created, &updated, &metadata, &snippet, &bm25); err != nil {
			return nil, fmt.Errorf("database: searching for %q: %w", q.Text, err)
		}

		n.SourceType = models.SourceType(sourceType)
		if n.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if n.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		if err := n.UnmarshalMetadata(metadata); err != nil {
			return nil, err
		}

		out = append(out, SearchHit{Note: &n, Snippet: snippet, Score: -bm25})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: searching for %q: %w", q.Text, err)
	}
	return out, nil
}

// ftsExpression turns what a person typed into an FTS5 expression.
//
// FTS5 MATCH takes a query language, not a string: AND, OR, NOT, NEAR, column
// filters, parentheses and quotes all mean something in it, and an unbalanced
// quote or a trailing OR is a syntax error that comes back as a failed query.
// Text from a chat message or a webhook will contain those characters sooner
// or later, so none of it is passed through. Every run of word characters
// becomes one double-quoted term and everything else is dropped; the terms are
// joined by FTS5's implicit AND, so all of them have to appear.
//
// The cost is that a user cannot write an FTS5 expression even deliberately.
// That is the right way round: the alternative is a search box that throws
// errors at apostrophes.
func ftsExpression(text string, prefix bool) string {
	var terms []string
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		terms = append(terms, `"`+field+`"`)
	}
	if len(terms) == 0 {
		return ""
	}
	if prefix {
		// Only the last term: the earlier ones are words the user
		// finished typing.
		terms[len(terms)-1] += `*`
	}
	return strings.Join(terms, " ")
}
