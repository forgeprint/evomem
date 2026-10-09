package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/forgeprint/evomem/core/organize"
)

// cmdOrganize asks a connected model to group the notes nothing has grouped.
//
// The second path to a cluster, beside the agent over MCP (ADR-0023). With no
// model connected the feature is off and this says so without touching a note
// (ADR-0027).
func cmdOrganize(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("organize", flag.ContinueOnError)
	project := fs.String("project", "default", "the project whose notes to group")
	dryRun := fs.Bool("dry-run", false, "ask the model and print what it would write, writing nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	connections, err := db.Connections(ctx)
	if err != nil {
		return err
	}
	key, err := secretKey()
	if err != nil {
		return fmt.Errorf("%w\n\nThe model's key is sealed with it; without it nothing can be grouped", err)
	}

	model, err := organize.FromConnection(connections, key)
	if errors.Is(err, organize.ErrNotConfigured) {
		fmt.Fprintln(out, "No model is connected, so nothing here calls one and no note leaves this machine.")
		fmt.Fprintln(out, "\nevomem connect -add model -project <id> \\")
		fmt.Fprintln(out, "  -url https://api.openai.com -query gpt-4o-mini")
		fmt.Fprintln(out, "\nAn agent connected over MCP can group notes without any of this.")
		return nil
	}
	if err != nil {
		return err
	}

	waiting, err := db.Ungrouped(ctx, *project, organize.MaxNotes)
	if err != nil {
		return err
	}
	if len(waiting) == 0 {
		fmt.Fprintf(out, "every note in %s is already in a group\n", *project)
		return nil
	}
	if *dryRun {
		fmt.Fprintf(out, "asking about %d note(s); nothing will be written\n\n", len(waiting))
	}

	result, err := organize.Run(ctx, db, model, *project, *dryRun, out)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\n%d note(s) offered, %d group(s), %d note(s) grouped\n",
		result.Offered, result.Created, result.Grouped)
	if result.Invented > 0 {
		fmt.Fprintf(out, "%d id(s) the model named were not sent to it, and were dropped\n", result.Invented)
	}
	if result.Dropped > 0 {
		fmt.Fprintf(out, "%d group(s) had nothing usable left in them\n", result.Dropped)
	}
	if result.Offered == organize.MaxNotes {
		fmt.Fprintf(out, "\nA run offers at most %d notes; press again for the rest.\n", organize.MaxNotes)
	}
	if !*dryRun && result.Created > 0 {
		fmt.Fprintln(out, "evomem clusters   shows what it made; -delete undoes one")
	}
	return nil
}
