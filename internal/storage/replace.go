package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
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
// favorites and recent entries survive a refresh. A channel whose provider ID
// changed but whose primary stream URL stayed the same in that source keeps its
// favorite and history too. A failure leaves the previous rows untouched.
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

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	if err := requireSource(ctx, r.store.db, sourceID); err != nil {
		return ReplaceResult{}, err
	}

	existing, err := existingChannels(ctx, r.store.db, sourceID)
	if err != nil {
		return ReplaceResult{}, err
	}

	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(batch))
	statements := make([]sqlite.Statement, 0, len(batch)*3+len(existing))

	for _, item := range batch {
		channel := item.Channel
		if channel.StreamURL == "" {
			channel.StreamURL = item.Streams[0].URL
		}

		statement, normalized, err := channelStatement(sourceID, channel, now)
		if err != nil {
			return ReplaceResult{}, err
		}
		if _, duplicate := seen[normalized.ProviderID]; duplicate {
			return ReplaceResult{}, fmt.Errorf(
				"%w: duplicate channel provider ID %q",
				ErrInvalid,
				normalized.ProviderID,
			)
		}
		seen[normalized.ProviderID] = struct{}{}
		statements = append(statements, statement)
	}

	statements = append(statements, sqlite.Statement{
		SQL:  `DELETE FROM channel_streams WHERE channel_id IN (SELECT id FROM channels WHERE source_id = ?)`,
		Args: []any{sourceID},
	})

	streamCount := 0
	for _, item := range batch {
		providerID, err := ProviderID(item.Channel.ProviderID, primaryStreamURL(item))
		if err != nil {
			return ReplaceResult{}, err
		}

		streamStatements, count, err := streamStatements(sourceID, providerID, item.Streams)
		if err != nil {
			return ReplaceResult{}, err
		}
		statements = append(statements, streamStatements...)
		streamCount += count
	}

	missing := make([]int64, 0, len(existing))
	for providerID, id := range existing {
		if _, ok := seen[providerID]; !ok {
			missing = append(missing, id)
		}
	}
	statements = append(statements, remapStatements(missing)...)
	statements = append(statements, deleteChannelStatements(missing)...)
	statements = append(statements, refreshSucceededStatement(sourceID, meta))

	if _, err := r.store.db.Transaction(ctx, statements); err != nil {
		return ReplaceResult{}, fmt.Errorf("commit source refresh: %w", classifyError(err))
	}

	return ReplaceResult{
		Channels: len(batch),
		Streams:  streamCount,
		Removed:  len(missing),
	}, nil
}

// ListStreams returns the stream variants for one channel, in order.
func (r *Channels) ListStreams(ctx context.Context, channelID int64) ([]Stream, error) {
	rows, err := r.store.db.Query(ctx, `
		SELECT id, channel_id, provider_id, url, COALESCE(quality, ''),
			is_hls, is_live, position, headers
		FROM channel_streams
		WHERE channel_id = ?
		ORDER BY position, id
	`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list channel streams: %w", err)
	}

	streams := []Stream{}
	for _, values := range rows.Values {
		stream, err := scanStreamValues(values)
		if err != nil {
			return nil, fmt.Errorf("scan channel stream: %w", err)
		}
		streams = append(streams, stream)
	}

	return streams, nil
}

// ListStreamsBySource returns streams grouped by channel ID for one source.
func (r *Channels) ListStreamsBySource(ctx context.Context, sourceID string) (map[int64][]Stream, error) {
	rows, err := r.store.db.Query(ctx, `
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

	grouped := map[int64][]Stream{}
	for _, values := range rows.Values {
		stream, err := scanStreamValues(values)
		if err != nil {
			return nil, fmt.Errorf("scan source stream: %w", err)
		}
		grouped[stream.ChannelID] = append(grouped[stream.ChannelID], stream)
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

func primaryStreamURL(item ChannelBatch) string {
	if item.Channel.StreamURL != "" {
		return item.Channel.StreamURL
	}

	return item.Streams[0].URL
}

func existingChannels(ctx context.Context, db *sqlite.DB, sourceID string) (map[string]int64, error) {
	rows, err := db.Query(ctx, `SELECT id, provider_id FROM channels WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list existing channels: %w", err)
	}

	existing := map[string]int64{}
	for _, values := range rows.Values {
		id, err := valueInt64(values[0])
		if err != nil {
			return nil, fmt.Errorf("scan existing channel id: %w", err)
		}
		providerID, err := valueString(values[1])
		if err != nil {
			return nil, fmt.Errorf("scan existing channel provider: %w", err)
		}
		existing[providerID] = id
	}

	return existing, nil
}

func streamStatements(
	sourceID, channelProviderID string,
	streams []Stream,
) ([]sqlite.Statement, int, error) {
	seen := make(map[string]string, len(streams))
	statements := make([]sqlite.Statement, 0, len(streams))

	for _, stream := range streams {
		providerID, err := ProviderID(stream.ProviderID, stream.URL)
		if err != nil {
			return nil, 0, err
		}
		if existing, exists := seen[providerID]; exists {
			if existing == stream.URL {
				continue
			}

			return nil, 0, fmt.Errorf("%w: duplicate stream provider ID %q", ErrInvalid, providerID)
		}
		seen[providerID] = stream.URL

		headers, err := encodeHeaders(stream.Headers)
		if err != nil {
			return nil, 0, err
		}

		statements = append(statements, sqlite.Statement{
			SQL: `
				INSERT INTO channel_streams (
					channel_id, provider_id, url, quality, is_hls, is_live, position, headers
				)
				SELECT id, ?, ?, ?, ?, ?, ?, ?
				FROM channels
				WHERE source_id = ? AND provider_id = ?
			`,
			Args: []any{
				providerID,
				stream.URL,
				nullString(stream.Quality),
				boolToInt(stream.HLS),
				boolToInt(stream.Live),
				len(statements),
				headers,
				sourceID,
				channelProviderID,
			},
		})
	}

	return statements, len(statements), nil
}

