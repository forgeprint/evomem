package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// A Cluster is a grouping over notes that are already stored: a label and a
// membership, never content of its own (ADR-0023).
type Cluster struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Size is how many notes are in it. Filled by the listing, because a
	// name without a size says nothing about whether the grouping worked.
	Size int `json:"size"`
}

// ErrEmptyClusterName is a cluster nobody could find again.
var ErrEmptyClusterName = errors.New("database: a cluster needs a name")

const clusterColumns = `id, project_id, name, summary, created_at, updated_at`

// CreateCluster names a grouping and puts notes in it.
//
// The notes are optional: an agent that names a cluster before it has decided
// what belongs in it is doing the same job in two calls.
func (d *DB) CreateCluster(ctx context.Context, c *Cluster, noteIDs []string) error {
	if c == nil {
		return errors.New("database: no cluster given")
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return models.ErrEmptyProjectID
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return ErrEmptyClusterName
	}
	c.Name = name
	c.Summary = strings.TrimSpace(c.Summary)
	if c.ID == "" {
		c.ID = models.NewULID()
	}
	id, err := models.NormalizeULID(c.ID)
	if err != nil {
		return err
	}
	c.ID = id

	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now

	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: creating cluster: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO clusters (`+clusterColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.Name, c.Summary, formatTime(c.CreatedAt), formatTime(c.UpdatedAt),
	); err != nil {
		return fmt.Errorf("database: creating cluster: %w", err)
	}
	if err := addMembers(ctx, tx, c.ID, noteIDs, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: creating cluster: %w", err)
	}
	c.Size = len(noteIDs)
	return nil
}

// addMembers puts notes in a cluster, ignoring one that is already there.
//
// A note that does not exist is refused by the foreign key rather than
// silently stored, so a cluster can never name something that is not there.
func addMembers(ctx context.Context, tx *sql.Tx, clusterID string, noteIDs []string, at time.Time) error {
	for _, raw := range noteIDs {
		noteID, err := models.NormalizeULID(raw)
		if err != nil {
			return fmt.Errorf("database: %q is not a note id: %w", raw, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO cluster_notes (cluster_id, note_id, added_at) VALUES (?, ?, ?)`,
			clusterID, noteID, formatTime(at),
		); err != nil {
			return fmt.Errorf("database: adding %s to cluster %s: %w", noteID, clusterID, err)
		}
	}
	return nil
}

// ClusterChange is what UpdateCluster may change. A nil field is one the
// caller is not touching, which is what lets an agent add a note without
// restating the name.
type ClusterChange struct {
	Name    *string
	Summary *string
	Add     []string
	Remove  []string
}

