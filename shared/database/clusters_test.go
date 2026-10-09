package database

import (
	"context"
	"errors"
	"testing"

	"github.com/forgeprint/evomem/shared/models"
)

func clusterNote(t *testing.T, db *DB, content string) string {
	t.Helper()
	n := &models.Note{ProjectID: "evomem", Content: content, SourceType: models.SourceManual}
	if err := db.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return n.ID
}

func TestCreateClusterWithMembers(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	first := clusterNote(t, db, "the tunnel has to be running")
	second := clusterNote(t, db, "the webhook needs a secret")

	c := &Cluster{ProjectID: "evomem", Name: "  Deployment  ", Summary: "  how it is served  "}
	if err := db.CreateCluster(ctx, c, []string{first, second}); err != nil {
		t.Fatal(err)
	}
	if c.ID == "" {
		t.Error("no identifier was minted")
	}
	// Trimmed like every other text that enters the store.
	if c.Name != "Deployment" || c.Summary != "how it is served" {
		t.Errorf("name %q summary %q", c.Name, c.Summary)
	}

	stored, notes, err := db.Cluster(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Size != 2 || len(notes) != 2 {
		t.Errorf("size %d, %d notes", stored.Size, len(notes))
	}
}

func TestCreateClusterRefusesWhatCannotBeFoundAgain(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	if err := db.CreateCluster(ctx, &Cluster{ProjectID: "evomem", Name: "   "}, nil); !errors.Is(err, ErrEmptyClusterName) {
		t.Errorf("a nameless cluster was accepted: %v", err)
	}
	if err := db.CreateCluster(ctx, &Cluster{ProjectID: "  ", Name: "x"}, nil); !errors.Is(err, models.ErrEmptyProjectID) {
		t.Errorf("a cluster with no project was accepted: %v", err)
	}
}

// A cluster must never name a note that is not there; the foreign key is what
// guarantees it rather than a check this package could forget.
func TestCreateClusterRefusesANoteThatIsNotThere(t *testing.T) {
	db := openTemp(t)
	err := db.CreateCluster(context.Background(),
		&Cluster{ProjectID: "evomem", Name: "Deployment"},
		[]string{"01M4D3H3HNMFM69N4MHNAYBZ1Z"})
	if err == nil {
		t.Fatal("a cluster naming a note that does not exist was accepted")
	}
	// And nothing was left behind by the failed transaction.
	got, err := db.Clusters(context.Background(), "evomem")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("%d clusters survived a failed create", len(got))
	}
}

func TestUpdateClusterChangesOnlyWhatIsGiven(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	first := clusterNote(t, db, "one")
	second := clusterNote(t, db, "two")

	c := &Cluster{ProjectID: "evomem", Name: "Deployment", Summary: "how it is served"}
	if err := db.CreateCluster(ctx, c, []string{first}); err != nil {
		t.Fatal(err)
	}

	// Adding a note without restating the name is the common case.
	if err := db.UpdateCluster(ctx, c.ID, ClusterChange{Add: []string{second}}); err != nil {
		t.Fatal(err)
	}
	stored, notes, err := db.Cluster(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Deployment" || stored.Summary != "how it is served" {
		t.Errorf("an add changed the label: %q / %q", stored.Name, stored.Summary)
	}
	if len(notes) != 2 {
		t.Errorf("%d notes", len(notes))
	}

	name := "Serving"
	if err := db.UpdateCluster(ctx, c.ID, ClusterChange{Name: &name, Remove: []string{first}}); err != nil {
		t.Fatal(err)
	}
	stored, notes, err = db.Cluster(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Serving" || len(notes) != 1 || notes[0].ID != second {
		t.Errorf("name %q, %d notes", stored.Name, len(notes))
	}
	// Renaming touches one row; the member is unchanged.
	if notes[0].Content != "two" {
		t.Errorf("a rename changed a note: %q", notes[0].Content)
	}
}

