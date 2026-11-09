package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Sources reads and writes playlist, stream and built-in source records.
type Sources struct {
	db *sql.DB
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

	var (
		createdAt     int64
		enabled       int
		lastSuccessAt int64
		itemCount     int
	)
	err = r.db.QueryRowContext(ctx, `
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
		RETURNING id, created_at, enabled, COALESCE(last_success_at, 0), COALESCE(item_count, 0)
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
	).Scan(&src.ID, &createdAt, &enabled, &lastSuccessAt, &itemCount)
	if err != nil {
		return Source{}, fmt.Errorf("upsert source: %w", classifyError(err))
	}

	src.CreatedAt = timeFromMillis(createdAt)
	src.Enabled = enabled != 0
	src.LastSuccessAt = timeFromMillis(lastSuccessAt)
	src.ItemCount = itemCount

	return src, nil
}

// Get returns the source with id, or false when it does not exist.
func (r *Sources) Get(ctx context.Context, id string) (Source, bool, error) {
	src, err := scanSource(r.db.QueryRowContext(ctx, `SELECT`+sourceColumns+`FROM sources WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, false, nil
	}
	if err != nil {
		return Source{}, false, fmt.Errorf("get source: %w", err)
	}

	return src, true, nil
}

// List returns every source ordered by display name.
func (r *Sources) List(ctx context.Context) ([]Source, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+sourceColumns+`FROM sources ORDER BY display_name, id`)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	sources := []Source{}
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		sources = append(sources, src)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sources: %w", err)
	}

	return sources, nil
}

// SetEnabled turns a source on or off.
func (r *Sources) SetEnabled(ctx context.Context, id string, enabled bool) error {
	result, err := r.db.ExecContext(
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

	result, err := r.db.ExecContext(ctx, `
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

	result, err := r.db.ExecContext(ctx, `
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
	result, err := r.db.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
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

func scanSource(scanner rowScanner) (Source, error) {
	var (
		src           Source
		kind          string
		enabled       int
		createdAt     int64
		updatedAt     int64
		lastRefreshAt sql.NullInt64
		lastSuccessAt sql.NullInt64
	)

	err := scanner.Scan(
		&src.ID,
		&kind,
		&src.Identity,
		&src.Location,
		&src.DisplayName,
		&enabled,
		&createdAt,
		&updatedAt,
		&lastRefreshAt,
		&lastSuccessAt,
		&src.RefreshETag,
		&src.RefreshError,
		&src.ItemCount,
	)
	if err != nil {
		return Source{}, err
	}

	src.Kind = SourceKind(kind)
	src.Enabled = enabled != 0
	src.CreatedAt = timeFromMillis(createdAt)
	src.UpdatedAt = timeFromMillis(updatedAt)
	src.LastRefreshAt = nullableMillis(lastRefreshAt)
	src.LastSuccessAt = nullableMillis(lastSuccessAt)

	return src, nil
}

func requireChanged(result sql.Result, entity string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count changed %s: %w", entity, err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, entity)
	}

	return nil
}
