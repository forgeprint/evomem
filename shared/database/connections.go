package database

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/forgeprint/evomem/shared/models"
)

// A Connection is a source somebody else owns, and what it takes to call it.
//
// Secret is never in this struct when it comes out of the store: reading one
// gives the fields a person configured, and the token is handed over only by
// Secret(), which has to be given the key (ADR-0025).
type Connection struct {
	ID           string     `json:"id"`
	SourceType   string     `json:"source_type"`
	ProjectID    string     `json:"project_id"`
	BaseURL      string     `json:"base_url"`
	Account      string     `json:"account,omitempty"`
	Query        string     `json:"query,omitempty"`
	Cursor       string     `json:"cursor,omitempty"`
	LastPulledAt *time.Time `json:"last_pulled_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// sealed is the ciphertext as stored. Unexported so that a connection
	// cannot be marshalled into a log or an HTTP response with somebody's
	// token in it.
	sealed []byte
}

// ModelSourceType marks a row in this table that is not a source at all: a
// model's API key, sealed the same way and listed by the same panel
// (ADR-0027).
//
// Reusing the keyring rather than building a second one is the decision; the
// cost is that two kinds of row are told apart by a string, and the two
// places that have to care say so where they do it.
const ModelSourceType = "model"

// IsModel reports whether this row holds a model key rather than a source.
func (c *Connection) IsModel() bool { return c.SourceType == ModelSourceType }

// ModelName is which model a model connection names.
//
// The query column means "what we ask this source for", and for a model that
// is which model. The overload has exactly one reader, here, so no caller
// has to know (ADR-0027).
func (c *Connection) ModelName() string { return c.Query }

// Failures a caller has to tell apart.
var (
	// ErrNoSecretKey is EVOMEM_SECRET_KEY missing or unusable. Without it
	// the stored tokens cannot be read and nothing can be pulled.
	ErrNoSecretKey = errors.New("database: no usable EVOMEM_SECRET_KEY")

	// ErrSecretUnreadable is a secret the key did not open: the wrong key,
	// or a row written by a different one.
	ErrSecretUnreadable = errors.New("database: this key does not open that secret")
)

const connectionColumns = `id, source_type, project_id, base_url, account, query,
	cursor, last_pulled_at, last_error, created_at, updated_at`

// SecretKey is the key connections are sealed with.
//
// It is a type rather than a []byte so that a caller cannot pass the token by
// accident, and so the one place that validates its length is here.
type SecretKey struct{ bytes []byte }

// ParseSecretKey reads the key from what the environment held.
//
// 32 bytes, as AES-256 takes. Given as text rather than hex or base64 on
// purpose: a person setting this in a launch agent types a passphrase, and a
// length check they can satisfy by counting is better than an encoding they
// will get wrong once and not notice until a pull fails.
func ParseSecretKey(raw string) (SecretKey, error) {
	key := []byte(raw)
	if len(key) != 32 {
		return SecretKey{}, fmt.Errorf("%w: it has to be exactly 32 bytes, this one is %d",
			ErrNoSecretKey, len(key))
	}
	return SecretKey{bytes: key}, nil
}

// IsZero reports whether no key was configured.
func (k SecretKey) IsZero() bool { return len(k.bytes) == 0 }

func (k SecretKey) seal(plaintext string) ([]byte, error) {
	gcm, err := k.gcm()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("database: sealing a secret: %w", err)
	}
	// The nonce goes in front of the ciphertext: it is not secret, it only
	// has to be different every time, and keeping it with the row is what
	// makes the row self-contained.
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (k SecretKey) open(sealed []byte) (string, error) {
	gcm, err := k.gcm()
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", ErrSecretUnreadable
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrSecretUnreadable
	}
	return string(plaintext), nil
}

func (k SecretKey) gcm() (cipher.AEAD, error) {
	if k.IsZero() {
		return nil, ErrNoSecretKey
	}
	block, err := aes.NewCipher(k.bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSecretKey, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSecretKey, err)
	}
	return gcm, nil
}

// Secret hands over the token this connection was configured with.
//
// It takes the key every time rather than holding the plaintext, so a
// connection sitting in memory is not a token sitting in memory.
func (c *Connection) Secret(key SecretKey) (string, error) {
	return key.open(c.sealed)
}

// AddConnection stores a source and seals its token.
func (d *DB) AddConnection(ctx context.Context, c *Connection, secret string, key SecretKey) error {
	if c == nil {
		return errors.New("database: no connection given")
	}
	if strings.TrimSpace(c.SourceType) == "" {
		return errors.New("database: a connection needs a source type")
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return models.ErrEmptyProjectID
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("database: a connection needs a base url")
	}
	if strings.TrimSpace(secret) == "" {
		return errors.New("database: a connection needs a token")
	}

	sealed, err := key.seal(secret)
	if err != nil {
		return err
	}
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
	c.sealed = sealed

	if _, err := d.write.ExecContext(ctx, `INSERT INTO connections
		(id, source_type, project_id, base_url, account, secret, query, cursor,
		 last_pulled_at, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', NULL, '', ?, ?)`,
		c.ID, c.SourceType, c.ProjectID, c.BaseURL, c.Account, sealed, c.Query,
		formatTime(c.CreatedAt), formatTime(c.UpdatedAt),
	); err != nil {
		return fmt.Errorf("database: adding a connection: %w", err)
	}
	return nil
}

// Connections lists what is configured, newest first. The tokens stay sealed.
func (d *DB) Connections(ctx context.Context) ([]*Connection, error) {
	rows, err := d.read.QueryContext(ctx,
		`SELECT `+connectionColumns+`, secret FROM connections ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("database: listing connections: %w", err)
	}
	defer rows.Close()

	var out []*Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("database: listing connections: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RemoveConnection forgets a source and the token with it.
func (d *DB) RemoveConnection(ctx context.Context, id string) error {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return err
	}
	res, err := d.write.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("database: removing connection %s: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("database: removing connection %s: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: connection %s", ErrNotFound, id)
	}
	return nil
}

// RecordPull saves where a source got to, and why it stopped when it did.
//
// Both in one call: a cursor that moved without the error being cleared would
// say the pull worked and failed at once.
func (d *DB) RecordPull(ctx context.Context, id, cursor, failure string) error {
	id, err := models.NormalizeULID(id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := d.write.ExecContext(ctx, `UPDATE connections
		SET cursor = ?, last_error = ?, last_pulled_at = ?, updated_at = ?
		WHERE id = ?`, cursor, failure, formatTime(now), formatTime(now), id); err != nil {
		return fmt.Errorf("database: recording a pull: %w", err)
	}
	return nil
}

func scanConnection(s interface{ Scan(...any) error }) (*Connection, error) {
	var (
		c                    Connection
		lastPulled           sql.NullString
		createdAt, updatedAt string
	)
	if err := s.Scan(&c.ID, &c.SourceType, &c.ProjectID, &c.BaseURL, &c.Account,
		&c.Query, &c.Cursor, &lastPulled, &c.LastError, &createdAt, &updatedAt,
		&c.sealed); err != nil {
		return nil, err
	}
	var err error
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	if lastPulled.Valid && lastPulled.String != "" {
		at, err := parseTime(lastPulled.String)
		if err != nil {
			return nil, err
		}
		c.LastPulledAt = &at
	}
	return &c, nil
}
