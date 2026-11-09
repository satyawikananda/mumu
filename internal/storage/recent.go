package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// RecentLimit bounds how many recent channels are kept.
const RecentLimit = 100

// RecentChannels reads and writes local playback history.
type RecentChannels struct {
	store *Store
}

// MarkPlayed records a real playback start and prunes old entries.
func (r *RecentChannels) MarkPlayed(ctx context.Context, channelID int64, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	_, err := r.store.db.Transaction(ctx, []sqlite.Statement{
		{
			SQL: `
				INSERT INTO recent_channels (channel_id, played_at, play_count)
				VALUES (?, ?, 1)
				ON CONFLICT (channel_id) DO UPDATE SET
					played_at = excluded.played_at,
					play_count = recent_channels.play_count + 1
			`,
			Args: []any{channelID, unixMillis(at)},
		},
		{
			SQL: `
				DELETE FROM recent_channels
				WHERE channel_id NOT IN (
					SELECT channel_id FROM recent_channels
					ORDER BY played_at DESC, channel_id DESC
					LIMIT ?
				)
			`,
			Args: []any{RecentLimit},
		},
	})
	if err != nil {
		return fmt.Errorf("mark recent channel: %w", classifyError(err))
	}

	return nil
}

// List returns recent channels, most recent first. limit is bounded to
// [1, RecentLimit].
func (r *RecentChannels) List(ctx context.Context, limit int) ([]Recent, error) {
	if limit <= 0 || limit > RecentLimit {
		limit = RecentLimit
	}

	rows, err := r.store.db.Query(ctx, `
		SELECT`+channelColumnsAlias+`, r.played_at, r.play_count
		FROM recent_channels r
		JOIN channels c ON c.id = r.channel_id
		ORDER BY r.played_at DESC, c.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent channels: %w", err)
	}

	recent := []Recent{}
	for _, values := range rows.Values {
		if len(values) < 14 {
			return nil, fmt.Errorf("%w: recent row has %d columns", ErrInvalid, len(values))
		}

		channel, err := scanChannelValues(values[:12])
		if err != nil {
			return nil, fmt.Errorf("scan recent channel: %w", err)
		}
		playedAt, err := valueInt64(values[12])
		if err != nil {
			return nil, fmt.Errorf("scan recent played time: %w", err)
		}
		playCount, err := valueInt64(values[13])
		if err != nil {
			return nil, fmt.Errorf("scan recent play count: %w", err)
		}

		recent = append(recent, Recent{
			Channel:   channel,
			PlayedAt:  timeFromMillis(playedAt),
			PlayCount: int(playCount),
		})
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
