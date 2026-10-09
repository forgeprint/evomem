package organize

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
)

// Result is what one run did, for the command and the panel to report.
type Result struct {
	// Offered is how many ungrouped notes the model was given.
	Offered int

	// Created is how many clusters were written.
	Created int

	// Grouped is how many notes ended up in one.
	Grouped int

	// Invented is how many ids the model named that were not sent to it.
	// Not an error on its own; a number worth seeing, because a model
	// that invents half its ids is a model pointed at the wrong task.
	Invented int

	// Dropped is how many groups came back with no usable note left.
	Dropped int
}

// Grouper is what a run needs from a model. An interface so a test can
// answer without a server, and so a second provider is an addition rather
// than a rewrite.
type Grouper interface {
	Group(ctx context.Context, items []Item) ([]Group, error)
}

// Run groups a project's ungrouped notes.
//
// Only notes in no cluster are offered, so pressing this twice adds groups
// and never dissolves one (ADR-0027). With dryRun it asks the model and
// reports what it would write, without writing.
func Run(
	ctx context.Context,
	db *database.DB,
	model Grouper,
	projectID string,
	dryRun bool,
	log io.Writer,
) (Result, error) {
	var result Result

	notes, err := db.Ungrouped(ctx, projectID, MaxNotes)
	if err != nil {
		return result, err
	}
	if len(notes) == 0 {
		return result, nil
	}

	items := make([]Item, 0, len(notes))
	known := make(map[string]bool, len(notes))
	for _, n := range notes {
		items = append(items, Item{ID: n.ID, Content: n.Content})
		known[n.ID] = true
	}
	result.Offered = len(items)

	groups, err := model.Group(ctx, items)
	if err != nil {
		return result, err
	}

	// One note goes in one group: the prompt asks for that, and a model
	// that puts a note in two would otherwise have the second group win
	// silently. Taken is what has already been placed.
	taken := make(map[string]bool, len(items))

	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			result.Dropped++
			continue
		}

		members := make([]string, 0, len(g.NoteIDs))
		for _, id := range g.NoteIDs {
			id = strings.TrimSpace(id)
			switch {
			case !known[id]:
				// An id nobody sent it. Dropped rather than
				// passed on: a cluster naming a note that is
				// not there, or one from another project, is
				// a grouping nobody can read.
				result.Invented++
			case taken[id]:
				// Already placed by an earlier group.
			default:
				members = append(members, id)
				taken[id] = true
			}
		}
		if len(members) == 0 {
			result.Dropped++
			continue
		}

		if dryRun {
			fmt.Fprintf(log, "would create  %s  (%d note(s))\n", name, len(members))
			result.Created++
			result.Grouped += len(members)
			continue
		}

		cluster := &database.Cluster{
			ProjectID: projectID,
			Name:      name,
			Summary:   strings.TrimSpace(g.Summary),
		}
		if err := db.CreateCluster(ctx, cluster, members); err != nil {
			// One group that will not store should not lose the
			// rest of the answer, which is the expensive part.
			fmt.Fprintf(log, "  %s: %v\n", name, err)
			for _, id := range members {
				delete(taken, id)
			}
			result.Dropped++
			continue
		}
		fmt.Fprintf(log, "%s  %s  (%d note(s))\n", cluster.ID, name, len(members))
		result.Created++
		result.Grouped += len(members)
	}

	return result, nil
}

// FromConnection builds a service from a model row in the keyring.
//
// Returns ErrNotConfigured when there is no model connected, which callers
// report as the feature being off rather than as a failure.
func FromConnection(connections []*database.Connection, key database.SecretKey) (*Service, error) {
	for _, c := range connections {
		if !c.IsModel() {
			continue
		}
		secret, err := c.Secret(key)
		if err != nil {
			return nil, err
		}
		return New(Config{
			BaseURL: c.BaseURL,
			Token:   secret,
			Model:   c.ModelName(),
		})
	}
	return nil, ErrNotConfigured
}
