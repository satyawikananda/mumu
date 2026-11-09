package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RecentLimit bounds how many recent channels are kept.
const RecentLimit = 100

// RecentChannels reads and writes local playback history.
type RecentChannels struct {
	db *sql.DB
}

// MarkPlayed records a real playback start and prunes old entries.
func (r *RecentChannels) MarkPlayed(ctx context.Context, channelID int64, at time.Time) error {
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recent update: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // rollback after commit is a no-op
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO recent_channels (channel_id, played_at, play_count)
		VALUES (?, ?, 1)
		ON CONFLICT (channel_id) DO UPDATE SET
			played_at = excluded.played_at,
			play_count = recent_channels.play_count + 1
	`, channelID, unixMillis(at))
	if err != nil {
		return fmt.Errorf("mark recent channel: %w", classifyError(err))
	}

	_, err = tx.ExecContext(ctx, `
		DELETE FROM recent_channels
		WHERE channel_id NOT IN (
			SELECT channel_id FROM recent_channels
			ORDER BY played_at DESC, channel_id DESC
			LIMIT ?
		)
	`, RecentLimit)
	if err != nil {
		return fmt.Errorf("prune recent channels: %w", classifyError(err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recent update: %w", err)
	}

	return nil
}

// List returns recent channels, most recent first. limit is bounded to
// [1, RecentLimit].
func (r *RecentChannels) List(ctx context.Context, limit int) ([]Recent, error) {
	if limit <= 0 || limit > RecentLimit {
		limit = RecentLimit
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT`+channelColumnsAlias+`, r.played_at, r.play_count
		FROM recent_channels r
		JOIN channels c ON c.id = r.channel_id
		ORDER BY r.played_at DESC, c.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	recent := []Recent{}
	for rows.Next() {
		var (
			channel   Channel
			live      int
			updatedAt int64
			playedAt  int64
			playCount int
		)

		if err := rows.Scan(
			&channel.ID,
			&channel.SourceID,
			&channel.ProviderID,
			&channel.Name,
			&channel.StreamURL,
			&channel.LogoURL,
			&channel.Country,
			&channel.Category,
			&channel.Language,
			&channel.EPGID,
			&live,
			&updatedAt,
			&playedAt,
			&playCount,
		); err != nil {
			return nil, fmt.Errorf("scan recent channel: %w", err)
		}

		channel.Live = live != 0
		channel.UpdatedAt = timeFromMillis(updatedAt)

		recent = append(recent, Recent{
			Channel:   channel,
			PlayedAt:  timeFromMillis(playedAt),
			PlayCount: playCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent channels: %w", err)
	}

	return recent, nil
}

// Last returns the most recent channel, if any.
func (r *RecentChannels) Last(ctx context.Context) (Recent, bool, error) {
	recent, err := r.List(ctx, 1)
	if err != nil {
		return Recent{}, false, err
	}
	if len(recent) == 0 {
		return Recent{}, false, nil
	}

	return recent[0], true, nil
}
