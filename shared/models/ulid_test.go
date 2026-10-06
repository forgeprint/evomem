package models

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNewULIDShape(t *testing.T) {
	id := NewULID()
	if len(id) != ulidLen {
		t.Fatalf("length is %d, want %d: %q", len(id), ulidLen, id)
	}
	if !ValidULID(id) {
		t.Fatalf("%q does not validate", id)
	}
	if id != strings.ToUpper(id) {
		t.Errorf("%q is not upper case", id)
	}
}

// The reason for choosing ULID over UUID is that the text form sorts in
// creation order, so this is the property to hold onto.
func TestNewULIDSortsInCreationOrder(t *testing.T) {
	const n = 2000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = NewULID()
	}

	sorted := make([]string, n)
	copy(sorted, ids)
	sort.Strings(sorted)

	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("identifier %d is out of order: %q, sorted gives %q", i, ids[i], sorted[i])
		}
	}
}

func TestNewULIDUnique(t *testing.T) {
	seen := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		id := NewULID()
		if seen[id] {
			t.Fatalf("%q was handed out twice", id)
		}
		seen[id] = true
	}
}

func TestULIDTimeRoundTrip(t *testing.T) {
	want := time.UnixMilli(time.Now().UnixMilli()).UTC()
	got, err := ULIDTime(newULIDAt(want))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestULIDTimeOnKnownValue(t *testing.T) {
	// The all-zero timestamp is the Unix epoch.
	got, err := ULIDTime("00000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(time.UnixMilli(0).UTC()) {
		t.Errorf("got %s, want the epoch", got)
	}
}

func TestValidULID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{NewULID(), true},
		{"00000000000000000000000000", true},
		{strings.ToLower(NewULID()), true},
		{"", false},
		{"0000000000000000000000000", false},   // one short
		{"000000000000000000000000000", false}, // one long
		{"80000000000000000000000000", false},  // overflows the 128th bit
		{"0000000000000000000000000I", false},  // I is not in Crockford base32
		{"0000000000000000000000000U", false},
		{"0000000000000000000000000!", false},
	}
	for _, c := range cases {
		if got := ValidULID(c.in); got != c.want {
			t.Errorf("ValidULID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeULID(t *testing.T) {
	id := NewULID()
	got, err := NormalizeULID(strings.ToLower(id))
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Errorf("got %q, want %q", got, id)
	}
	if _, err := NormalizeULID("nope"); err != ErrInvalidULID {
		t.Errorf("got %v, want ErrInvalidULID", err)
	}
}

// A clock that steps backwards must not produce an identifier that sorts
// before one already handed out, or the ordering guarantee is worthless on any
// machine that runs NTP.
func TestNewULIDSurvivesBackwardsClock(t *testing.T) {
	now := time.Now()
	first := newULIDAt(now)
	second := newULIDAt(now.Add(-time.Hour))
	if second <= first {
		t.Errorf("after the clock went back: %q is not above %q", second, first)
	}
}
