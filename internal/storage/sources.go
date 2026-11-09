package storage

import (
	"context"
	"fmt"
	"time"
)

// Sources reads and writes playlist, stream and built-in source records.
type Sources struct {
	store *Store
}

const sourceColumns = `
	id, kind, identity, location, display_name, enabled,
	created_at, updated_at,
	COALESCE(last_refresh_at, 0), COALESCE(last_success_at, 0),
	COALESCE(refresh_etag, ''), COALESCE(refresh_error, ''), COALESCE(item_count, 0)
`

// Upsert inserts or updates src, returning the stored row with its
// deterministic ID and timestamps.
func (r *Sources) Upsert(ctx context.Context, src Source) (Source, error) {
	src, err := normalizeSource(src)
	if err != nil {
		return Source{}, err
	}

	now := time.Now().UTC()
	if src.CreatedAt.IsZero() {
		src.CreatedAt = now
	}
	src.UpdatedAt = now

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	row, found, err := queryRow(ctx, r.store.db, `
		INSERT INTO sources (
			id, kind, identity, location, display_name, enabled,
			created_at, updated_at, last_refresh_at, refresh_etag, refresh_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, identity) DO UPDATE SET
			location = excluded.location,
			display_name = excluded.display_name,
			updated_at = excluded.updated_at,
			last_refresh_at = excluded.last_refresh_at,
			refresh_etag = excluded.refresh_etag,
			refresh_error = excluded.refresh_error
		RETURNING id
	`,
		src.ID,
		src.Kind,
		src.Identity,
		src.Location,
		src.DisplayName,
		boolToInt(src.Enabled),
		unixMillis(src.CreatedAt),
		unixMillis(src.UpdatedAt),
		nullMillis(src.LastRefreshAt),
		nullString(src.RefreshETag),
		nullString(src.RefreshError),
	)
	if err != nil {
		return Source{}, fmt.Errorf("upsert source: %w", classifyError(err))
	}
	if !found {
		return Source{}, fmt.Errorf("%w: source", ErrNotFound)
	}

	id, err := valueString(row[0])
	if err != nil {
		return Source{}, fmt.Errorf("scan source id: %w", err)
	}
	stored, found, err := r.Get(ctx, id)
	if err != nil {
		return Source{}, err
	}
	if !found {
		return Source{}, fmt.Errorf("%w: source", ErrNotFound)
	}

	return stored, nil
}

// Get returns the source with id, or false when it does not exist.
func (r *Sources) Get(ctx context.Context, id string) (Source, bool, error) {
	row, found, err := queryRow(ctx, r.store.db, `SELECT`+sourceColumns+`FROM sources WHERE id = ?`, id)
	if err != nil {
		return Source{}, false, fmt.Errorf("get source: %w", err)
	}
	if !found {
		return Source{}, false, nil
	}

	var src Source
	if err := scanSourceValues(row, &src); err != nil {
		return Source{}, false, fmt.Errorf("scan source: %w", err)
	}

	return src, true, nil
}

// List returns every source ordered by display name.
func (r *Sources) List(ctx context.Context) ([]Source, error) {
	rows, err := r.store.db.Query(ctx, `SELECT`+sourceColumns+`FROM sources ORDER BY display_name, id`)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}

	sources := []Source{}
	for _, values := range rows.Values {
		var src Source
		if err := scanSourceValues(values, &src); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		sources = append(sources, src)
	}

	return sources, nil
}

