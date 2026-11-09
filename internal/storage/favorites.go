package storage

import (
	"context"
	"fmt"
	"time"
)

// Favorites reads and writes the user's favorite channels.
type Favorites struct {
	store *Store
}

// Add marks channelID as a favorite. Repeated calls keep the first timestamp.
func (r *Favorites) Add(ctx context.Context, channelID int64, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	_, err := r.store.db.Execute(ctx, `
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
	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	_, err := r.store.db.Execute(ctx, `DELETE FROM favorites WHERE channel_id = ?`, channelID)
	if err != nil {
		return fmt.Errorf("remove favorite: %w", classifyError(err))
	}

	return nil
}

// Has reports whether channelID is a favorite.
func (r *Favorites) Has(ctx context.Context, channelID int64) (bool, error) {
	_, found, err := queryRow(
		ctx,
		r.store.db,
		`SELECT 1 FROM favorites WHERE channel_id = ?`,
		channelID,
	)
	if err != nil {
		return false, fmt.Errorf("check favorite: %w", err)
	}

	return found, nil
}

// List returns favorite channels, most recently added first.
func (r *Favorites) List(ctx context.Context) ([]Channel, error) {
	rows, err := r.store.db.Query(ctx, `
		SELECT`+channelColumnsAlias+`
		FROM favorites f
		JOIN channels c ON c.id = f.channel_id
		ORDER BY f.created_at DESC, c.id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}

	return collectChannels(rows)
}
