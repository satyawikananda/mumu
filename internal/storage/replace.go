package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	maxStreamHeaderBytes = 4096
	deleteChunkSize      = 500
)

// ChannelBatch is one channel and its playable variants for an atomic refresh.
type ChannelBatch struct {
	Channel Channel
	Streams []Stream
}

// RefreshMeta records the outcome that replaces a source's channel rows.
type RefreshMeta struct {
	RefreshedAt time.Time
	ETag        string
	ItemCount   int
}

// ReplaceResult counts what one atomic refresh changed.
type ReplaceResult struct {
	Channels int
	Streams  int
	Removed  int
}

// ReplaceForSource replaces every channel and stream of sourceID in one
// transaction. Channels keep their local IDs when provider IDs are stable, so
// favorites and recent entries survive a refresh. A failure leaves the
// previous rows untouched.
func (r *Channels) ReplaceForSource(
	ctx context.Context,
	sourceID string,
	batch []ChannelBatch,
	meta RefreshMeta,
) (ReplaceResult, error) {
	if strings.TrimSpace(sourceID) == "" {
		return ReplaceResult{}, fmt.Errorf("%w: channel has no source", ErrInvalid)
	}
	if err := validateBatch(batch); err != nil {
		return ReplaceResult{}, err
	}
	if meta.RefreshedAt.IsZero() {
		meta.RefreshedAt = time.Now().UTC()
	}
	if meta.ItemCount <= 0 {
		meta.ItemCount = len(batch)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ReplaceResult{}, fmt.Errorf("begin source refresh: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // rollback after commit is a no-op
	}()

	if err := requireSource(ctx, tx, sourceID); err != nil {
		return ReplaceResult{}, err
	}

	existing, err := existingChannels(ctx, tx, sourceID)
	if err != nil {
		return ReplaceResult{}, err
	}

	channelIDs := make([]int64, 0, len(batch))
	seen := make(map[string]struct{}, len(batch))
	now := time.Now().UTC()

	for _, item := range batch {
		channel := item.Channel
		if channel.StreamURL == "" {
			channel.StreamURL = item.Streams[0].URL
		}

		providerID, err := ProviderID(channel.ProviderID, channel.StreamURL)
		if err != nil {
			return ReplaceResult{}, err
		}
		if _, duplicate := seen[providerID]; duplicate {
			return ReplaceResult{}, fmt.Errorf("%w: duplicate channel provider ID %q", ErrInvalid, providerID)
		}

		id, normalized, err := upsertChannelTx(ctx, tx, sourceID, channel, now)
		if err != nil {
			return ReplaceResult{}, err
		}
		channelIDs = append(channelIDs, id)
		seen[normalized.ProviderID] = struct{}{}
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM channel_streams
		WHERE channel_id IN (SELECT id FROM channels WHERE source_id = ?)
	`, sourceID); err != nil {
		return ReplaceResult{}, fmt.Errorf("clear source streams: %w", classifyError(err))
	}

	streamCount := 0
	for i, item := range batch {
		count, err := insertStreamsTx(ctx, tx, channelIDs[i], item.Streams)
		if err != nil {
			return ReplaceResult{}, err
		}
		streamCount += count
	}

	missing := make([]int64, 0, len(existing))
	for providerID, id := range existing {
		if _, ok := seen[providerID]; !ok {
			missing = append(missing, id)
		}
	}
	if err := deleteChannelsByID(ctx, tx, missing); err != nil {
		return ReplaceResult{}, err
	}

	if err := markRefreshSucceededTx(ctx, tx, sourceID, meta); err != nil {
		return ReplaceResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return ReplaceResult{}, fmt.Errorf("commit source refresh: %w", err)
	}

	return ReplaceResult{
		Channels: len(batch),
		Streams:  streamCount,
		Removed:  len(missing),
	}, nil
}

// ListStreams returns the stream variants for one channel, in order.
func (r *Channels) ListStreams(ctx context.Context, channelID int64) ([]Stream, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, channel_id, provider_id, url, COALESCE(quality, ''),
			is_hls, is_live, position, headers
		FROM channel_streams
		WHERE channel_id = ?
		ORDER BY position, id
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel streams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	streams := []Stream{}
	for rows.Next() {
		stream, err := scanStream(rows)
		if err != nil {
			return nil, fmt.Errorf("scan channel stream: %w", err)
		}
		streams = append(streams, stream)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channel streams: %w", err)
	}

	return streams, nil
}

// ListStreamsBySource returns streams grouped by channel ID for one source.
func (r *Channels) ListStreamsBySource(ctx context.Context, sourceID string) (map[int64][]Stream, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.channel_id, s.provider_id, s.url, COALESCE(s.quality, ''),
			s.is_hls, s.is_live, s.position, s.headers
		FROM channel_streams s
		JOIN channels c ON c.id = s.channel_id
		WHERE c.source_id = ?
		ORDER BY s.channel_id, s.position, s.id
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list source streams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	grouped := map[int64][]Stream{}
	for rows.Next() {
		stream, err := scanStream(rows)
		if err != nil {
			return nil, fmt.Errorf("scan source stream: %w", err)
		}
		grouped[stream.ChannelID] = append(grouped[stream.ChannelID], stream)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source streams: %w", err)
	}

	return grouped, nil
}

func validateBatch(batch []ChannelBatch) error {
	for i, item := range batch {
		if strings.TrimSpace(item.Channel.Name) == "" {
			return fmt.Errorf("%w: channel %d has no name", ErrInvalid, i)
		}
		if len(item.Streams) == 0 {
			return fmt.Errorf("%w: channel %q has no streams", ErrInvalid, item.Channel.Name)
		}
		for j, stream := range item.Streams {
			if strings.TrimSpace(stream.URL) == "" {
				return fmt.Errorf("%w: channel %q stream %d has no URL", ErrInvalid, item.Channel.Name, j)
			}
		}
	}

	return nil
}

func existingChannels(ctx context.Context, tx *sql.Tx, sourceID string) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, provider_id FROM channels WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list existing channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	existing := map[string]int64{}
	for rows.Next() {
		var (
			id         int64
			providerID string
		)
		if err := rows.Scan(&id, &providerID); err != nil {
			return nil, fmt.Errorf("scan existing channel: %w", err)
		}
		existing[providerID] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing channels: %w", err)
	}

	return existing, nil
}

func insertStreamsTx(ctx context.Context, tx *sql.Tx, channelID int64, streams []Stream) (int, error) {
	seen := make(map[string]string, len(streams))
	inserted := 0

	for _, stream := range streams {
		providerID, err := ProviderID(stream.ProviderID, stream.URL)
		if err != nil {
			return 0, err
		}
		if existing, exists := seen[providerID]; exists {
			if existing == stream.URL {
				continue
			}

			return 0, fmt.Errorf("%w: duplicate stream provider ID %q", ErrInvalid, providerID)
		}
		seen[providerID] = stream.URL

		headers, err := encodeHeaders(stream.Headers)
		if err != nil {
			return 0, err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO channel_streams (
				channel_id, provider_id, url, quality, is_hls, is_live, position, headers
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (channel_id, provider_id) DO UPDATE SET
				url = excluded.url,
				quality = excluded.quality,
				is_hls = excluded.is_hls,
				is_live = excluded.is_live,
				position = excluded.position,
				headers = excluded.headers
		`,
			channelID,
			providerID,
			stream.URL,
			nullString(stream.Quality),
			boolToInt(stream.HLS),
			boolToInt(stream.Live),
			inserted,
			headers,
		)
		if err != nil {
			return 0, fmt.Errorf("insert channel stream: %w", classifyError(err))
		}
		inserted++
	}

	return inserted, nil
}

