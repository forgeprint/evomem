package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// A Token is one device's way in (ADR-0030).
//
// The token itself is not in here and never comes out of the store: the row
// holds a hash, and the only moment the secret exists is the reply that
// created it.
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Revoked reports whether this token has been turned off.
func (t *Token) Revoked() bool { return t.RevokedAt != nil }

// ErrEmptyTokenName is a token nobody could tell apart from the others.
var ErrEmptyTokenName = errors.New("database: a token needs a name")

const tokenColumns = `id, name, prefix, created_at, last_used_at, revoked_at`

// tokenPrefixBytes is how much of a token is kept in the clear, as raw
// bytes before encoding. Enough to tell two rows apart by eye, far too
// little to help anybody guess the rest.
const tokenPrefixBytes = 4

// tokenSecretBytes is the part that is actually secret. 32 bytes from
// crypto/rand, which is why hashing it with SHA-256 rather than a slow
// password hash is the right call (ADR-0030).
const tokenSecretBytes = 32

// NewToken mints a token and stores its hash.
//
// Returns the token exactly once. Nothing can read it back afterwards,
// which is the point: a store that could hand it over would be a store
// worth stealing.
func (d *DB) NewToken(ctx context.Context, name string) (*Token, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", ErrEmptyTokenName
	}

	prefix, err := randomText(tokenPrefixBytes)
	if err != nil {
		return nil, "", err
	}
	secret, err := randomText(tokenSecretBytes)
	if err != nil {
		return nil, "", err
	}
	// Shaped so that a token found in a log or a config file is
	// recognisable as one of ours, and so the prefix can be read off it
	// without the secret.
	plaintext := "evm_" + prefix + "_" + secret

	token := &Token{
		ID:        models.NewULID(),
		Name:      name,
		Prefix:    prefix,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := d.write.ExecContext(ctx, `INSERT INTO tokens
		(id, name, prefix, hash, created_at, last_used_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, NULL, NULL)`,
		token.ID, token.Name, token.Prefix, hashToken(plaintext),
		formatTime(token.CreatedAt),
	); err != nil {
		return nil, "", fmt.Errorf("database: storing a token: %w", err)
	}
	return token, plaintext, nil
}

// TokenFor returns the token this secret belongs to, or ErrNotFound.
//
// A revoked token is ErrNotFound too: the caller is deciding whether to
// serve a request, and "revoked" and "never existed" are the same answer to
// that question. Which it was is visible in the listing.
func (d *DB) TokenFor(ctx context.Context, plaintext string) (*Token, error) {
	if plaintext == "" {
		return nil, ErrNotFound
	}
	// Looked up by hash rather than scanned and compared, so the work does
	// not grow with the number of tokens. The hash is of high-entropy
	// input, so there is nothing a lookup leaks by being fast.
	row := d.read.QueryRowContext(ctx,
		`SELECT `+tokenColumns+` FROM tokens WHERE hash = ?`, hashToken(plaintext))

	token, err := scanToken(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("database: looking up a token: %w", err)
	case token.Revoked():
		return nil, ErrNotFound
	}
	return token, nil
}

// lastUsedResolution is how stale "last used" is allowed to get.
//
// Writing on every request would be a write per request, and on SQLite that
// queues behind the single writer (ADR-0004). A minute is far finer than the
// question this answers, which is "has this device been seen lately".
const lastUsedResolution = time.Minute

// TouchToken records that a token was used, at most once a minute.
func (d *DB) TouchToken(ctx context.Context, token *Token) error {
	now := time.Now().UTC()
	if token.LastUsedAt != nil && now.Sub(*token.LastUsedAt) < lastUsedResolution {
		return nil
	}
	if _, err := d.write.ExecContext(ctx,
		`UPDATE tokens SET last_used_at = ? WHERE id = ?`,
		formatTime(now), token.ID); err != nil {
		return fmt.Errorf("database: recording a token's use: %w", err)
	}
	token.LastUsedAt = &now
	return nil
}

// Tokens lists what exists, newest first, revoked ones included.
//
// Revoked rows are kept and shown: "this was revoked in March" is a
// different thing to know than a row that quietly disappeared.
func (d *DB) Tokens(ctx context.Context) ([]*Token, error) {
	rows, err := d.read.QueryContext(ctx,
		`SELECT `+tokenColumns+` FROM tokens ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("database: listing tokens: %w", err)
	}
	defer rows.Close()

	var out []*Token
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, fmt.Errorf("database: listing tokens: %w", err)
		}
		out = append(out, token)
	}
	return out, rows.Err()
}

// RevokeToken turns one off. Revoking one that is already revoked is not an
// error: the caller wanted it off and it is off.
func (d *DB) RevokeToken(ctx context.Context, id string) error {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return err
	}
	res, err := d.write.ExecContext(ctx,
		`UPDATE tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		formatTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("database: revoking token %s: %w", id, err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		// Either it is not there or it was already revoked. Told apart
		// so a panel does not report success for a typo.
		var exists int
		if err := d.read.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tokens WHERE id = ?`, id).Scan(&exists); err != nil {
			return fmt.Errorf("database: revoking token %s: %w", id, err)
		}
		if exists == 0 {
			return fmt.Errorf("%w: token %s", ErrNotFound, id)
		}
	}
	return nil
}

// hashToken is what the row stores.
//
// SHA-256 rather than a password hash on purpose: the input is 32 bytes of
// crypto/rand, so there is no guessing to make expensive, and a slow hash on
// every request would be a cost with no matching benefit (ADR-0030).
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// randomText is n bytes of randomness, in a form that survives a URL, a
// shell and a config file unescaped.
func randomText(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("database: making a token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func scanToken(s interface{ Scan(...any) error }) (*Token, error) {
	var (
		t                   Token
		createdAt           string
		lastUsed, revokedAt sql.NullString
	)
	if err := s.Scan(&t.ID, &t.Name, &t.Prefix, &createdAt, &lastUsed, &revokedAt); err != nil {
		return nil, err
	}
	var err error
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if lastUsed.Valid && lastUsed.String != "" {
		at, err := parseTime(lastUsed.String)
		if err != nil {
			return nil, err
		}
		t.LastUsedAt = &at
	}
	if revokedAt.Valid && revokedAt.String != "" {
		at, err := parseTime(revokedAt.String)
		if err != nil {
			return nil, err
		}
		t.RevokedAt = &at
	}
	return &t, nil
}
