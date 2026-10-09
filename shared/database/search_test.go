package database

import (
	"context"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

func TestSearchFindsAWord(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.CreateBatch(ctx, []*models.Note{
		newNote("evomem", "the gateway returned a 502 under load"),
		newNote("evomem", "remember to vendor the sqlite driver"),
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "gateway"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if !strings.Contains(hits[0].Note.Content, "gateway") {
		t.Errorf("the wrong note matched: %q", hits[0].Note.Content)
	}
	if !strings.Contains(hits[0].Snippet, SnippetOpen+"gateway"+SnippetClose) {
		t.Errorf("the snippet does not mark the match: %q", hits[0].Snippet)
	}
}

// Two words mean both words. An OR here would make every search return
// everything.
func TestSearchTermsAreAnded(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.CreateBatch(ctx, []*models.Note{
		newNote("evomem", "sqlite wal mode"),
		newNote("evomem", "sqlite fts5 index"),
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "sqlite wal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("got %d hits, want 1", len(hits))
	}
}

func TestSearchFilters(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	a := newNote("evomem", "migration failed")
	b := newNote("pendra", "migration failed")
	c := newNote("evomem", "migration failed")
	c.SourceType = models.SourceTelegram
	if err := db.CreateBatch(ctx, []*models.Note{a, b, c}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		q    SearchQuery
		want int
	}{
		{"everything", SearchQuery{Text: "migration"}, 3},
		{"one project", SearchQuery{Text: "migration", ProjectID: "evomem"}, 2},
		{"one source", SearchQuery{Text: "migration", SourceType: models.SourceTelegram}, 1},
		{"both", SearchQuery{Text: "migration", ProjectID: "pendra", SourceType: models.SourceTelegram}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := db.Search(ctx, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if len(hits) != tc.want {
				t.Errorf("got %d hits, want %d", len(hits), tc.want)
			}
		})
	}
}

// The index is an external content table, which SQLite does not keep in step
// on its own. These three tests are what prove the triggers are there.
func TestSearchSeesANewNote(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "kelime")); err != nil {
		t.Fatal(err)
	}
	hits, err := db.Search(ctx, SearchQuery{Text: "kelime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("got %d hits, want 1", len(hits))
	}
}

func TestSearchFollowsAnUpdate(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "eskikelime")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	n.Content = "yenikelime"
	if err := db.Update(ctx, n); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "yenikelime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("the new text is not searchable: %d hits", len(hits))
	}

	hits, err = db.Search(ctx, SearchQuery{Text: "eskikelime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("the old text is still in the index: %d hits", len(hits))
	}
}

func TestSearchForgetsADeletedNote(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	n := newNote("evomem", "silinenkelime")
	if err := db.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, n.ID); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "silinenkelime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("a deleted note is still searchable: %d hits", len(hits))
	}
}

// The text in a search box is not an FTS5 expression. Every one of these is
// valid in that query language and would be a syntax error, a wrong answer, or
// a column filter if it were passed through.
func TestSearchSurvivesFTSSyntax(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "the deploy broke at midnight")); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{
		`"`,
		`deploy"`,
		`deploy OR`,
		`AND`,
		`NOT deploy`,
		`NEAR(deploy broke`,
		`(deploy`,
		`deploy)`,
		`content:deploy`,
		`deploy*`,
		`-deploy`,
		`deploy^2`,
		`it's`,
		`***`,
		`düğüm's "quoted" (thing)`,
	} {
		if _, err := db.Search(ctx, SearchQuery{Text: text}); err != nil {
			t.Errorf("Search(%q) failed: %v", text, err)
		}
	}
}

// A search for punctuation alone has nothing to look for. That is an empty
// result, not an error, and must not reach SQLite as an empty MATCH.
func TestSearchEmptyQuery(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "bir sey")); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"", "   ", "!!!", `"" ()`} {
		hits, err := db.Search(ctx, SearchQuery{Text: text})
		if err != nil {
			t.Errorf("Search(%q) failed: %v", text, err)
		}
		if len(hits) != 0 {
			t.Errorf("Search(%q) returned %d hits", text, len(hits))
		}
	}
}

func TestSearchPrefix(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "synchronisation pipeline")); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "synchro", Prefix: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("a prefix search found %d hits, want 1", len(hits))
	}

	hits, err = db.Search(ctx, SearchQuery{Text: "synchro"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("a whole-word search matched a prefix: %d hits", len(hits))
	}
}

// The tokenizer folds a diacritic onto its base letter, so a Turkish note is
// findable from a keyboard that is not set up for one.
func TestSearchFoldsDiacritics(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "düğüm çözülemedi")); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"düğüm", "dugum", "DUGUM", "çözülemedi", "cozulemedi"} {
		hits, err := db.Search(ctx, SearchQuery{Text: text})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 {
			t.Errorf("Search(%q) found %d hits, want 1", text, len(hits))
		}
	}
}

