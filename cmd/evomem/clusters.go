package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/forgeprint/evomem/shared/database"
)

// cmdClusters shows what an agent has grouped, and takes a grouping back.
//
// A grouping an agent made and only an agent can see is a change nobody can
// audit. This is the other half of letting the cluster tools write without a
// review queue (ADR-0023).
func cmdClusters(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("clusters", flag.ContinueOnError)
	project := fs.String("project", "", "narrow to one project")
	show := fs.String("show", "", "read one grouping and the notes in it")
	remove := fs.String("delete", "", "delete a grouping; the notes stay")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()

	if *remove != "" {
		if err := db.DeleteCluster(ctx, *remove); err != nil {
			if errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("there is no grouping %s", *remove)
			}
			return err
		}
		fmt.Fprintf(out, "deleted the grouping %s; the notes that were in it are untouched\n", *remove)
		return nil
	}

	if *show != "" {
		cluster, notes, err := db.Cluster(ctx, *show)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				return fmt.Errorf("there is no grouping %s", *show)
			}
			return err
		}
		fmt.Fprintf(out, "%s  %s  %d note(s)\n  %s\n", cluster.ID, cluster.ProjectID, len(notes), cluster.Name)
		if cluster.Summary != "" {
			fmt.Fprintf(out, "  %s\n", cluster.Summary)
		}
		for _, note := range notes {
			fmt.Fprintf(out, "\n%s  %s  %s\n  %s\n",
				note.ID, note.SourceType, note.CreatedAt.Format("2006-01-02 15:04"), note.Content)
		}
		return nil
	}

	clusters, err := db.Clusters(ctx, *project)
	if err != nil {
		return err
	}
	if len(clusters) == 0 {
		fmt.Fprintln(out, "no groupings yet; an agent makes them through the MCP tools")
		return nil
	}

	fmt.Fprintf(out, "%d grouping(s), most recently changed first:\n", len(clusters))
	for _, c := range clusters {
		fmt.Fprintf(out, "\n%s  %s  %d note(s)  %s\n  %s\n",
			c.ID, c.ProjectID, c.Size, c.UpdatedAt.Format("2006-01-02 15:04"), c.Name)
		if c.Summary != "" {
			fmt.Fprintf(out, "  %s\n", c.Summary)
		}
	}
	fmt.Fprintln(out, "\nevomem clusters -show <id>    read one")
	fmt.Fprintln(out, "evomem clusters -delete <id>  take the grouping off; the notes stay")
	return nil
}
