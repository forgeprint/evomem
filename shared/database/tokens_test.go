package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestATokenIsHandedOverOnceAndNeverAgain(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	token, plaintext, err := db.NewToken(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if plaintext == "" {
		t.Fatal("no token came back")
	}
	if !strings.HasPrefix(plaintext, "evm_") {
		t.Fatalf("a token should be recognisable as one: %q", plaintext)
	}
	// The prefix is in the clear so a person can tell rows apart; the rest
	// is not in the row at all.
	if !strings.Contains(plaintext, token.Prefix) {
		t.Fatal("the stored prefix is not part of the token")
	}

	listed, err := db.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "phone" {
		t.Fatalf("listing: %+v", listed)
	}
}

// The whole reason the row holds a hash: a stolen database is not a set of
// working credentials.
func TestTheTokenIsNotInTheStore(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	_, plaintext, err := db.NewToken(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}

	// The secret half, which is everything after the prefix.
	secret := plaintext[strings.LastIndexByte(plaintext, '_')+1:]
	var found string
	if err := db.read.QueryRowContext(ctx,
		`SELECT hash FROM tokens`).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(found, secret) {
		t.Fatal("the secret is in the row")
	}
	if found == plaintext {
		t.Fatal("the row holds the token itself")
	}
}

func TestTokenForFindsItsOwn(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	made, plaintext, err := db.NewToken(ctx, "browser")
	if err != nil {
		t.Fatal(err)
	}

	found, err := db.TokenFor(ctx, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != made.ID {
		t.Fatalf("found %s, made %s", found.ID, made.ID)
	}

	for _, wrong := range []string{"", "evm_nope_nope", plaintext + "x"} {
		if _, err := db.TokenFor(ctx, wrong); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q was accepted", wrong)
		}
	}
}

// The unit of revocation is the point of the whole table: losing one device
// has to cost one device.
func TestRevokingOneLeavesTheOthers(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	phone, phoneToken, err := db.NewToken(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}
	_, browserToken, err := db.NewToken(ctx, "browser")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.RevokeToken(ctx, phone.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.TokenFor(ctx, phoneToken); !errors.Is(err, ErrNotFound) {
		t.Fatal("a revoked token still works")
	}
	if _, err := db.TokenFor(ctx, browserToken); err != nil {
		t.Fatalf("revoking one took the other with it: %v", err)
	}
}

// A revoked row is kept, not deleted: "this was revoked in March" is a
// different thing to know than a row that quietly disappeared.
func TestARevokedTokenIsStillListed(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	token, _, err := db.NewToken(ctx, "old laptop")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeToken(ctx, token.ID); err != nil {
		t.Fatal(err)
	}

	listed, err := db.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("%d token(s) listed", len(listed))
	}
	if !listed[0].Revoked() {
		t.Fatal("the listing does not say it was revoked")
	}
}

func TestRevokingWhatIsNotThere(t *testing.T) {
	db := openTemp(t)
	if err := db.RevokeToken(context.Background(), "01M4D3H3HNMFM69N4MHNAYBZ1X"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Revoking twice is not an error: the caller wanted it off and it is off.
func TestRevokingTwiceIsFine(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	token, _, err := db.NewToken(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeToken(ctx, token.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeToken(ctx, token.ID); err != nil {
		t.Fatalf("revoking an already revoked token failed: %v", err)
	}
}

func TestATokenNeedsAName(t *testing.T) {
	db := openTemp(t)
	if _, _, err := db.NewToken(context.Background(), "  "); !errors.Is(err, ErrEmptyTokenName) {
		t.Fatalf("expected ErrEmptyTokenName, got %v", err)
	}
}

// Two tokens must never collide, or revoking one would revoke both.
func TestTokensAreDistinct(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		_, plaintext, err := db.NewToken(ctx, "device")
		if err != nil {
			t.Fatal(err)
		}
		if seen[plaintext] {
			t.Fatal("the same token was issued twice")
		}
		seen[plaintext] = true
	}
}

// Writing on every request would queue behind SQLite's single writer, so
// this is throttled; the staleness it buys is irrelevant to the question
// "has this device been seen lately" (ADR-0030).
func TestLastUsedIsRecordedButNotOnEveryRequest(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	token, _, err := db.NewToken(ctx, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if token.LastUsedAt != nil {
		t.Fatal("a token that was never used has a last-used time")
	}

	if err := db.TouchToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	first := token.LastUsedAt
	if first == nil {
		t.Fatal("using a token did not record it")
	}

	if err := db.TouchToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if token.LastUsedAt != first {
		t.Fatal("a second use within the minute wrote again")
	}

	// An hour ago is well past the resolution, so this one writes.
	hourAgo := time.Now().UTC().Add(-time.Hour)
	token.LastUsedAt = &hourAgo
	if err := db.TouchToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if token.LastUsedAt.Equal(hourAgo) {
		t.Fatal("a use after the resolution did not write")
	}

	reread, err := db.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reread[0].LastUsedAt == nil {
		t.Fatal("the store did not keep it")
	}
}