func deleteChannelsByID(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for start := 0; start < len(ids); start += deleteChunkSize {
		end := min(start+deleteChunkSize, len(ids))
		chunk := ids[start:end]

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}

		if _, err := tx.ExecContext(
			ctx,
			`DELETE FROM channels WHERE id IN (`+placeholders+`)`,
			args...,
		); err != nil {
			return fmt.Errorf("remove stale channels: %w", classifyError(err))
		}
	}

	return nil
}

func markRefreshSucceededTx(ctx context.Context, tx *sql.Tx, sourceID string, meta RefreshMeta) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE sources
		SET last_refresh_at = ?, last_success_at = ?, refresh_etag = ?,
			refresh_error = NULL, item_count = ?, updated_at = ?
		WHERE id = ?
	`,
		unixMillis(meta.RefreshedAt),
		unixMillis(meta.RefreshedAt),
		nullString(meta.ETag),
		meta.ItemCount,
		unixMillis(time.Now().UTC()),
		sourceID,
	)
	if err != nil {
		return fmt.Errorf("mark source refreshed: %w", classifyError(err))
	}

	return nil
}

func encodeHeaders(headers map[string]string) (any, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	for name, value := range headers {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "user-agent", "referer":
		default:
			return nil, fmt.Errorf("%w: stream header %q is not allowed", ErrInvalid, name)
		}
		if hasHeaderControl(name) || hasHeaderControl(value) {
			return nil, fmt.Errorf("%w: stream header has control characters", ErrInvalid)
		}
	}
	if len(headers) > 2 {
		return nil, fmt.Errorf("%w: too many stream headers", ErrInvalid)
	}

	data, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("%w: encode stream headers: %w", ErrInvalid, err)
	}
	if len(data) > maxStreamHeaderBytes {
		return nil, fmt.Errorf("%w: stream headers are too large", ErrInvalid)
	}

	return string(data), nil
}

func hasHeaderControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}

	return false
}

func scanStream(scanner rowScanner) (Stream, error) {
	var (
		stream  Stream
		hls     int
		live    int
		headers sql.NullString
	)

	err := scanner.Scan(
		&stream.ID,
		&stream.ChannelID,
		&stream.ProviderID,
		&stream.URL,
		&stream.Quality,
		&hls,
		&live,
		&stream.Position,
		&headers,
	)
	if err != nil {
		return Stream{}, err
	}

	stream.HLS = hls != 0
	stream.Live = live != 0
	if headers.Valid && headers.String != "" {
		decoded := map[string]string{}
		if err := json.Unmarshal([]byte(headers.String), &decoded); err == nil {
			stream.Headers = decoded
		}
	}

	return stream, nil
}
