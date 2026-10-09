package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

func newProposal(project, content string) *Proposal {
	return &Proposal{ProjectID: project, Content: content, ProposedBy: "test-agent 1.0"}
}

func TestProposeFillsInTheBlanks(t *testing.T) {
	db := openTemp(t)
	p := newProposal("evomem", "WAL modu olmadan eşzamanlı yazma kilitleniyor")

	if err := db.Propose(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !models.ValidULID(p.ID) {
		t.Errorf("id is %q", p.ID)
	}
	if p.Status != ProposalPending {
		t.Errorf("status is %q, want pending", p.Status)
	}
	if p.ProposedAt.IsZero() {
		t.Error("proposed_at was not set")
	}
	// An agent proposing through MCP is the default source.
	if p.SourceType != models.SourceMCP {
		t.Errorf("source is %q, want mcp", p.SourceType)
	}
}

// The whole point: a proposal is not memory. Nothing that reads notes may
// see it.
func TestAProposalIsNotANote(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	p := newProposal("evomem", "onerilmiskelime ve baska bir sey")
	if err := db.Propose(ctx, p); err != nil {
		t.Fatal(err)
	}

	hits, err := db.Search(ctx, SearchQuery{Text: "onerilmiskelime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("a proposal is searchable: %d hits", len(hits))
	}

	notes, err := db.List(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("a proposal is listed as a note: %d notes", len(notes))
	}

	if _, err := db.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a proposal is readable as a note: %v", err)
	}

	// And it does not get pushed to the mirror either.
	pending, _, err := db.PendingNotes(ctx, Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("a proposal is pending for sync: %d", len(pending))
	}
}

func TestProposeValidates(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.Propose(ctx, nil); err == nil {
		t.Error("a nil proposal was accepted")
	}
	if err := db.Propose(ctx, newProposal("", "icerik")); !errors.Is(err, models.ErrEmptyProjectID) {
		t.Errorf("got %v, want ErrEmptyProjectID", err)
	}
	if err := db.Propose(ctx, newProposal("evomem", "  ")); !errors.Is(err, models.ErrEmptyContent) {
		t.Errorf("got %v, want ErrEmptyContent", err)
	}
}

// An agent that retries a failed call, or raises the same thing twice in one
// conversation, should not make the queue longer.
func TestProposeIsIdempotentWhilePending(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	first := newProposal("evomem", "ayni icerik")
	if err := db.Propose(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := newProposal("evomem", "ayni icerik")
	if err := db.Propose(ctx, second); err != nil {
		t.Fatal(err)
	}

	if second.ID != first.ID {
		t.Errorf("a duplicate got its own id: %s and %s", first.ID, second.ID)
	}
	pending, err := db.PendingProposals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Errorf("%d proposals are pending, want 1", pending)
	}

	// Another project is another proposal.
	other := newProposal("pendra", "ayni icerik")
	if err := db.Propose(ctx, other); err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID {
		t.Error("the same content in another project was folded into one proposal")
	}
}

// Once decided, the same content may be proposed again: circumstances change,
// and a rejection is not a permanent ban.
func TestProposeAgainAfterADecision(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	first := newProposal("evomem", "tekrar onerilecek")
	if err := db.Propose(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := db.RejectProposal(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	second := newProposal("evomem", "tekrar onerilecek")
	if err := db.Propose(ctx, second); err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Error("a rejected proposal was reused instead of a new one being made")
	}
}

// An agent in a loop can propose without limit, and a review queue with ten
// thousand things in it is not a review queue.
func TestProposalQueueIsBounded(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	for i := 0; i < maxPendingProposals; i++ {
		if err := db.Propose(ctx, newProposal("evomem", fmt.Sprintf("oneri %d", i))); err != nil {
			t.Fatalf("proposal %d failed: %v", i, err)
		}
	}

	err := db.Propose(ctx, newProposal("evomem", "bir fazla"))
	if !errors.Is(err, ErrTooManyProposals) {
		t.Fatalf("got %v, want ErrTooManyProposals", err)
	}
	// The message has to tell the agent what to do instead.
	if err != nil && !strings.Contains(err.Error(), "evomem review") {
		t.Errorf("the error does not point at the review command: %v", err)
	}

	// Deciding one makes room again.
	proposals, err := db.Proposals(ctx, ProposalOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RejectProposal(ctx, proposals[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Propose(ctx, newProposal("evomem", "bir fazla")); err != nil {
		t.Errorf("the queue did not free up after a decision: %v", err)
	}
}

func TestAcceptProposal(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	p := newProposal("evomem", "kabul edilecek bulgu")
	p.Reason = "bir sonraki oturum bunu tekrar aramasin"
	if err := db.Propose(ctx, p); err != nil {
		t.Fatal(err)
	}

	note, err := db.AcceptProposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if note.ProjectID != "evomem" || note.Content != "kabul edilecek bulgu" {
		t.Errorf("the note is %+v", note)
	}
	// Now it is memory: searchable, listed, and pending for the mirror.
	hits, err := db.Search(ctx, SearchQuery{Text: "bulgu"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Errorf("the accepted note is not searchable: %d hits", len(hits))
	}

	// The provenance survives the decision.
	if got, _ := note.MetaString("proposed_by"); got != "test-agent 1.0" {
		t.Errorf("proposed_by is %q", got)
	}
	if got, _ := note.MetaString("proposal_id"); got != p.ID {
		t.Errorf("proposal_id is %q, want %s", got, p.ID)
	}
	// The reason was for the person deciding, not for the note.
	if _, ok := note.Metadata["reason"]; ok {
		t.Error("the reason was carried into the note")
	}

	// And the proposal is closed, pointing at what it became.
	closed, err := db.GetProposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != ProposalAccepted {
		t.Errorf("status is %q, want accepted", closed.Status)
	}
	if closed.NoteID != note.ID {
		t.Errorf("note_id is %q, want %s", closed.NoteID, note.ID)
	}
	if closed.DecidedAt == nil {
		t.Error("decided_at was not set")
	}
}

// A person read it and said yes. That is the endorsement the tainted mark
// exists to be absent for — but what the agent claimed is kept, under a name
// that cannot be mistaken for the live mark.
func TestAcceptedNoteIsNotTaintedButRemembersTheClaim(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	p := newProposal("evomem", "bir web sayfasindan geldi")
	p.Metadata = map[string]any{models.MetaTainted: true}
	if err := db.Propose(ctx, p); err != nil {
		t.Fatal(err)
	}
	if !p.Tainted() {
		t.Fatal("the proposal does not report itself as tainted")
	}

	note, err := db.AcceptProposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if note.Tainted() {
		t.Error("a note a person accepted is still marked tainted")
	}
	if note.Metadata["proposed_tainted"] != true {
		t.Errorf("the claim was lost: %v", note.Metadata)
	}
}

func TestRejectProposal(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	p := newProposal("evomem", "reddedilecek")
	if err := db.Propose(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := db.RejectProposal(ctx, p.ID); err != nil {
		t.Fatal(err)
	}

	// Nothing became a note.
	notes, err := db.List(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("a rejected proposal became a note: %d", len(notes))
	}
	// But the row is kept, so there is a record of what was refused.
	closed, err := db.GetProposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != ProposalRejected {
		t.Errorf("status is %q, want rejected", closed.Status)
	}
}

// Deciding twice would make a second note from one proposal.
func TestDecidingTwice(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	accepted := newProposal("evomem", "bir kere kabul")
	if err := db.Propose(ctx, accepted); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AcceptProposal(ctx, accepted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AcceptProposal(ctx, accepted.ID); err == nil {
		t.Error("the same proposal was accepted twice")
	}
	if err := db.RejectProposal(ctx, accepted.ID); err == nil {
		t.Error("an accepted proposal was then rejected")
	}

	count, err := db.Count(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("%d notes came from one proposal", count)
	}
}

func TestProposalNotFound(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if _, err := db.GetProposal(ctx, models.NewULID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
	if _, err := db.GetProposal(ctx, "nonsense"); !errors.Is(err, models.ErrInvalidULID) {
		t.Errorf("got %v, want ErrInvalidULID", err)
	}
	if _, err := db.AcceptProposal(ctx, models.NewULID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
	if err := db.RejectProposal(ctx, models.NewULID()); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestProposalsListing(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	pending := newProposal("evomem", "bekleyen")
	accepted := newProposal("evomem", "kabul edilen")
	rejected := newProposal("pendra", "reddedilen")
	for _, p := range []*Proposal{pending, accepted, rejected} {
		if err := db.Propose(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.AcceptProposal(ctx, accepted.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RejectProposal(ctx, rejected.ID); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		opts ProposalOptions
		want int
	}{
		{"pending by default", ProposalOptions{}, 1},
		{"accepted", ProposalOptions{Status: ProposalAccepted}, 1},
		{"rejected", ProposalOptions{Status: ProposalRejected}, 1},
		{"all", ProposalOptions{Status: "all"}, 3},
		{"one project", ProposalOptions{Status: "all", ProjectID: "pendra"}, 1},
	}
	for _, c := range cases {
		got, err := db.Proposals(ctx, c.opts)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d, want %d", c.name, len(got), c.want)
		}
	}
}

// Archiving clears decided proposals, because the table would otherwise only
// grow. A pending one is never touched: nobody has looked at it yet.
func TestArchiveClearsDecidedProposalsOnly(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	old := newProposal("evomem", "eski ve karara baglanmis")
	if err := db.Propose(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := db.RejectProposal(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	// Back-date the decision so it is older than the cutoff.
	if _, err := db.write.ExecContext(ctx, `UPDATE proposals SET decided_at = ? WHERE id = ?`,
		formatTime(time.Now().AddDate(0, -8, 0)), old.ID); err != nil {
		t.Fatal(err)
	}

	waiting := newProposal("evomem", "eski ama hala bekliyor")
	if err := db.Propose(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	if _, err := db.write.ExecContext(ctx, `UPDATE proposals SET proposed_at = ? WHERE id = ?`,
		formatTime(time.Now().AddDate(0, -8, 0)), waiting.ID); err != nil {
		t.Fatal(err)
	}

	result, err := db.Archive(ctx, ArchiveOptions{Before: time.Now().AddDate(0, -6, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProposalsRemoved != 1 {
		t.Errorf("%d proposals removed, want 1", result.ProposalsRemoved)
	}
	if _, err := db.GetProposal(ctx, waiting.ID); err != nil {
		t.Errorf("a pending proposal was archived: %v", err)
	}
}
