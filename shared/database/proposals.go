package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// What a proposal can be.
const (
	ProposalPending  = "pending"
	ProposalAccepted = "accepted"
	ProposalRejected = "rejected"
)

// maxPendingProposals is how many undecided proposals a store will hold.
//
// An agent in a loop can propose without limit, and a review queue with ten
// thousand things in it is not a review queue. Refusing with a clear message
// is better than letting it fill: the agent is told to stop, and the person
// is not handed a pile.
const maxPendingProposals = 200

// ErrTooManyProposals is returned when the review queue is full.
var ErrTooManyProposals = errors.New("database: too many proposals are waiting for review")

// A Proposal is a note an agent suggested. It is not memory: nothing reads it
// but the review command, and the only way into notes is Accept.
type Proposal struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Content    string            `json:"content"`
	SourceType models.SourceType `json:"source_type"`
	Metadata   map[string]any    `json:"metadata,omitempty"`

	// Reason is what the agent said about why this is worth keeping. It
	// is for the person deciding, and is not carried into the note.
	Reason string `json:"reason,omitempty"`

	// ProposedBy is the client that asked, as it named itself. Taken from
	// the MCP client's own identity, so it is a label rather than a
	// credential.
	ProposedBy string `json:"proposed_by,omitempty"`

	ProposedAt time.Time  `json:"proposed_at"`
	Status     string     `json:"status"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`

	// NoteID is the note this became, once accepted.
	NoteID string `json:"note_id,omitempty"`
}

// Tainted reports whether the agent declared that this came from outside the
// project.
//
// Self-declared, which is as much as a store can do: an agent that read a web
// page is the only party that knows it did. The review step is what the
// property actually rests on.
func (p *Proposal) Tainted() bool {
	v, ok := p.Metadata[models.MetaTainted]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// Propose records a suggestion for review.
//
// An identical pending proposal in the same project is returned rather than
// duplicated. An agent that retries a failed call, or raises the same thing
// twice in one conversation, should not make the queue longer.
func (d *DB) Propose(ctx context.Context, p *Proposal) error {
	if p == nil {
		return errors.New("database: no proposal given")
	}
	if strings.TrimSpace(p.ProjectID) == "" {
		return models.ErrEmptyProjectID
	}
	if strings.TrimSpace(p.Content) == "" {
		return models.ErrEmptyContent
	}
	if strings.TrimSpace(string(p.SourceType)) == "" {
		p.SourceType = models.SourceMCP
	}

	// Same content, same project, still undecided: hand back the one that
	// is already waiting.
	var existing string
	err := d.read.QueryRowContext(ctx,
		`SELECT id FROM proposals
		 WHERE status = ? AND project_id = ? AND content = ?`,
		ProposalPending, p.ProjectID, p.Content).Scan(&existing)
	switch {
	case err == nil:
		p.ID = existing
		return d.loadProposal(ctx, p)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("database: looking for an existing proposal: %w", err)
	}

	var pending int
	if err := d.read.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM proposals WHERE status = ?`, ProposalPending).Scan(&pending); err != nil {
		return fmt.Errorf("database: counting proposals: %w", err)
	}
	if pending >= maxPendingProposals {
		return fmt.Errorf("%w: %d are pending and the limit is %d; review them with evomem review",
			ErrTooManyProposals, pending, maxPendingProposals)
	}

	if p.ID == "" {
		p.ID = models.NewULID()
	}
	if p.ProposedAt.IsZero() {
		p.ProposedAt = time.Now().UTC()
	}
	p.Status = ProposalPending

	metadata := "{}"
	if len(p.Metadata) > 0 {
		note := models.Note{Metadata: p.Metadata}
		var err error
		if metadata, err = note.MarshalMetadata(); err != nil {
			return err
		}
	}

	_, err = d.write.ExecContext(ctx, `
		INSERT INTO proposals
			(id, project_id, content, source_type, metadata, reason, proposed_by, proposed_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.Content, string(p.SourceType), metadata,
		p.Reason, p.ProposedBy, formatTime(p.ProposedAt), p.Status)
	if err != nil {
		return fmt.Errorf("database: recording proposal %s: %w", p.ID, err)
	}
	return nil
}

const proposalColumns = `id, project_id, content, source_type, metadata, reason,
	proposed_by, proposed_at, status, decided_at, note_id`

func scanProposal(s interface{ Scan(...any) error }) (*Proposal, error) {
	var (
		p          Proposal
		sourceType string
		metadata   string
		proposedAt string
		decidedAt  sql.NullString
	)
	if err := s.Scan(&p.ID, &p.ProjectID, &p.Content, &sourceType, &metadata, &p.Reason,
		&p.ProposedBy, &proposedAt, &p.Status, &decidedAt, &p.NoteID); err != nil {
		return nil, err
	}

	p.SourceType = models.SourceType(sourceType)

	var note models.Note
	if err := note.UnmarshalMetadata(metadata); err != nil {
		return nil, err
	}
	p.Metadata = note.Metadata

	var err error
	if p.ProposedAt, err = parseTime(proposedAt); err != nil {
		return nil, err
	}
	if decidedAt.Valid && decidedAt.String != "" {
		decided, err := parseTime(decidedAt.String)
		if err != nil {
			return nil, err
		}
		p.DecidedAt = &decided
	}
	return &p, nil
}

// loadProposal fills p from the row its ID names.
func (d *DB) loadProposal(ctx context.Context, p *Proposal) error {
	got, err := d.GetProposal(ctx, p.ID)
	if err != nil {
		return err
	}
	*p = *got
	return nil
}

// GetProposal returns one proposal.
func (d *DB) GetProposal(ctx context.Context, id string) (*Proposal, error) {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return nil, err
	}

	row := d.read.QueryRowContext(ctx,
		`SELECT `+proposalColumns+` FROM proposals WHERE id = ?`, id)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: proposal %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("database: reading proposal %s: %w", id, err)
	}
	return p, nil
}