func deleteChannelStatements(ids []int64) []sqlite.Statement {
	statements := make([]sqlite.Statement, 0, len(ids)/deleteChunkSize+1)

	for start := 0; start < len(ids); start += deleteChunkSize {
		end := min(start+deleteChunkSize, len(ids))
		chunk := ids[start:end]

		statements = append(statements, sqlite.Statement{
			SQL:  `DELETE FROM channels WHERE id IN (` + placeholders(len(chunk)) + `)`,
			Args: idArgs(chunk),
		})
	}

	return statements
}

// remapStatements moves favorites and recent entries from the stale channel
// rows in ids to a channel of the same source that kept the primary stream
// URL, then removes the rows still bound to those stale ids.
//
// A provider that starts or stops publishing a tvg-id changes the channel
// identity without changing the stream, so the user's favorite and history
// must follow the stream rather than the identity. Display names are never
// used: a rename alone does not move user data. A stale channel whose stream
// URL disappeared keeps no row at all.
func remapStatements(ids []int64) []sqlite.Statement {
	statements := make([]sqlite.Statement, 0, len(ids)/deleteChunkSize*4+4)

	for start := 0; start < len(ids); start += deleteChunkSize {
		end := min(start+deleteChunkSize, len(ids))
		chunk := ids[start:end]
		args := idArgs(chunk)
		in := `(` + placeholders(len(chunk)) + `)`

		statements = append(statements,
			sqlite.Statement{SQL: remapFavoritesHead + in + remapFavoritesTail, Args: args},
			sqlite.Statement{SQL: `DELETE FROM favorites WHERE channel_id IN ` + in, Args: args},
			sqlite.Statement{SQL: remapRecentHead + in + remapRecentTail, Args: args},
			sqlite.Statement{SQL: `DELETE FROM recent_channels WHERE channel_id IN ` + in, Args: args},
		)
	}

	return statements
}

// remapFavoritesHead copies each stale favorite to every channel of the same
// source that offers the same primary stream URL. An existing favorite keeps
// its original timestamp.
const remapFavoritesHead = `
	INSERT INTO favorites (channel_id, created_at)
	SELECT n.id, f.created_at
	FROM favorites f
	JOIN channels o ON o.id = f.channel_id
	JOIN channels n ON n.source_id = o.source_id
		AND n.id <> o.id
		AND n.stream_url = o.stream_url
	WHERE o.id IN `

const remapFavoritesTail = `
	ON CONFLICT (channel_id) DO NOTHING
`

// remapRecentHead copies each stale recent entry to the same replacement
// channels. The merge keeps the newest play time and the highest play count so
// a refresh never inflates the user's history.
const remapRecentHead = `
	INSERT INTO recent_channels (channel_id, played_at, play_count)
	SELECT n.id, r.played_at, r.play_count
	FROM recent_channels r
	JOIN channels o ON o.id = r.channel_id
	JOIN channels n ON n.source_id = o.source_id
		AND n.id <> o.id
		AND n.stream_url = o.stream_url
	WHERE o.id IN `

const remapRecentTail = `
	ON CONFLICT (channel_id) DO UPDATE SET
		played_at = MAX(recent_channels.played_at, excluded.played_at),
		play_count = MAX(recent_channels.play_count, excluded.play_count)
`

func refreshSucceededStatement(sourceID string, meta RefreshMeta) sqlite.Statement {
	return sqlite.Statement{
		SQL: `
			UPDATE sources
			SET last_refresh_at = ?, last_success_at = ?, refresh_etag = ?,
				refresh_error = NULL, item_count = ?, updated_at = ?
			WHERE id = ?
		`,
		Args: []any{
			unixMillis(meta.RefreshedAt),
			unixMillis(meta.RefreshedAt),
			nullString(meta.ETag),
			meta.ItemCount,
			unixMillis(time.Now().UTC()),
			sourceID,
		},
	}
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

func scanStreamValues(values []any) (Stream, error) {
	if len(values) < 9 {
		return Stream{}, fmt.Errorf("%w: stream row has %d columns", ErrInvalid, len(values))
	}

	var (
		stream Stream
		err    error
		hls    int64
		live   int64
	)

	if stream.ID, err = valueInt64(values[0]); err != nil {
		return Stream{}, err
	}
	if stream.ChannelID, err = valueInt64(values[1]); err != nil {
		return Stream{}, err
	}
	if stream.ProviderID, err = valueString(values[2]); err != nil {
		return Stream{}, err
	}
	if stream.URL, err = valueString(values[3]); err != nil {
		return Stream{}, err
	}
	if stream.Quality, err = valueString(values[4]); err != nil {
		return Stream{}, err
	}
	if hls, err = valueInt64(values[5]); err != nil {
		return Stream{}, err
	}
	if live, err = valueInt64(values[6]); err != nil {
		return Stream{}, err
	}
	position, err := valueInt64(values[7])
	if err != nil {
		return Stream{}, err
	}
	stream.Position = int(position)
	headers, hasHeaders, err := valueOptionalString(values[8])
	if err != nil {
		return Stream{}, err
	}

	stream.HLS = hls != 0
	stream.Live = live != 0
	if hasHeaders && headers != "" {
		decoded := map[string]string{}
		if err := json.Unmarshal([]byte(headers), &decoded); err == nil {
			stream.Headers = decoded
		}
	}

	return stream, nil
}
