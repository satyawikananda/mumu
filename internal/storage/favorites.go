package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Favorites reads and writes the user's favorite channels.
type Favorites struct {
	db *sql.DB
}

// Add marks channelID as a favorite. Repeated calls keep the first timestamp.
func (r *Favorites) Add(ctx context.Context, channelID int64, at time.Time) error {
	exists, err := channelExists(ctx, r.db, channelID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: channel", ErrNotFound)
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO favorites (channel_id, created_at) VALUES (?, ?)
		ON CONFLICT (channel_id) DO NOTHING
	`, channelID, unixMillis(at))
	if err != nil {
		return fmt.Errorf("add favorite: %w", classifyError(err))
	}

	return nil
}

// Remove deletes a favorite. Removing a missing favorite is a no-op.
func (r *Favorites) Remove(ctx context.Context, channelID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM favorites WHERE channel_id = ?`, channelID)
	if err != nil {
		return fmt.Errorf("remove favorite: %w", classifyError(err))
	}

	return nil
}

// Has reports whether channelID is a favorite.
func (r *Favorites) Has(ctx context.Context, channelID int64) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM favorites WHERE channel_id = ?`, channelID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check favorite: %w", err)
	}

	return true, nil
}

// List returns favorite channels, most recently added first.
func (r *Favorites) List(ctx context.Context) ([]Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT`+channelColumnsAlias+`
		FROM favorites f
		JOIN channels c ON c.id = f.channel_id
		ORDER BY f.created_at DESC, c.id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return collectChannels(rows)
}