// UpdateCluster renames, re-summarises and changes who is in a cluster.
func (d *DB) UpdateCluster(ctx context.Context, id string, change ClusterChange) error {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return err
	}

	tx, err := d.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: updating cluster: %w", err)
	}
	defer tx.Rollback()

	var exists string
	switch err := tx.QueryRowContext(ctx, `SELECT id FROM clusters WHERE id = ?`, id).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: cluster %s", ErrNotFound, id)
	case err != nil:
		return fmt.Errorf("database: updating cluster: %w", err)
	}

	now := time.Now().UTC()
	if change.Name != nil {
		name := strings.TrimSpace(*change.Name)
		if name == "" {
			return ErrEmptyClusterName
		}
		if _, err := tx.ExecContext(ctx, `UPDATE clusters SET name = ? WHERE id = ?`, name, id); err != nil {
			return fmt.Errorf("database: renaming cluster: %w", err)
		}
	}
	if change.Summary != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE clusters SET summary = ? WHERE id = ?`, strings.TrimSpace(*change.Summary), id); err != nil {
			return fmt.Errorf("database: summarising cluster: %w", err)
		}
	}
	if err := addMembers(ctx, tx, id, change.Add, now); err != nil {
		return err
	}
	for _, raw := range change.Remove {
		noteID, err := models.NormalizeULID(raw)
		if err != nil {
			return fmt.Errorf("database: %q is not a note id: %w", raw, err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM cluster_notes WHERE cluster_id = ? AND note_id = ?`, id, noteID); err != nil {
			return fmt.Errorf("database: removing %s from cluster %s: %w", noteID, id, err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE clusters SET updated_at = ? WHERE id = ?`, formatTime(now), id); err != nil {
		return fmt.Errorf("database: updating cluster: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: updating cluster: %w", err)
	}
	return nil
}

// DeleteCluster removes a grouping. The notes in it are untouched: a cluster
// is a label, and taking the label off is how a bad grouping is undone.
func (d *DB) DeleteCluster(ctx context.Context, id string) error {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return err
	}
	res, err := d.write.ExecContext(ctx, `DELETE FROM clusters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("database: deleting cluster %s: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("database: deleting cluster %s: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: cluster %s", ErrNotFound, id)
	}
	return nil
}

// Clusters lists a project's groupings, most recently changed first, with how
// many notes are in each.
func (d *DB) Clusters(ctx context.Context, projectID string) ([]Cluster, error) {
	query := `SELECT ` + prefixed(clusterColumns, "c") + `,
		(SELECT COUNT(*) FROM cluster_notes m WHERE m.cluster_id = c.id)
		FROM clusters c`
	var args []any
	if strings.TrimSpace(projectID) != "" {
		query += ` WHERE c.project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY c.updated_at DESC, c.id DESC`

	rows, err := d.read.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("database: listing clusters: %w", err)
	}
	defer rows.Close()

	var out []Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, fmt.Errorf("database: listing clusters: %w", err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: listing clusters: %w", err)
	}
	return out, nil
}

// Cluster reads one grouping and the notes in it, newest note first.
func (d *DB) Cluster(ctx context.Context, id string) (*Cluster, []*models.Note, error) {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return nil, nil, err
	}

	row := d.read.QueryRowContext(ctx, `SELECT `+prefixed(clusterColumns, "c")+`,
		(SELECT COUNT(*) FROM cluster_notes m WHERE m.cluster_id = c.id)
		FROM clusters c WHERE c.id = ?`, id)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("%w: cluster %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("database: reading cluster %s: %w", id, err)
	}

	rows, err := d.read.QueryContext(ctx, `SELECT `+prefixed(noteColumns, "n")+`
		FROM notes n JOIN cluster_notes m ON m.note_id = n.id
		WHERE m.cluster_id = ?
		ORDER BY n.created_at DESC, n.id DESC`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("database: reading cluster %s: %w", id, err)
	}
	defer rows.Close()

	var notes []*models.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("database: reading cluster %s: %w", id, err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("database: reading cluster %s: %w", id, err)
	}
	return c, notes, nil
}

// ClustersOf is which groupings a note is in, for a reader who found the note
// first.
func (d *DB) ClustersOf(ctx context.Context, noteID string) ([]Cluster, error) {
	noteID, err := models.NormalizeULID(noteID)
	if err != nil {
		return nil, err
	}
	rows, err := d.read.QueryContext(ctx, `SELECT `+prefixed(clusterColumns, "c")+`,
		(SELECT COUNT(*) FROM cluster_notes m2 WHERE m2.cluster_id = c.id)
		FROM clusters c JOIN cluster_notes m ON m.cluster_id = c.id
		WHERE m.note_id = ?
		ORDER BY c.updated_at DESC, c.id DESC`, noteID)
	if err != nil {
		return nil, fmt.Errorf("database: reading the clusters of %s: %w", noteID, err)
	}
	defer rows.Close()

	var out []Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, fmt.Errorf("database: reading the clusters of %s: %w", noteID, err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// prefixed qualifies a column list with a table alias, so a join does not
// have to repeat it by hand.
func prefixed(columns, alias string) string {
	parts := strings.Split(columns, ", ")
	for i, part := range parts {
		parts[i] = alias + "." + part
	}
	return strings.Join(parts, ", ")
}

func scanCluster(s interface{ Scan(...any) error }) (*Cluster, error) {
	var (
		c                    Cluster
		createdAt, updatedAt string
	)
	if err := s.Scan(&c.ID, &c.ProjectID, &c.Name, &c.Summary, &createdAt, &updatedAt, &c.Size); err != nil {
		return nil, err
	}
	var err error
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
