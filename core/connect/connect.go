// Package connect pulls notes out of sources somebody else owns.
//
// The other direction from core/api's webhooks: there a source pushes to us
// on its own schedule, here the hub calls a vendor's API because a person
// asked it to (ADR-0025). Nothing in this package runs on a timer.
package connect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// A Puller reads what a source has and turns it into notes.
//
// The cursor it returns is the source's own idea of where we got to — a page
// token, a timestamp, whatever that API hands back — kept opaque, because a
// cursor this code interprets is one that breaks when the vendor changes it.
type Puller interface {
	// Pull returns notes and the cursor to resume from. An empty cursor
	// means the source has no way to resume and the next run starts over.
	Pull(ctx context.Context, conn *database.Connection, secret string) ([]*models.Note, string, error)
}

// ErrNoPuller is a connection for a source this build cannot call.
var ErrNoPuller = errors.New("connect: nothing here knows how to pull that source")

// Result is what one run did, for the command to print.
type Result struct {
	// Pulled is how many notes arrived and were stored.
	Pulled int

	// Considered is how many connections were tried.
	Considered int

	// Failed is how many connections reported a problem.
	Failed int
}

// Run pulls from every configured connection.
//
// A failure on one source never stops the run: the reason goes on that
// connection and the next one is tried. A run that aborted on the first
// expired token would leave every other source unsynced for as long as
// somebody left that one broken.
func Run(
	ctx context.Context,
	db *database.DB,
	key database.SecretKey,
	pullers map[string]Puller,
	log io.Writer,
) (Result, error) {
	var result Result

	connections, err := db.Connections(ctx)
	if err != nil {
		return result, err
	}
	result.Considered = len(connections)

	for _, conn := range connections {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		if conn.IsModel() {
			// Not a source: a model key, kept in the same keyring
			// (ADR-0027). Skipped by name rather than by falling
			// through to "no puller", so a source type that is
			// genuinely unknown is still reported as one.
			result.Considered--
			continue
		}

		puller, ok := pullers[conn.SourceType]
		if !ok {
			result.Failed++
			record(ctx, db, conn, log, fmt.Errorf("%w: %s", ErrNoPuller, conn.SourceType))
			continue
		}

		secret, err := conn.Secret(key)
		if err != nil {
			// Worth distinguishing: a key that opens nothing is a
			// configuration problem, not a source that is down.
			result.Failed++
			record(ctx, db, conn, log, err)
			continue
		}

		notes, cursor, err := puller.Pull(ctx, conn, secret)
		if err != nil {
			result.Failed++
			record(ctx, db, conn, log, err)
			continue
		}

		stored := 0
		for _, note := range notes {
			// Everything a source hands over is third-party text
			// arriving without anyone having read it, which is what
			// the mark exists for (ADR-0009).
			note.MarkTainted(conn.SourceType)
			note.ProjectID = conn.ProjectID
			if err := db.Create(ctx, note); err != nil {
				// One note that will not store should not lose
				// the rest of the page, nor the cursor.
				fmt.Fprintf(log, "  %s: %v\n", conn.SourceType, err)
				continue
			}
			stored++
		}
		result.Pulled += stored

		if err := db.RecordPull(ctx, conn.ID, cursor, ""); err != nil {
			return result, err
		}
		fmt.Fprintf(log, "%s  %s  %d note(s)\n", conn.ID, conn.SourceType, stored)
	}

	return result, nil
}

// record writes why a connection did not work where somebody will find it.
func record(ctx context.Context, db *database.DB, conn *database.Connection, log io.Writer, failure error) {
	fmt.Fprintf(log, "%s  %s  %v\n", conn.ID, conn.SourceType, failure)
	// The cursor is left where it was: a failed pull has not moved on.
	if err := db.RecordPull(ctx, conn.ID, conn.Cursor, failure.Error()); err != nil {
		fmt.Fprintf(log, "  could not record that failure: %v\n", err)
	}
}

// NoteFrom builds a note the way every puller should: the source's own
// identifier in the metadata, so a second pull can tell what it has seen.
func NoteFrom(sourceType, content string, metadata map[string]any, createdAt time.Time) *models.Note {
	note := &models.Note{
		Content:    content,
		SourceType: models.SourceType(sourceType),
		Metadata:   metadata,
	}
	if !createdAt.IsZero() {
		note.CreatedAt = createdAt
	}
	return note
}
