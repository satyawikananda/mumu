package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Channels reads and writes channel records scoped to a source.
type Channels struct {
	db *sql.DB
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

// Upsert inserts or updates one channel and returns it with its local ID.
func (r *Channels) Upsert(ctx context.Context, sourceID string, channel Channel) (Channel, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Channel{}, fmt.Errorf("begin channel update: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // rollback after commit is a no-op
	}()

	if err := requireSource(ctx, tx, sourceID); err != nil {
		return Channel{}, err
	}

	id, normalized, err := upsertChannelTx(ctx, tx, sourceID, channel, time.Now().UTC())
	if err != nil {
		return Channel{}, err
	}

	if err := tx.Commit(); err != nil {
		return Channel{}, fmt.Errorf("commit channel update: %w", err)
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin channel batch: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // rollback after commit is a no-op
	}()

	if err := requireSource(ctx, tx, sourceID); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, channel := range channels {
		if _, _, err := upsertChannelTx(ctx, tx, sourceID, channel, now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit channel batch: %w", err)
	}

	return nil
}

func requireSource(ctx context.Context, tx *sql.Tx, sourceID string) error {
	if sourceID == "" {
		return fmt.Errorf("%w: channel has no source", ErrInvalid)
	}

	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM sources WHERE id = ?`, sourceID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: source", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("find channel source: %w", err)
	}

	return nil
}

func upsertChannelTx(
	ctx context.Context,
	tx *sql.Tx,
	sourceID string,
	channel Channel,
	now time.Time,
) (int64, Channel, error) {
	providerID, err := ProviderID(channel.ProviderID, channel.StreamURL)
	if err != nil {
		return 0, Channel{}, err
	}
	if channel.Name == "" {
		return 0, Channel{}, fmt.Errorf("%w: channel has no name", ErrInvalid)
	}
	if channel.StreamURL == "" {
		return 0, Channel{}, fmt.Errorf("%w: channel has no stream URL", ErrInvalid)
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
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
		RETURNING id
	`,
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
	).Scan(&id)
	if err != nil {
		return 0, Channel{}, fmt.Errorf("upsert channel: %w", classifyError(err))
	}

	channel.ID = id
	channel.SourceID = sourceID
	channel.ProviderID = providerID
	channel.UpdatedAt = now

	return id, channel, nil
}

// Get returns one channel by local ID.
func (r *Channels) Get(ctx context.Context, id int64) (Channel, bool, error) {
	channel, err := scanChannel(r.db.QueryRowContext(ctx, `SELECT`+channelColumns+`FROM channels WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, false, nil
	}
	if err != nil {
		return Channel{}, false, fmt.Errorf("get channel: %w", err)
	}

	return channel, true, nil
}

// FindByProvider returns a channel scoped to sourceID.
func (r *Channels) FindByProvider(ctx context.Context, sourceID, providerID string) (Channel, bool, error) {
	channel, err := scanChannel(r.db.QueryRowContext(
		ctx,
		`SELECT`+channelColumns+`FROM channels WHERE source_id = ? AND provider_id = ?`,
		sourceID,
		providerID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, false, nil
	}
	if err != nil {
		return Channel{}, false, fmt.Errorf("find channel: %w", err)
	}

	return channel, true, nil
}

// List returns every channel for sourceID ordered by name.
func (r *Channels) List(ctx context.Context, sourceID string) ([]Channel, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT`+channelColumns+`FROM channels WHERE source_id = ? ORDER BY name, id`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return collectChannels(rows)
}

func collectChannels(rows *sql.Rows) ([]Channel, error) {
	channels := []Channel{}
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}

	return channels, nil
}

func scanChannel(scanner rowScanner) (Channel, error) {
	var (
		channel   Channel
		live      int
		updatedAt int64
	)

	err := scanner.Scan(
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
	)
	if err != nil {
		return Channel{}, err
	}

	channel.Live = live != 0
	channel.UpdatedAt = timeFromMillis(updatedAt)

	return channel, nil
}
