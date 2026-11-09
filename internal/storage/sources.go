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
	COALESCE(last_refresh_at, 0), COALESCE(refresh_etag, ''), COALESCE(refresh_error, '')
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
		createdAt int64
		enabled   int
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
		RETURNING id, created_at, enabled
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
	).Scan(&src.ID, &createdAt, &enabled)
	if err != nil {
		return Source{}, fmt.Errorf("upsert source: %w", classifyError(err))
	}

	src.CreatedAt = timeFromMillis(createdAt)
	src.Enabled = enabled != 0

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

// MarkRefreshed records the outcome of a source refresh. refreshError must
// already be safe to show; never pass credentials or full URLs.
func (r *Sources) MarkRefreshed(
	ctx context.Context,
	id string,
	at time.Time,
	etag string,
	refreshError string,
) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE sources
		SET last_refresh_at = ?, refresh_etag = ?, refresh_error = ?, updated_at = ?
		WHERE id = ?
	`,
		unixMillis(at),
		nullString(etag),
		nullString(refreshError),
		unixMillis(time.Now().UTC()),
		id,
	)
	if err != nil {
		return fmt.Errorf("mark source refreshed: %w", classifyError(err))
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
		&src.RefreshETag,
		&src.RefreshError,
	)
	if err != nil {
		return Source{}, err
	}

	src.Kind = SourceKind(kind)
	src.Enabled = enabled != 0
	src.CreatedAt = timeFromMillis(createdAt)
	src.UpdatedAt = timeFromMillis(updatedAt)
	src.LastRefreshAt = nullableMillis(lastRefreshAt)

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
