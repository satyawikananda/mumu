package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Settings stores small user-owned preferences as key/value pairs.
//
// Keys are owned by the caller. Values are opaque strings; callers that need
// typed values should encode and validate them at the boundary.
type Settings struct {
	db *sql.DB
}

// Get returns the value for key, or false when it is not set.
func (r *Settings) Get(ctx context.Context, key string) (string, bool, error) {
	if err := validateKey(key); err != nil {
		return "", false, err
	}

	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting: %w", err)
	}

	return value, true, nil
}

// Set inserts or replaces key.
func (r *Settings) Set(ctx context.Context, key, value string) error {
	if err := validateKey(key); err != nil {
		return err
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, unixMillis(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("set setting: %w", classifyError(err))
	}

	return nil
}

// Delete removes key. Deleting a missing key is a no-op.
func (r *Settings) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}

	_, err := r.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete setting: %w", classifyError(err))
	}

	return nil
}

// All returns every setting.
func (r *Settings) All(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		settings[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate settings: %w", err)
	}

	return settings, nil
}

func validateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: setting key is empty", ErrInvalid)
	}

	return nil
}