// ProposalOptions bounds a listing. An empty Status means pending only, which
// is what a review is.
type ProposalOptions struct {
	Status    string
	ProjectID string
	Limit     int
}

// Proposals lists proposals, newest first.
func (d *DB) Proposals(ctx context.Context, opts ProposalOptions) ([]*Proposal, error) {
	status := opts.Status
	if status == "" {
		status = ProposalPending
	}

	query := `SELECT ` + proposalColumns + ` FROM proposals`
	var (
		where []string
		args  []any
	)
	if status != "all" {
		where = append(where, `status = ?`)
		args = append(args, status)
	}
	if opts.ProjectID != "" {
		where = append(where, `project_id = ?`)
		args = append(args, opts.ProjectID)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY proposed_at DESC, id DESC LIMIT ?`
	args = append(args, clampLimit(opts.Limit))

	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: listing proposals: %w", err)
	}
	defer rows.Close()

	out := []*Proposal{}
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, fmt.Errorf("database: listing proposals: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: listing proposals: %w", err)
	}
	return out, nil
}

// PendingProposals is how many are waiting, for a status line.
func (d *DB) PendingProposals(ctx context.Context) (int, error) {
	var n int
	if err := d.read.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM proposals WHERE status = ?`, ProposalPending).Scan(&n); err != nil {
		return 0, fmt.Errorf("database: counting proposals: %w", err)
	}
	return n, nil
}

// AcceptProposal turns a proposal into a note. It is the only path from a
// proposal into memory, and a person is the only caller.
//
// The note is not marked tainted even when the proposal was, because a person
// read it and said yes: that is the endorsement the mark exists to be absent
// for. What the proposal claimed is kept in the note's metadata instead —
// proposed_by, and proposed_tainted when the agent declared it — so the
// provenance survives the decision.
func (d *DB) AcceptProposal(ctx context.Context, id string) (*models.Note, error) {
	p, err := d.GetProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status != ProposalPending {
		return nil, fmt.Errorf("database: proposal %s was already %s", p.ID, p.Status)
	}

	note := &models.Note{
		ProjectID:  p.ProjectID,
		Content:    p.Content,
		SourceType: p.SourceType,
		Metadata:   map[string]any{},
	}
	for k, v := range p.Metadata {
		note.Metadata[k] = v
	}
	// The accepted note is the person's, so the agent's self-declaration
	// moves to a name that cannot be mistaken for the live mark.
	delete(note.Metadata, models.MetaTainted)
	if p.Tainted() {
		note.SetMeta("proposed_tainted", true)
	}
	note.SetMeta("proposed_by", p.ProposedBy)
	note.SetMeta("proposal_id", p.ID)

	if err := d.Create(ctx, note); err != nil {
		return nil, err
	}

	if _, err := d.write.ExecContext(ctx,
		`UPDATE proposals SET status = ?, decided_at = ?, note_id = ? WHERE id = ?`,
		ProposalAccepted, formatTime(time.Now().UTC()), note.ID, p.ID); err != nil {
		// The note exists and the proposal still says pending. Accepting
		// again would make a second note, so the caller has to see this.
		return note, fmt.Errorf("database: note %s was created but proposal %s was not closed: %w",
			note.ID, p.ID, err)
	}
	return note, nil
}

// RejectProposal turns one down. The row is kept so the same thing is not
// proposed again tomorrow, and so there is a record of what was refused.
func (d *DB) RejectProposal(ctx context.Context, id string) error {
	p, err := d.GetProposal(ctx, id)
	if err != nil {
		return err
	}
	if p.Status != ProposalPending {
		return fmt.Errorf("database: proposal %s was already %s", p.ID, p.Status)
	}

	if _, err := d.write.ExecContext(ctx,
		`UPDATE proposals SET status = ?, decided_at = ? WHERE id = ?`,
		ProposalRejected, formatTime(time.Now().UTC()), p.ID); err != nil {
		return fmt.Errorf("database: rejecting proposal %s: %w", p.ID, err)
	}
	return nil
}
