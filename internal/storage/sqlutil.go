package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func unixMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}

	return t.UTC().UnixMilli()
}

func timeFromMillis(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}

	return time.UnixMilli(value).UTC()
}

func nullMillis(t time.Time) any {
	if t.IsZero() {
		return nil
	}

	return unixMillis(t)
}

func nullableMillis(value sql.NullInt64) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return timeFromMillis(value.Int64)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}

	return 0
}

func nullString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func channelExists(ctx context.Context, db *sql.DB, id int64) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM channels WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find channel: %w", err)
	}

	return true, nil
}
