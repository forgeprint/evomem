package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/forgeprint/evomem/shared/database"
)

// cmdArchive removes old notes to thin the local file.
func cmdArchive(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	months := fs.Int("months", 6, "remove notes older than this many months")
	before := fs.String("before", "", "remove notes created before this instant (RFC 3339), instead of -months")
	project := fs.String("project", "", "only this project")
	vacuum := fs.Bool("vacuum", false, "rewrite the file afterwards to return the space")
	includeUnsynced := fs.Bool("include-unsynced", false,
		"also remove notes that have not been synced; this loses them permanently")
	dryRun := fs.Bool("dry-run", false, "say what would go, and remove nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, -*months, 0)
	if *before != "" {
		parsed, err := time.Parse(time.RFC3339, *before)
		if err != nil {
			return fmt.Errorf("-before is not an RFC 3339 instant: %w", err)
		}
		cutoff = parsed
	}
	if cutoff.After(time.Now()) {
		return errors.New("the cutoff is in the future, which would remove everything")
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	opts := database.ArchiveOptions{
		Before:          cutoff,
		ProjectID:       *project,
		IncludeUnsynced: *includeUnsynced,
		Vacuum:          *vacuum,
	}

	if *dryRun {
		return archiveDryRun(ctx, db, out, opts)
	}

	result, err := db.Archive(ctx, opts)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "cutoff %s\n", cutoff.UTC().Format(time.RFC3339))
	fmt.Fprintf(out, "removed %d note(s), %d tombstone(s)\n",
		result.NotesRemoved, result.TombstonesRemoved)
	if result.Skipped > 0 {
		fmt.Fprintf(out,
			"kept %d note(s) that are old enough but have not been synced;\n"+
				"  they exist only in this file. -include-unsynced removes them anyway.\n",
			result.Skipped)
	}
	if opts.Vacuum {
		fmt.Fprintf(out, "file %s -> %s\n",
			humanBytes(result.BytesBefore), humanBytes(result.BytesAfter))
	} else if result.NotesRemoved > 0 {
		fmt.Fprintln(out, "the space is free inside the file; -vacuum returns it to the disk")
	}
	return nil
}

func archiveDryRun(ctx context.Context, db *database.DB, out io.Writer, opts database.ArchiveOptions) error {
	total, err := db.Count(context.Background(), database.ListOptions{ProjectID: opts.ProjectID})
	if err != nil {
		return err
	}
	notes, deletions, err := db.PendingCount(context.Background())
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "cutoff %s (dry run, nothing removed)\n", opts.Before.UTC().Format(time.RFC3339))
	fmt.Fprintf(out, "%d note(s) in scope, %d unsynced note(s) and %d unsent deletion(s) in the store\n",
		total, notes, deletions)
	if notes > 0 && !opts.IncludeUnsynced {
		fmt.Fprintln(out, "unsynced notes will be kept; run evomem sync-status for detail")
	}
	return nil
}

func cmdSyncStatus(_ []string, out io.Writer) error {
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	notes, deletions, err := db.PendingCount(ctx)
	if err != nil {
		return err
	}
	noteCursor, err := db.Cursor(ctx, database.CursorNotes)
	if err != nil {
		return err
	}
	deleteCursor, err := db.Cursor(ctx, database.CursorDeletions)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "store %s\n", db.Path())
	fmt.Fprintf(out, "notes      %d pending, cursor %s\n", notes, noteCursor)
	fmt.Fprintf(out, "deletions  %d pending, cursor %s\n", deletions, deleteCursor)
	if noteCursor.IsZero() {
		fmt.Fprintln(out, "\nnothing has been synced, so evomem archive will not remove anything")
	}
	if line := reviewReminder(ctx, db); line != "" {
		fmt.Fprintf(out, "\n%s\n", line)
	}
	return nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/1024)
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