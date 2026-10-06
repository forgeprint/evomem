package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/forgeprint/evomem/core/sync"
)

// cmdSync pushes local changes to the remote copy.
//
// The connection string comes from the environment, not a flag: it carries a
// password, and a flag is visible in ps to every process on the machine.
func cmdSync(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	once := fs.Bool("once", false, "make one pass and stop")
	initRemote := fs.Bool("init-remote", false, "create the remote schema, then stop")
	interval := fs.Duration("interval", sync.DefaultInterval, "wait between passes")
	batch := fs.Int("batch", sync.DefaultBatchSize, "notes or deletions per push")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dsn := os.Getenv("EVOMEM_POSTGRES_DSN")
	if dsn == "" {
		return errors.New("EVOMEM_POSTGRES_DSN is not set; it carries a password, so it is not a flag")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	remote, err := sync.OpenPostgres(ctx, dsn)
	if err != nil {
		return err
	}
	defer remote.Close()

	// Explicit, because running DDL against someone's database because a
	// process started is not a sync client's decision to make.
	if *initRemote {
		if err := remote.EnsureSchema(ctx); err != nil {
			return err
		}
		fmt.Fprintln(out, "the remote schema is ready")
		return nil
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	worker, err := sync.New(db, remote, sync.Config{Interval: *interval, BatchSize: *batch})
	if err != nil {
		return err
	}

	if *once {
		stats, err := worker.RunOnce(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, describePass(stats))
		return nil
	}

	// A daemon has nowhere to print to but its log, and a pass that did
	// nothing is not worth a line: a laptop that is offline all day would
	// otherwise fill the log with it.
	worker.Observe(func(stats sync.Stats, err error) {
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "evomem sync: %v\n", err)
		case !stats.Empty():
			fmt.Fprintf(os.Stderr, "evomem sync: %s\n", describePass(stats))
		}
	})

	fmt.Fprintf(out, "evomem sync: every %s, store %s\n", *interval, db.Path())
	err = worker.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// describePass is one line a person can read.
func describePass(stats sync.Stats) string {
	if stats.Offline {
		return "the remote is unreachable; nothing was sent"
	}
	if stats.Empty() {
		return "nothing to send"
	}
	return fmt.Sprintf("%d note(s) and %d deletion(s) in %d batch(es), %s",
		stats.NotesPushed, stats.DeletionsPushed, stats.Batches,
		stats.Duration.Round(time.Millisecond))
}