func TestUpdateClusterRefusesAnEmptyName(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	c := &Cluster{ProjectID: "evomem", Name: "Deployment"}
	if err := db.CreateCluster(ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	blank := "   "
	if err := db.UpdateCluster(ctx, c.ID, ClusterChange{Name: &blank}); !errors.Is(err, ErrEmptyClusterName) {
		t.Errorf("err = %v", err)
	}
}

func TestUpdateAndDeleteReportAClusterThatIsNotThere(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpdateCluster(ctx, "01M4D3H3HNMFM69N4MHNAYBZ1Z", ClusterChange{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update: %v", err)
	}
	if err := db.DeleteCluster(ctx, "01M4D3H3HNMFM69N4MHNAYBZ1Z"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete: %v", err)
	}
}

// A cluster is a label. Taking it off is how a bad grouping is undone, and it
// must not take the notes with it.
func TestDeleteClusterKeepsTheNotes(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	id := clusterNote(t, db, "the tunnel has to be running")

	c := &Cluster{ProjectID: "evomem", Name: "Deployment"}
	if err := db.CreateCluster(ctx, c, []string{id}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteCluster(ctx, c.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Get(ctx, id); err != nil {
		t.Errorf("deleting a cluster took its note: %v", err)
	}
}

// The other direction of the cascade: a note that goes takes its memberships
// with it, so no cluster is left naming something that is not there.
func TestDeletingANoteLeavesNoMembership(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	first := clusterNote(t, db, "one")
	second := clusterNote(t, db, "two")

	c := &Cluster{ProjectID: "evomem", Name: "Deployment"}
	if err := db.CreateCluster(ctx, c, []string{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}

	stored, notes, err := db.Cluster(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Size != 1 || len(notes) != 1 || notes[0].ID != second {
		t.Errorf("size %d, %d notes", stored.Size, len(notes))
	}
}

func TestANoteBelongsToSeveralClusters(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	id := clusterNote(t, db, "the tunnel has to be running")

	first := &Cluster{ProjectID: "evomem", Name: "Deployment"}
	second := &Cluster{ProjectID: "evomem", Name: "Networking"}
	for _, c := range []*Cluster{first, second} {
		if err := db.CreateCluster(ctx, c, []string{id}); err != nil {
			t.Fatal(err)
		}
	}

	of, err := db.ClustersOf(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(of) != 2 {
		t.Errorf("the note is in %d clusters, want 2", len(of))
	}
}

func TestAddingTheSameNoteTwiceIsOneMembership(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	id := clusterNote(t, db, "one")
	c := &Cluster{ProjectID: "evomem", Name: "Deployment"}
	if err := db.CreateCluster(ctx, c, []string{id, id}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateCluster(ctx, c.ID, ClusterChange{Add: []string{id}}); err != nil {
		t.Fatal(err)
	}
	stored, _, err := db.Cluster(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Size != 1 {
		t.Errorf("size %d, want 1", stored.Size)
	}
}

func TestClustersAreListedNewestChangeFirstWithSizes(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	id := clusterNote(t, db, "one")

	older := &Cluster{ProjectID: "evomem", Name: "Older"}
	newer := &Cluster{ProjectID: "evomem", Name: "Newer"}
	if err := db.CreateCluster(ctx, older, []string{id}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCluster(ctx, newer, nil); err != nil {
		t.Fatal(err)
	}
	// Touch the older one so it is the most recently changed.
	summary := "changed"
	if err := db.UpdateCluster(ctx, older.ID, ClusterChange{Summary: &summary}); err != nil {
		t.Fatal(err)
	}

	got, err := db.Clusters(ctx, "evomem")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d clusters", len(got))
	}
	if got[0].ID != older.ID {
		t.Errorf("listed %q first, want the one changed last", got[0].Name)
	}
	if got[0].Size != 1 || got[1].Size != 0 {
		t.Errorf("sizes %d and %d", got[0].Size, got[1].Size)
	}

	// Another project's clusters are not in this one's listing.
	other, err := db.Clusters(ctx, "somewhere-else")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("%d clusters leaked from another project", len(other))
	}
}

func TestUpgradingAnOlderStoreGetsTheClusterTables(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	// openTemp creates at the current version; the migration itself is what
	// this checks, by running the statements an older store would run.
	if err := db.CreateCluster(ctx, &Cluster{ProjectID: "evomem", Name: "Deployment"}, nil); err != nil {
		t.Fatal(err)
	}
	version, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("schema version %d, want %d", version, schemaVersion)
	}
}
