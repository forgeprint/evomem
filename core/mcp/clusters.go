package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/forgeprint/evomem/shared/database"
	"github.com/forgeprint/evomem/shared/models"
)

// clusterView is one grouping as a client sees it.
type clusterView struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Summary   string `json:"summary,omitempty"`
	Size      int    `json:"size"`
	UpdatedAt string `json:"updated_at"`
}

func viewOfCluster(c database.Cluster) clusterView {
	return clusterView{
		ID:        c.ID,
		ProjectID: c.ProjectID,
		Name:      c.Name,
		Summary:   c.Summary,
		Size:      c.Size,
		UpdatedAt: c.UpdatedAt.Format("2006-01-02 15:04"),
	}
}

// clusterError turns the store's errors into something a model can act on.
func clusterError(err error, id string) error {
	switch {
	case errors.Is(err, models.ErrInvalidULID):
		return fmt.Errorf("%q is not an id; ids come from list_clusters or search_notes", id)
	case errors.Is(err, database.ErrNotFound):
		return fmt.Errorf("there is no cluster %s", id)
	case errors.Is(err, database.ErrEmptyClusterName):
		return errors.New("a cluster needs a name somebody could find it by")
	case errors.Is(err, models.ErrEmptyProjectID):
		return errors.New("a cluster belongs to a project; list_projects says what exists")
	default:
		return err
	}
}

func (s *Server) toolCreateCluster(raw json.RawMessage) (string, any, error) {
	var args struct {
		ProjectID string   `json:"project_id"`
		Name      string   `json:"name"`
		Summary   string   `json:"summary"`
		NoteIDs   []string `json:"note_ids"`
	}
	_ = json.Unmarshal(raw, &args)

	c := &database.Cluster{ProjectID: args.ProjectID, Name: args.Name, Summary: args.Summary}
	if err := s.db.CreateCluster(s.ctx, c, args.NoteIDs); err != nil {
		return "", nil, clusterError(err, "")
	}

	view := viewOfCluster(*c)
	view.Size = len(args.NoteIDs)
	return fmt.Sprintf("Created %s in %s with %d note(s): %s\n%s",
		view.ID, view.ProjectID, view.Size, view.Name,
		"A person sees this with `evomem clusters`, and `delete_cluster` undoes it."), view, nil
}

func (s *Server) toolUpdateCluster(raw json.RawMessage) (string, any, error) {
	var args struct {
		ID            string   `json:"id"`
		Name          *string  `json:"name"`
		Summary       *string  `json:"summary"`
		AddNoteIDs    []string `json:"add_note_ids"`
		RemoveNoteIDs []string `json:"remove_note_ids"`
	}
	_ = json.Unmarshal(raw, &args)

	change := database.ClusterChange{
		Name:    args.Name,
		Summary: args.Summary,
		Add:     args.AddNoteIDs,
		Remove:  args.RemoveNoteIDs,
	}
	if err := s.db.UpdateCluster(s.ctx, args.ID, change); err != nil {
		return "", nil, clusterError(err, args.ID)
	}

	c, notes, err := s.db.Cluster(s.ctx, args.ID)
	if err != nil {
		return "", nil, clusterError(err, args.ID)
	}
	view := viewOfCluster(*c)
	return fmt.Sprintf("%s now holds %d note(s): %s", view.ID, len(notes), view.Name), view, nil
}

func (s *Server) toolDeleteCluster(raw json.RawMessage) (string, any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)

	if err := s.db.DeleteCluster(s.ctx, args.ID); err != nil {
		return "", nil, clusterError(err, args.ID)
	}
	return fmt.Sprintf("Deleted the grouping %s. The notes that were in it are untouched.", args.ID),
		map[string]any{"id": args.ID, "deleted": true}, nil
}

func (s *Server) toolListClusters(raw json.RawMessage) (string, any, error) {
	var args struct {
		ProjectID string `json:"project_id"`
	}
	_ = json.Unmarshal(raw, &args)

	clusters, err := s.db.Clusters(s.ctx, args.ProjectID)
	if err != nil {
		return "", nil, err
	}

	views := make([]clusterView, 0, len(clusters))
	for _, c := range clusters {
		views = append(views, viewOfCluster(c))
	}
	if len(views) == 0 {
		where := "anywhere"
		if args.ProjectID != "" {
			where = args.ProjectID
		}
		return fmt.Sprintf("No groupings in %s yet. `create_cluster` makes one.", where), views, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d grouping(s), most recently changed first.\n", len(views))
	for _, v := range views {
		fmt.Fprintf(&b, "\n%s  %s  %d note(s)  %s\n  %s\n", v.ID, v.ProjectID, v.Size, v.UpdatedAt, v.Name)
		if v.Summary != "" {
			fmt.Fprintf(&b, "  %s\n", v.Summary)
		}
	}
	return b.String(), views, nil
}

func (s *Server) toolGetCluster(raw json.RawMessage) (string, any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)

	c, notes, err := s.db.Cluster(s.ctx, args.ID)
	if err != nil {
		return "", nil, clusterError(err, args.ID)
	}

	views := make([]noteView, 0, len(notes))
	for _, n := range notes {
		views = append(views, viewOf(n))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %d note(s)\n  %s\n", c.ID, c.ProjectID, len(notes), c.Name)
	if c.Summary != "" {
		fmt.Fprintf(&b, "  %s\n", c.Summary)
	}
	for _, v := range views {
		fmt.Fprintf(&b, "\n%s  %s  %s\n", v.ID, v.SourceType, v.CreatedAt)
		if v.Tainted {
			b.WriteString(taintWarning(v.Origin))
		}
		b.WriteString(audioNote(v))
		fmt.Fprintf(&b, "%s\n", v.Content)
	}

	return b.String(), map[string]any{"cluster": viewOfCluster(*c), "notes": views}, nil
}
