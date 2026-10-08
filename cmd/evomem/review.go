package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
)

// cmdReview is the human half of the proposal flow: an agent suggests, a
// person decides, and only a decision here creates a note.
func cmdReview(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	accept := fs.String("accept", "", "accept this proposal and store it as a note")
	reject := fs.String("reject", "", "turn this proposal down")
	project := fs.String("project", "", "only this project")
	status := fs.String("status", database.ProposalPending,
		"pending, accepted, rejected, or all")
	limit := fs.Int("limit", 20, "how many to show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *accept != "" && *reject != "" {
		return errors.New("pass -accept or -reject, not both")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	switch {
	case *accept != "":
		note, err := db.AcceptProposal(ctx, *accept)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "accepted; stored as note %s in %s\n", note.ID, note.ProjectID)
		return nil
	case *reject != "":
		if err := db.RejectProposal(ctx, *reject); err != nil {
			return err
		}
		fmt.Fprintf(out, "rejected %s\n", *reject)
		return nil
	}

	proposals, err := db.Proposals(ctx, database.ProposalOptions{
		Status: *status, ProjectID: *project, Limit: *limit,
	})
	if err != nil {
		return err
	}
	if len(proposals) == 0 {
		fmt.Fprintf(out, "nothing %s\n", *status)
		return nil
	}

	for i, p := range proposals {
		if i > 0 {
			fmt.Fprintln(out)
		}
		writeProposal(out, p)
	}

	if *status == database.ProposalPending {
		fmt.Fprintf(out, "\n%d waiting. Decide on one by id:\n", len(proposals))
		fmt.Fprintf(out, "  evomem review -accept %s\n", proposals[0].ID)
		fmt.Fprintf(out, "  evomem review -reject %s\n", proposals[0].ID)
		fmt.Fprintln(out, "\nThere is no accept-with-edit: a note is the person's words once it")
		fmt.Fprintln(out, "is in memory, so to change the wording, reject it and use evomem add.")
	}
	return nil
}

func writeProposal(out io.Writer, p *database.Proposal) {
	fmt.Fprintf(out, "%s  %s  %s", p.ID, p.ProjectID, p.ProposedAt.Format("2006-01-02 15:04"))
	if p.ProposedBy != "" {
		fmt.Fprintf(out, "  by %s", p.ProposedBy)
	}
	if p.Status != database.ProposalPending {
		fmt.Fprintf(out, "  [%s", p.Status)
		if p.DecidedAt != nil {
			fmt.Fprintf(out, " %s", p.DecidedAt.Format("2006-01-02 15:04"))
		}
		if p.NoteID != "" {
			fmt.Fprintf(out, " -> %s", p.NoteID)
		}
		fmt.Fprint(out, "]")
	}
	fmt.Fprintln(out)

	if p.Tainted() {
		fmt.Fprintln(out, "  [the agent says this came from outside the project]")
	}
	for _, line := range strings.Split(p.Content, "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
	if p.Reason != "" {
		fmt.Fprintf(out, "  why: %s\n", p.Reason)
	}
}

func reviewReminder(ctx context.Context, db *database.DB) string {
	pending, err := db.PendingProposals(ctx)
	if err != nil || len(pending) == 0 {
		return ""
	}
	noun := "proposals"
	if len(pending) == 1 {
		noun = "proposal"
	}
	return fmt.Sprintf("%d %s waiting for review; see evomem review", len(pending), noun)
}