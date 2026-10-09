package database

import (
	"context"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

func TestSearchFindsAWord(t *testing.T) {
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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

// What the fold does not reach: ı is its own letter in Unicode, not an i
// carrying a mark, so nothing decomposes and "veritabani" does not find
// "veritabanı". Fixing it means a Turkish-aware tokenizer, which unicode61 is
// not; this test is here so the limit is known rather than discovered by a
// user. See docs/adr/0003-turkish-text-search.md.
func TestSearchDoesNotFoldDotlessI(t *testing.T) {
	skipUntilPostgresSearch(t)
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
	if len(hits) != 0 {
		t.Errorf("i matched ı, which unicode61 does not do; the fold has changed")
	}
}

// A better match comes first, and the score a caller compares is the right way
// round.
func TestSearchRanksAndScores(t *testing.T) {
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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
	skipUntilPostgresSearch(t)
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

func TestFTSExpression(t *testing.T) {
	skipUntilPostgresSearch(t)
	cases := []struct {
		in     string
		prefix bool
		want   string
	}{
		{"gateway", false, `"gateway"`},
		{"two words", false, `"two" "words"`},
		{"two words", true, `"two" "words"*`},
		{`it's a "trap"`, false, `"it" "s" "a" "trap"`},
		{"NEAR(a b)", false, `"NEAR" "a" "b"`},
		{"", false, ""},
		{"!!! ???", false, ""},
		{"EVO-12", false, `"EVO" "12"`},
		{"düğüm", false, `"düğüm"`},
	}
	for _, c := range cases {
		if got := ftsExpression(c.in, c.prefix); got != c.want {
			t.Errorf("ftsExpression(%q, %v) = %q, want %q", c.in, c.prefix, got, c.want)
		}
	}
}
