package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// cmdEndorse is how a person says they have read a note that came from
// outside and stand behind it.
//
// A command rather than an MCP tool: every tool is read-only (ADR-0008), and
// an agent endorsing text it read itself would be both the proposer and the
// endorser, which empties the rule that a person decides (ADR-0013). See
// ADR-0021.
func cmdEndorse(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("endorse", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "say what would change, change nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("endorse takes one note id")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	note, err := db.Get(ctx, fs.Arg(0))
	switch {
	case errors.Is(err, models.ErrInvalidULID):
		return fmt.Errorf("%q is not a note id", fs.Arg(0))
	case errors.Is(err, database.ErrNotFound):
		return fmt.Errorf("there is no note %s", fs.Arg(0))
	case err != nil:
		return err
	}

	origin, _ := note.MetaString(models.MetaOrigin)
	if origin == "" {
		origin = "outside this project"
	}

	// The content, before the decision. Endorsing without reading is the one
	// thing this command cannot prevent; showing the words is what it can do.
	fmt.Fprintf(out, "%s  %s  %s\n", note.ID, note.ProjectID, note.SourceType)
	fmt.Fprintf(out, "  came from %s\n", origin)
	if note.Transcribed() {
		fmt.Fprintln(out, "  machine transcription of audio; this mark stays")
	}
	fmt.Fprintf(out, "\n%s\n\n", note.Content)

	if !note.Endorse(time.Now()) {
		// Not an error: the person asked for a state the note is already in.
		fmt.Fprintln(out, "nothing to endorse: this note is not marked as coming from outside")
		return nil
	}
	if *dryRun {
		fmt.Fprintln(out, "would clear the untrusted mark; nothing was written")
		return nil
	}
	if err := db.Update(ctx, note); err != nil {
		return err
	}

	fmt.Fprintln(out, "endorsed: the untrusted mark is gone, and the note records that it was there")
	if note.Transcribed() {
		fmt.Fprintln(out, "It is still marked as a machine transcription, which a reading does not change.")
	}
	fmt.Fprintln(out, "There is no way to undo this. To take it back, delete the note.")
	return nil
}
