package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

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

// queryRow returns the first row, or false when the statement returned none.
func queryRow(ctx context.Context, db *sqlite.DB, query string, args ...any) ([]any, bool, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	if len(rows.Values) == 0 {
		return nil, false, nil
	}

	return rows.Values[0], true, nil
}

func valueInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case float64:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("%w: cannot read integer from %T", ErrInvalid, value)
	}
}

func valueString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("%w: cannot read text from %T", ErrInvalid, value)
	}
}

func valueOptionalString(value any) (string, bool, error) {
	if value == nil {
		return "", false, nil
	}

	text, err := valueString(value)

	return text, true, err
}

func valueMillis(value any) (time.Time, error) {
	if value == nil {
		return time.Time{}, nil
	}
	millis, err := valueInt64(value)
	if err != nil {
		return time.Time{}, err
	}

	return timeFromMillis(millis), nil
}

func requireChanged(result sqlite.Result, entity string) error {
	if result.Changes == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, entity)
	}

	return nil
}

func requireSource(ctx context.Context, db *sqlite.DB, sourceID string) error {
	if sourceID == "" {
		return fmt.Errorf("%w: channel has no source", ErrInvalid)
	}

	if _, found, err := queryRow(ctx, db, `SELECT 1 FROM sources WHERE id = ?`, sourceID); err != nil {
		return fmt.Errorf("find channel source: %w", err)
	} else if !found {
		return fmt.Errorf("%w: source", ErrNotFound)
	}

	return nil
}
