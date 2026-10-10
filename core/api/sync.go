package api

import (
	"context"
	"net/http"
	"time"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// Syncer is the part of the database the pull side needs: the same two
// cursor walks the mirror sync uses, pointed the other way.
//
// Separate from Reader, which is a view — a client asking what is here. These
// two are for a client keeping a copy, which is a different promise (ADR-0034).
type Syncer interface {
	PendingNotes(ctx context.Context, from database.Cursor, limit int) ([]*models.Note, database.Cursor, error)
	PendingDeletions(ctx context.Context, from database.Cursor, limit int) ([]database.Deletion, database.Cursor, error)
}

// maxPullBatch bounds one page, whatever was asked for.
const maxPullBatch = 500

// cursorFrom reads the (since, since_id) pair a client sends back.
//
// Both or neither. An empty pair is the zero cursor, which means "from the
// beginning" — a device that has never pulled. A time without an identifier
// would be a cursor that can only say "this instant", which has to either
// repeat a row or skip one, so it is refused rather than guessed at.
func cursorFrom(r *http.Request) (database.Cursor, bool) {
	query := r.URL.Query()
	since, sinceID := query.Get("since"), query.Get("since_id")
	if since == "" && sinceID == "" {
		return database.Cursor{}, true
	}
	if since == "" || sinceID == "" {
		return database.Cursor{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, since)
	if err != nil {
		return database.Cursor{}, false
	}
	return database.Cursor{UpdatedAt: at.UTC(), ID: sinceID}, true
}

// pullLimit is what the client asked for, bounded.
func pullLimit(r *http.Request) int {
	limit := intParam(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > maxPullBatch {
		return maxPullBatch
	}
	return limit
}

// writeCursor is the shape both pull endpoints answer with.
//
// The cursor comes back even when nothing did, so a client that has caught
// up stores the position it is at rather than an empty string — the mistake
// that made the mirror's own pull fail permanently once it had caught up
// (ADR-0032).
func pullCursor(next database.Cursor) map[string]any {
	if next.IsZero() {
		return map[string]any{"since": "", "since_id": ""}
	}
	return map[string]any{
		"since":    next.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"since_id": next.ID,
	}
}

// handlePullNotes answers notes changed since the client's cursor, oldest
// first.
//
// Not GET /notes. That one is a view — newest first, offset paging, no
// watermark — and a client paging through a view while somebody writes to it
// silently misses rows. This orders by the same (updated_at, id) pair the
// mirror sync uses, which is the ordering the index exists for.
func (s *Server) handlePullNotes(w http.ResponseWriter, r *http.Request) {
	from, ok := cursorFrom(r)
	if !ok {
		http.Error(w, "since and since_id go together, and since is RFC 3339", http.StatusBadRequest)
		return
	}

	notes, next, err := s.syncer.PendingNotes(r.Context(), from, pullLimit(r))
	if err != nil {
		http.Error(w, "could not read notes", http.StatusInternalServerError)
		return
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	// Nothing new: hand back the position the client is already at, so it
	// does not forget where it was.
	if len(notes) == 0 {
		next = from
	}

	writeReadJSON(w, map[string]any{"notes": notes, "cursor": pullCursor(next)})
}

// handlePullDeletions answers the tombstones since the client's cursor.
//
// Without this a note deleted here stays on the device for ever, and the
// device pushes it back (ADR-0033 found that on the other sync path). The
// tombstone is what tells it.
func (s *Server) handlePullDeletions(w http.ResponseWriter, r *http.Request) {
	from, ok := cursorFrom(r)
	if !ok {
		http.Error(w, "since and since_id go together, and since is RFC 3339", http.StatusBadRequest)
		return
	}

	deletions, next, err := s.syncer.PendingDeletions(r.Context(), from, pullLimit(r))
	if err != nil {
		http.Error(w, "could not read deletions", http.StatusInternalServerError)
		return
	}
	if deletions == nil {
		deletions = []database.Deletion{}
	}
	if len(deletions) == 0 {
		next = from
	}

	writeReadJSON(w, map[string]any{"deletions": deletions, "cursor": pullCursor(next)})
}
