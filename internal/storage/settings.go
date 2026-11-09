package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Settings stores small user-owned preferences as key/value pairs.
//
// Keys are owned by the caller. Values are opaque strings; callers that need
// typed values should encode and validate them at the boundary.
type Settings struct {
	store *Store
}

// Get returns the value for key, or false when it is not set.
func (r *Settings) Get(ctx context.Context, key string) (string, bool, error) {
	if err := validateKey(key); err != nil {
		return "", false, err
	}

	row, found, err := queryRow(ctx, r.store.db, `SELECT value FROM settings WHERE key = ?`, key)
	if err != nil {
		return "", false, fmt.Errorf("get setting: %w", err)
	}
	if !found {
		return "", false, nil
	}

	value, err := valueString(row[0])
	if err != nil {
		return "", false, fmt.Errorf("scan setting: %w", err)
	}

	return value, true, nil
}

// Set inserts or replaces key.
func (r *Settings) Set(ctx context.Context, key, value string) error {
	if err := validateKey(key); err != nil {
		return err
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	_, err := r.store.db.Execute(ctx, `
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

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	_, err := r.store.db.Execute(ctx, `DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete setting: %w", classifyError(err))
	}

	return nil
}

// All returns every setting.
func (r *Settings) All(ctx context.Context) (map[string]string, error) {
	rows, err := r.store.db.Query(ctx, `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}

	settings := map[string]string{}
	for _, values := range rows.Values {
		key, err := valueString(values[0])
		if err != nil {
			return nil, fmt.Errorf("scan setting key: %w", err)
		}
		value, err := valueString(values[1])
		if err != nil {
			return nil, fmt.Errorf("scan setting value: %w", err)
		}
		settings[key] = value
	}

	return settings, nil
}

func validateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: setting key is empty", ErrInvalid)
	}

	return nil
}
