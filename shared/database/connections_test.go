package database

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// testKey is the 32 bytes a key has to be, spelled so that it reads as a
// placeholder rather than as somebody's key: a high-entropy literal here
// is indistinguishable from a leaked one, to a scanner and to a reader.
var testKey = strings.Repeat("evomem-test-key-", 2)

func key(t *testing.T) SecretKey {
	t.Helper()
	k, err := ParseSecretKey(testKey)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSecretKeyNeedsTheRightLength(t *testing.T) {
	for _, raw := range []string{"", "short", strings.Repeat("a", 31), strings.Repeat("a", 33)} {
		if _, err := ParseSecretKey(raw); !errors.Is(err, ErrNoSecretKey) {
			t.Errorf("a %d-byte key was accepted", len(raw))
		}
	}
	if _, err := ParseSecretKey(testKey); err != nil {
		t.Errorf("a 32-byte key was refused: %v", err)
	}
}

func TestAConnectionRoundTripsItsToken(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	c := &Connection{
		SourceType: "jira",
		ProjectID:  "evomem",
		BaseURL:    "https://example.atlassian.net",
		Account:    "someone@example.com",
		Query:      "project = EVO ORDER BY updated DESC",
	}
	if err := db.AddConnection(ctx, c, "the-api-token", key(t)); err != nil {
		t.Fatal(err)
	}

	stored, err := db.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("%d connections", len(stored))
	}
	secret, err := stored[0].Secret(key(t))
	if err != nil {
		t.Fatal(err)
	}
	if secret != "the-api-token" {
		t.Errorf("secret = %q", secret)
	}
	if stored[0].Account != "someone@example.com" || stored[0].Query == "" {
		t.Errorf("connection = %+v", stored[0])
	}
}

// The point of sealing it: a copy of the file is not enough.
func TestTheTokenIsNotInTheFile(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	c := &Connection{SourceType: "jira", ProjectID: "evomem", BaseURL: "https://example.atlassian.net"}
	if err := db.AddConnection(ctx, c, "sekritsekritsekrit", key(t)); err != nil {
		t.Fatal(err)
	}

	var stored []byte
	if err := db.read.QueryRowContext(ctx, `SELECT secret FROM connections WHERE id = ?`, c.ID).
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "sekrit") {
		t.Error("the token is in the row as written")
	}
}

// Two writes of the same token must not produce the same bytes, or a reader
// of the file learns that two sources share a token.
func TestSealingIsDifferentEveryTime(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	var sealed [][]byte
	for range 2 {
		c := &Connection{SourceType: "jira", ProjectID: "evomem", BaseURL: "https://example.atlassian.net"}
		if err := db.AddConnection(ctx, c, "same-token", key(t)); err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if err := db.read.QueryRowContext(ctx, `SELECT secret FROM connections WHERE id = ?`, c.ID).
			Scan(&raw); err != nil {
			t.Fatal(err)
		}
		sealed = append(sealed, raw)
	}
	if string(sealed[0]) == string(sealed[1]) {
		t.Error("the same token sealed to the same bytes twice")
	}
}

func TestAnotherKeyDoesNotOpenIt(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	c := &Connection{SourceType: "jira", ProjectID: "evomem", BaseURL: "https://example.atlassian.net"}
	if err := db.AddConnection(ctx, c, "the-api-token", key(t)); err != nil {
		t.Fatal(err)
	}

	other, err := ParseSecretKey(strings.Repeat("another-test-key", 2))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := db.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stored[0].Secret(other); !errors.Is(err, ErrSecretUnreadable) {
		t.Errorf("err = %v, want ErrSecretUnreadable", err)
	}
	// And no key at all is a different answer: nothing was configured.
	if _, err := stored[0].Secret(SecretKey{}); !errors.Is(err, ErrNoSecretKey) {
		t.Errorf("err = %v, want ErrNoSecretKey", err)
	}
}

func TestAddConnectionRefusesWhatItCannotUse(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	for name, c := range map[string]*Connection{
		"no source":  {ProjectID: "evomem", BaseURL: "https://x"},
		"no project": {SourceType: "jira", BaseURL: "https://x"},
		"no url":     {SourceType: "jira", ProjectID: "evomem"},
	} {
		if err := db.AddConnection(ctx, c, "token", key(t)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// A connection with no token could never call anything.
	if err := db.AddConnection(ctx,
		&Connection{SourceType: "jira", ProjectID: "evomem", BaseURL: "https://x"},
		"  ", key(t)); err == nil {
		t.Error("a connection with no token was accepted")
	}
}

func TestRecordPullSavesWhereItGotToAndWhyItStopped(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	c := &Connection{SourceType: "jira", ProjectID: "evomem", BaseURL: "https://example.atlassian.net"}
	if err := db.AddConnection(ctx, c, "token", key(t)); err != nil {
		t.Fatal(err)
	}

	if err := db.RecordPull(ctx, c.ID, "page-2", "the server answered 500"); err != nil {
		t.Fatal(err)
	}
	stored, err := db.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].Cursor != "page-2" || stored[0].LastError == "" {
		t.Errorf("connection = %+v", stored[0])
	}
	// Recorded on the row, not only in a log that has rotated by the time
	// somebody asks.
	if stored[0].LastPulledAt == nil {
		t.Error("no pull time was recorded")
	}

	// A later success clears the failure rather than leaving it to be read
	// as current.
	if err := db.RecordPull(ctx, c.ID, "page-3", ""); err != nil {
		t.Fatal(err)
	}
	stored, _ = db.Connections(ctx)
	if stored[0].LastError != "" {
		t.Errorf("a success left the old failure: %q", stored[0].LastError)
	}
}

func TestRemoveConnectionForgetsTheToken(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	c := &Connection{SourceType: "jira", ProjectID: "evomem", BaseURL: "https://example.atlassian.net"}
	if err := db.AddConnection(ctx, c, "token", key(t)); err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveConnection(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := db.Connections(ctx)
	if len(stored) != 0 {
		t.Errorf("%d connections survived", len(stored))
	}
	if err := db.RemoveConnection(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
