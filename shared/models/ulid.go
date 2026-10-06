package models

import (
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"
)

// A ULID is a 128-bit identifier whose first 48 bits are a millisecond
// timestamp, so the text form sorts in creation order. That ordering is the
// whole point here: SQLite stores a primary key in a B-tree, and monotonic
// keys append to the right-hand edge of it instead of splitting pages all over
// the index the way a UUID does.
//
// Written here rather than taken from a library because this project depends
// on the standard library and modernc.org/sqlite and nothing else (ADR-0002).

// ulidLen is the length of the text form: 26 Crockford base32 characters.
const ulidLen = 26

// crockford is base32 without I, L, O and U, so a transcribed identifier
// cannot be misread.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	// ErrInvalidULID is returned for text that is not a ULID.
	ErrInvalidULID = errors.New("models: not a ULID")

	// decodeCrockford maps a byte to its value, or 0xff when it has none.
	decodeCrockford [256]byte

	ulidMu   sync.Mutex
	lastMs   uint64
	lastRand [10]byte
)

func init() {
	for i := range decodeCrockford {
		decodeCrockford[i] = 0xff
	}
	for i, c := range crockford {
		decodeCrockford[c] = byte(i)
		decodeCrockford[lower(byte(c))] = byte(i)
	}
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// NewULID returns a ULID for the current time.
//
// Two identifiers made in the same millisecond are still ordered: the random
// half is incremented rather than drawn again, which keeps a burst of writes
// appending in the order it happened. Exhausting that half within one
// millisecond would take 2^80 identifiers, so the overflow case cannot be
// reached in practice; it draws fresh randomness if it ever is.
func NewULID() string { return newULIDAt(time.Now()) }

func newULIDAt(t time.Time) string {
	ms := uint64(t.UnixMilli())

	ulidMu.Lock()
	switch {
	case ms > lastMs:
		lastMs = ms
		if _, err := rand.Read(lastRand[:]); err != nil {
			ulidMu.Unlock()
			panic("models: crypto/rand failed: " + err.Error())
		}
	default:
		// Same millisecond, or a clock that went backwards: keep the
		// timestamp we already handed out so the sequence never
		// regresses, and step the random half.
		ms = lastMs
		if !increment(&lastRand) {
			if _, err := rand.Read(lastRand[:]); err != nil {
				ulidMu.Unlock()
				panic("models: crypto/rand failed: " + err.Error())
			}
		}
	}
	entropy := lastRand
	ulidMu.Unlock()

	var raw [16]byte
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], entropy[:])

	return encodeULID(raw)
}

// increment adds one to the entropy half, reporting false on overflow.
func increment(b *[10]byte) bool {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return true
		}
	}
	return false
}

// encodeULID writes the 16 bytes as 26 base32 characters. The first character
// carries only two bits, which is why the canonical form cannot exceed '7'
// there.
func encodeULID(raw [16]byte) string {
	var out [ulidLen]byte
	var acc uint16
	bits := 0
	pos := ulidLen

	for i := len(raw) - 1; i >= 0; i-- {
		acc |= uint16(raw[i]) << bits
		bits += 8
		for bits >= 5 {
			pos--
			out[pos] = crockford[acc&0x1f]
			acc >>= 5
			bits -= 5
		}
	}
	if bits > 0 {
		pos--
		out[pos] = crockford[acc&0x1f]
	}
	for pos > 0 {
		pos--
		out[pos] = '0'
	}
	return string(out[:])
}

// ValidULID reports whether s is a well-formed ULID. It is the check a handler
// does on an identifier that arrived from outside before it reaches a query.
func ValidULID(s string) bool {
	if len(s) != ulidLen {
		return false
	}
	// 26 characters hold 130 bits; the top three of the first character
	// have nowhere to go, so anything above '7' does not round-trip.
	if decodeCrockford[s[0]] > 7 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if decodeCrockford[s[i]] == 0xff {
			return false
		}
	}
	return true
}

// ULIDTime returns the timestamp encoded in a ULID.
func ULIDTime(s string) (time.Time, error) {
	if !ValidULID(s) {
		return time.Time{}, ErrInvalidULID
	}
	var ms uint64
	for i := 0; i < 10; i++ {
		ms = ms<<5 | uint64(decodeCrockford[s[i]])
	}
	// Ten characters cover 50 bits; the timestamp is the low 48.
	return time.UnixMilli(int64(ms & 0xffffffffffff)).UTC(), nil
}

// NormalizeULID upper-cases a ULID so that two spellings of the same
// identifier compare equal. SQLite compares TEXT byte by byte.
func NormalizeULID(s string) (string, error) {
	if !ValidULID(s) {
		return "", ErrInvalidULID
	}
	return strings.ToUpper(s), nil
}