// The one place the two backends genuinely disagree, and it is measured
// here rather than left for somebody to report as a bug.
//
// On SQLite, ı is its own letter in Unicode — not an i carrying a mark — so
// unicode61 decomposes nothing and "veritabani" does not find "veritabanı"
// (ADR-0003). On PostgreSQL, unaccent's rules do map ı to i, so the same
// search finds it. That is the better answer for Turkish and it costs
// nothing, so it is kept rather than crippled to match; ADR-0028 records
// the difference.
func TestSearchFoldsDotlessIOnlyOnPostgres(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "veritabanı kilitlendi")); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "veritabanı"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("the word as written found %d hits, want 1", len(hits))
	}

	hits, err = db.Search(ctx, SearchQuery{Text: "veritabani"})
	if err != nil {
		t.Fatal(err)
	}
	if onPostgres() {
		if len(hits) != 1 {
			t.Errorf("i did not match ı; unaccent folds it, so the configuration has changed")
		}
		return
	}
	if len(hits) != 0 {
		t.Errorf("i matched ı, which unicode61 does not do; the fold has changed")
	}
}

// A better match comes first, and the score a caller compares is the right way
// round.
func TestSearchRanksAndScores(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.CreateBatch(ctx, []*models.Note{
		newNote("evomem", "a passing mention of locking"),
		newNote("evomem", "locking locking locking, all about locking"),
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "locking"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if !strings.HasPrefix(hits[0].Note.Content, "locking locking") {
		t.Errorf("the weaker match came first: %q", hits[0].Note.Content)
	}
	if hits[0].Score < hits[1].Score {
		t.Errorf("scores are %v and %v; a better hit must score higher",
			hits[0].Score, hits[1].Score)
	}
}

func TestSearchPaging(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	var notes []*models.Note
	for i := 0; i < 10; i++ {
		notes = append(notes, newNote("evomem", "ortak kelime ve ayrica farkli"))
	}
	if err := db.CreateBatch(ctx, notes); err != nil {
		t.Fatal(err)
	}

	first, err := db.Search(ctx, SearchQuery{Text: "ortak", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.Search(ctx, SearchQuery{Text: "ortak", Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("pages are %d and %d long", len(first), len(second))
	}
	for _, a := range first {
		for _, b := range second {
			if a.Note.ID == b.Note.ID {
				t.Errorf("%s is on both pages", a.Note.ID)
			}
		}
	}
}

// The index is derived, so it can always be thrown away and rebuilt. This is
// the escape hatch if it ever disagrees with the table.
func TestRebuildIndex(t *testing.T) {
	skipOnPostgres(t, "there is no index to rebuild: the GIN index is over an expression on notes.content, so it cannot fall out of step the way an FTS5 external-content table can")
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Create(ctx, newNote("evomem", "yeniden kurulacak")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.write.ExecContext(ctx, `DELETE FROM notes_fts`); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "kurulacak"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("after a rebuild the search found %d hits, want 1", len(hits))
	}
}

// What a person typed becomes terms, in both query languages. The point is
// that nothing a chat message can contain reaches either one as syntax: an
// unbalanced quote, a NEAR, a colon, an apostrophe are all just characters
// to drop.
func TestSearchTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"gateway", []string{"gateway"}},
		{"two words", []string{"two", "words"}},
		{`it's a "trap"`, []string{"it", "s", "a", "trap"}},
		{"NEAR(a b)", []string{"NEAR", "a", "b"}},
		{"", nil},
		{"!!! ???", nil},
		{"EVO-12", []string{"EVO", "12"}},
		{"düğüm", []string{"düğüm"}},
		// A colon is to_tsquery's prefix marker and would be syntax
		// if it got through.
		{"a:*", []string{"a"}},
	}
	for _, c := range cases {
		got := searchTerms(c.in)
		if len(got) != len(c.want) {
			t.Errorf("searchTerms(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("searchTerms(%q) = %q, want %q", c.in, got, c.want)
				break
			}
		}
	}
}

// The expressions each builder makes out of those terms. Checked as strings
// because the difference between the two query languages is the thing this
// step is about, and a shape that drifts is a query that stops using its
// index or stops meaning AND.
func TestTheTwoQueryLanguages(t *testing.T) {
	terms := []string{"iki", "kelime"}

	sqlite, _ := sqliteSearch(terms, SearchQuery{})
	if !strings.Contains(sqlite, "notes_fts MATCH ?") {
		t.Errorf("the FTS5 query lost its MATCH:\n%s", sqlite)
	}

	postgres, args := postgresSearch(terms, SearchQuery{})
	if !strings.Contains(postgres, "to_tsquery('evomem', ?)") {
		t.Errorf("the PostgreSQL query lost its tsquery:\n%s", postgres)
	}
	// The index is over to_tsvector('evomem', content) and PostgreSQL
	// only uses an index when the expression matches exactly.
	if !strings.Contains(postgres, "to_tsvector('evomem', n.content) @@") {
		t.Errorf("the PostgreSQL query would not use its index:\n%s", postgres)
	}
	if args[1] != "iki & kelime" {
		t.Errorf("terms are not ANDed: %q", args[1])
	}

	withPrefix, prefixArgs := postgresSearch(terms, SearchQuery{Prefix: true})
	if prefixArgs[1] != "iki & kelime:*" {
		t.Errorf("prefix marks the wrong thing: %q", prefixArgs[1])
	}
	_ = withPrefix
}