// SetEnabled turns a source on or off.
func (r *Sources) SetEnabled(ctx context.Context, id string, enabled bool) error {
	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	result, err := r.store.db.Execute(
		ctx,
		`UPDATE sources SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled),
		unixMillis(time.Now().UTC()),
		id,
	)
	if err != nil {
		return fmt.Errorf("set source enabled: %w", classifyError(err))
	}

	return requireChanged(result, "source")
}

// MarkRefreshSucceeded records a successful source refresh and its item count.
// refreshError is cleared. etag may be empty.
func (r *Sources) MarkRefreshSucceeded(
	ctx context.Context,
	id string,
	at time.Time,
	etag string,
	itemCount int,
) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if itemCount < 0 {
		itemCount = 0
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	result, err := r.store.db.Execute(ctx, `
		UPDATE sources
		SET last_refresh_at = ?, last_success_at = ?, refresh_etag = ?,
			refresh_error = NULL, item_count = ?, updated_at = ?
		WHERE id = ?
	`,
		unixMillis(at),
		unixMillis(at),
		nullString(etag),
		itemCount,
		unixMillis(time.Now().UTC()),
		id,
	)
	if err != nil {
		return fmt.Errorf("mark source refreshed: %w", classifyError(err))
	}

	return requireChanged(result, "source")
}

// MarkRefreshFailed records a failed attempt and keeps the last successful
// metadata and channel rows untouched. message must already be user-safe.
func (r *Sources) MarkRefreshFailed(
	ctx context.Context,
	id string,
	at time.Time,
	message string,
) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	result, err := r.store.db.Execute(ctx, `
		UPDATE sources
		SET last_refresh_at = ?, refresh_error = ?, updated_at = ?
		WHERE id = ?
	`,
		unixMillis(at),
		nullString(message),
		unixMillis(time.Now().UTC()),
		id,
	)
	if err != nil {
		return fmt.Errorf("mark source refresh failed: %w", classifyError(err))
	}

	return requireChanged(result, "source")
}

// Delete removes a source and its channels, favorites and recent entries.
func (r *Sources) Delete(ctx context.Context, id string) error {
	r.store.writeMu.Lock()
	defer r.store.writeMu.Unlock()

	result, err := r.store.db.Execute(ctx, `DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete source: %w", classifyError(err))
	}

	return requireChanged(result, "source")
}

func normalizeSource(src Source) (Source, error) {
	if !src.Kind.Valid() {
		return Source{}, fmt.Errorf("%w: unsupported source kind %q", ErrInvalid, src.Kind)
	}

	identity, location, err := normalizeLocation(src.Kind, src.Location)
	if err != nil {
		return Source{}, err
	}

	src.Identity = identity
	src.Location = location
	src.ID = sourceID(src.Kind, identity)

	return src, nil
}

func scanSourceValues(values []any, src *Source) error {
	if len(values) < 13 {
		return fmt.Errorf("%w: source row has %d columns", ErrInvalid, len(values))
	}

	var (
		err       error
		enabled   int64
		createdAt int64
		updatedAt int64
	)

	if src.ID, err = valueString(values[0]); err != nil {
		return err
	}
	kind, err := valueString(values[1])
	if err != nil {
		return err
	}
	src.Kind = SourceKind(kind)
	if src.Identity, err = valueString(values[2]); err != nil {
		return err
	}
	if src.Location, err = valueString(values[3]); err != nil {
		return err
	}
	if src.DisplayName, err = valueString(values[4]); err != nil {
		return err
	}
	if enabled, err = valueInt64(values[5]); err != nil {
		return err
	}
	if createdAt, err = valueInt64(values[6]); err != nil {
		return err
	}
	if updatedAt, err = valueInt64(values[7]); err != nil {
		return err
	}
	if src.LastRefreshAt, err = valueMillis(values[8]); err != nil {
		return err
	}
	if src.LastSuccessAt, err = valueMillis(values[9]); err != nil {
		return err
	}
	if src.RefreshETag, err = valueString(values[10]); err != nil {
		return err
	}
	if src.RefreshError, err = valueString(values[11]); err != nil {
		return err
	}
	itemCount, err := valueInt64(values[12])
	if err != nil {
		return err
	}
	src.ItemCount = int(itemCount)

	src.Enabled = enabled != 0
	src.CreatedAt = timeFromMillis(createdAt)
	src.UpdatedAt = timeFromMillis(updatedAt)

	return nil
}
