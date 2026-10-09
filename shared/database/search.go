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
	terms := searchTerms(q.Text)
	if len(terms) == 0 {
		// Nothing searchable was given. An empty expression is a
		// syntax error in both query languages, and a search for
		// nothing is not an error.
		return nil, nil
	}

	var query string
	var args []any
	if d.read.dialect == dialectPostgres {
		query, args = postgresSearch(terms, q)
	} else {
		query, args = sqliteSearch(terms, q)
	}

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
			rank       float64
		)
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Content, &sourceType,
			&created, &updated, &metadata, &snippet, &rank); err != nil {
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

		// Score is higher-is-better for the caller. SQLite's bm25 is
		// smaller-is-better and is negated; ts_rank_cd is already the
		// right way round.
		score := rank
		if d.read.dialect != dialectPostgres {
			score = -rank
		}
		out = append(out, SearchHit{Note: &n, Snippet: snippet, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: searching for %q: %w", q.Text, err)
	}
	return out, nil
}

// searchTerms turns what a person typed into words that are safe to put in a
// query language.
//
// Neither MATCH nor to_tsquery takes a string: AND, OR, NOT, parentheses,
// quotes and colons all mean something in one or both, and an unbalanced
// quote or a trailing operator comes back as a failed query. Text from a chat
// message or a webhook will contain those characters sooner or later, so none
// of it is passed through — every run of letters and digits is one term and
// everything else is dropped.
//
// The cost is that a user cannot write a search expression even deliberately.
// That is the right way round: the alternative is a search box that throws
// errors at apostrophes. It is also what makes the two builders below safe,
// since a term here cannot carry syntax into either language.
func searchTerms(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

// sqliteSearch builds the FTS5 query (ADR-0005).
//
// Terms are double-quoted and joined by FTS5's implicit AND, so all of them
// have to appear.
func sqliteSearch(terms []string, q SearchQuery) (string, []any) {
	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + term + `"`
	}
	if q.Prefix {
		// Only the last term: the earlier ones are words the user
		// finished typing.
		quoted[len(quoted)-1] += `*`
	}

	query := `
		SELECT n.id, n.project_id, n.content, n.source_type, n.created_at, n.updated_at, n.metadata,
		       snippet(notes_fts, 0, ?, ?, '…', ?), bm25(notes_fts)
		FROM notes_fts
		JOIN notes n ON n.rowid = notes_fts.rowid
		WHERE notes_fts MATCH ?`
	args := []any{SnippetOpen, SnippetClose, snippetTokens, strings.Join(quoted, " ")}

	query, args = narrow(query, args, q)
	query += ` ORDER BY bm25(notes_fts), n.created_at DESC LIMIT ? OFFSET ?`
	return query, append(args, clampLimit(q.Limit), max(0, q.Offset))
}

// postgresSearch builds the tsquery (ADR-0028).
//
// `&` is to_tsquery's AND, so this matches FTS5's implicit one: all the terms
// have to appear. `:*` is its prefix marker.
//
// The configuration is named in every call rather than left to
// default_text_search_config, because the GIN index was built over
// `to_tsvector('evomem', content)` and an index is only used when the
// expression matches exactly.
func postgresSearch(terms []string, q SearchQuery) (string, []any) {
	joined := strings.Join(terms, " & ")
	if q.Prefix {
		joined += ":*"
	}

	// MinWords=1 so a one-word note still produces a headline, and
	// HighlightAll=false so a long note is cut rather than returned whole.
	const headlineOptions = "StartSel=" + SnippetOpen + ",StopSel=" + SnippetClose +
		",MaxWords=32,MinWords=1,ShortWord=0,HighlightAll=false"

	query := `
		SELECT n.id, n.project_id, n.content, n.source_type, n.created_at, n.updated_at, n.metadata,
		       ts_headline('evomem', n.content, q.q, ?),
		       ts_rank_cd(to_tsvector('evomem', n.content), q.q)
		FROM notes n, to_tsquery('evomem', ?) AS q(q)
		WHERE to_tsvector('evomem', n.content) @@ q.q`
	args := []any{headlineOptions, joined}

	query, args = narrow(query, args, q)
	// DESC: ts_rank_cd is higher-is-better, the opposite of bm25.
	query += ` ORDER BY ts_rank_cd(to_tsvector('evomem', n.content), q.q) DESC,
		n.created_at DESC LIMIT ? OFFSET ?`
	return query, append(args, clampLimit(q.Limit), max(0, q.Offset))
}

// narrow adds the filters both builders share.
func narrow(query string, args []any, q SearchQuery) (string, []any) {
	if q.ProjectID != "" {
		query += ` AND n.project_id = ?`
		args = append(args, q.ProjectID)
	}
	if q.SourceType != "" {
		query += ` AND n.source_type = ?`
		args = append(args, string(q.SourceType))
	}
	return query, args
}
