package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// Channels reads and writes channel records scoped to a source.
type Channels struct {
	store *Store
}

const channelColumns = `
	id, source_id, provider_id, name, stream_url,
	COALESCE(logo_url, ''), COALESCE(country, ''), COALESCE(category, ''),
	COALESCE(language, ''), COALESCE(epg_id, ''), is_live, updated_at
`

const channelColumnsAlias = `
	c.id, c.source_id, c.provider_id, c.name, c.stream_url,
	COALESCE(c.logo_url, ''), COALESCE(c.country, ''), COALESCE(c.category, ''),
	COALESCE(c.language, ''), COALESCE(c.epg_id, ''), c.is_live, c.updated_at
`

const upsertChannelSQL = `
	INSERT INTO channels (
		source_id, provider_id, name, stream_url, logo_url,
		country, category, language, epg_id, is_live, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (source_id, provider_id) DO UPDATE SET
		name = excluded.name,
		stream_url = excluded.stream_url,
		logo_url = excluded.logo_url,
		country = excluded.country,
		category = excluded.category,
		language = excluded.language,
		epg_id = excluded.epg_id,
		is_live = excluded.is_live,
		updated_at = excluded.updated_at
`

// Upsert inserts or updates one channel and returns it with its local ID.
func (r *Channels) Upsert(ctx context.Context, sourceID string, channel Channel) (Channel, error) {
	statement, normalized, err := channelStatement(sourceID, channel, time.Now().UTC())
	if err != nil {
		return Channel{}, err
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	row, found, err := queryRow(ctx, r.store.db, statement.SQL+` RETURNING id`, statement.Args...)
	if err != nil {
		return Channel{}, fmt.Errorf("upsert channel: %w", classifyError(err))
	}
	if !found {
		return Channel{}, fmt.Errorf("%w: channel", ErrNotFound)
	}

	id, err := valueInt64(row[0])
	if err != nil {
		return Channel{}, fmt.Errorf("scan channel id: %w", err)
	}
	normalized.ID = id

	return normalized, nil
}

// UpsertMany inserts or updates channels for one source in a transaction.
// Duplicate provider IDs in the batch keep the last value.
func (r *Channels) UpsertMany(ctx context.Context, sourceID string, channels []Channel) error {
	if sourceID == "" {
		return fmt.Errorf("%w: channel has no source", ErrInvalid)
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	if err := requireSource(ctx, r.store.db, sourceID); err != nil {
		return err
	}

	now := time.Now().UTC()
	statements := make([]sqlite.Statement, 0, len(channels))
	for _, channel := range channels {
		statement, _, err := channelStatement(sourceID, channel, now)
		if err != nil {
			return err
		}
		statements = append(statements, statement)
	}
	if len(statements) == 0 {
		return nil
	}

	if _, err := r.store.db.Transaction(ctx, statements); err != nil {
		return fmt.Errorf("commit channel batch: %w", classifyError(err))
	}

	return nil
}

func channelStatement(
	sourceID string,
	channel Channel,
	now time.Time,
) (sqlite.Statement, Channel, error) {
	providerID, err := ProviderID(channel.ProviderID, channel.StreamURL)
	if err != nil {
		return sqlite.Statement{}, Channel{}, err
	}
	if channel.Name == "" {
		return sqlite.Statement{}, Channel{}, fmt.Errorf("%w: channel has no name", ErrInvalid)
	}
	if channel.StreamURL == "" {
		return sqlite.Statement{}, Channel{}, fmt.Errorf("%w: channel has no stream URL", ErrInvalid)
	}

	channel.ID = 0
	channel.SourceID = sourceID
	channel.ProviderID = providerID
	channel.UpdatedAt = now

	return sqlite.Statement{
		SQL: upsertChannelSQL,
		Args: []any{
			sourceID,
			providerID,
			channel.Name,
			channel.StreamURL,
			nullString(channel.LogoURL),
			nullString(channel.Country),
			nullString(channel.Category),
			nullString(channel.Language),
			nullString(channel.EPGID),
			boolToInt(channel.Live),
			unixMillis(now),
		},
	}, channel, nil
}

// Get returns one channel by local ID.
func (r *Channels) Get(ctx context.Context, id int64) (Channel, bool, error) {
	row, found, err := queryRow(ctx, r.store.db, `SELECT`+channelColumns+`FROM channels WHERE id = ?`, id)
	if err != nil {
		return Channel{}, false, fmt.Errorf("get channel: %w", err)
	}
	if !found {
		return Channel{}, false, nil
	}

	channel, err := scanChannelValues(row)
	if err != nil {
		return Channel{}, false, fmt.Errorf("scan channel: %w", err)
	}

	return channel, true, nil
}

// FindByProvider returns a channel scoped to sourceID.
func (r *Channels) FindByProvider(ctx context.Context, sourceID, providerID string) (Channel, bool, error) {
	row, found, err := queryRow(
		ctx,
		r.store.db,
		`SELECT`+channelColumns+`FROM channels WHERE source_id = ? AND provider_id = ?`,
		sourceID,
		providerID,
	)
	if err != nil {
		return Channel{}, false, fmt.Errorf("find channel: %w", err)
	}
	if !found {
		return Channel{}, false, nil
	}

	channel, err := scanChannelValues(row)
	if err != nil {
		return Channel{}, false, fmt.Errorf("scan channel: %w", err)
	}

	return channel, true, nil
}

// List returns every channel for sourceID ordered by name.
func (r *Channels) List(ctx context.Context, sourceID string) ([]Channel, error) {
	rows, err := r.store.db.Query(
		ctx,
		`SELECT`+channelColumns+`FROM channels WHERE source_id = ? ORDER BY name, id`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	return collectChannels(rows)
}

// ListAll returns channels across sources. limit <= 0 means no limit.
func (r *Channels) ListAll(ctx context.Context, limit int) ([]Channel, error) {
	query := `SELECT` + channelColumns + `FROM channels ORDER BY name, id`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := r.store.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all channels: %w", err)
	}

	return collectChannels(rows)
}

func collectChannels(rows sqlite.Rows) ([]Channel, error) {
	channels := []Channel{}
	for _, values := range rows.Values {
		channel, err := scanChannelValues(values)
		if err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, channel)
	}

	return channels, nil
}

func scanChannelValues(values []any) (Channel, error) {
	if len(values) < 12 {
		return Channel{}, fmt.Errorf("%w: channel row has %d columns", ErrInvalid, len(values))
	}

	var (
		channel   Channel
		err       error
		live      int64
		updatedAt int64
	)

	if channel.ID, err = valueInt64(values[0]); err != nil {
		return Channel{}, err
	}
	if channel.SourceID, err = valueString(values[1]); err != nil {
		return Channel{}, err
	}
	if channel.ProviderID, err = valueString(values[2]); err != nil {
		return Channel{}, err
	}
	if channel.Name, err = valueString(values[3]); err != nil {
		return Channel{}, err
	}
	if channel.StreamURL, err = valueString(values[4]); err != nil {
		return Channel{}, err
	}
	if channel.LogoURL, err = valueString(values[5]); err != nil {
		return Channel{}, err
	}
	if channel.Country, err = valueString(values[6]); err != nil {
		return Channel{}, err
	}
	if channel.Category, err = valueString(values[7]); err != nil {
		return Channel{}, err
	}
	if channel.Language, err = valueString(values[8]); err != nil {
		return Channel{}, err
	}
	if channel.EPGID, err = valueString(values[9]); err != nil {
		return Channel{}, err
	}
	if live, err = valueInt64(values[10]); err != nil {
		return Channel{}, err
	}
	if updatedAt, err = valueInt64(values[11]); err != nil {
		return Channel{}, err
	}

	channel.Live = live != 0
	channel.UpdatedAt = timeFromMillis(updatedAt)

	return channel, nil
}
