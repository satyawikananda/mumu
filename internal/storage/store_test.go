package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"
)

func execSQL(t *testing.T, db *sqlite.DB, query string, args ...any) {
	t.Helper()

	if _, err := db.Execute(t.Context(), query, args...); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func queryInt(t *testing.T, db *sqlite.DB, query string, args ...any) int64 {
	t.Helper()

	row, found, err := queryRow(t.Context(), db, query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !found {
		t.Fatalf("query returned no row: %s", query)
	}
	value, err := valueInt64(row[0])
	if err != nil {
		t.Fatalf("read integer: %v", err)
	}

	return value
}

func queryStrings(t *testing.T, db *sqlite.DB, query string, args ...any) []string {
	t.Helper()

	row, found, err := queryRow(t.Context(), db, query, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !found {
		t.Fatalf("query returned no row: %s", query)
	}
	values := make([]string, 0, len(row))
	for _, cell := range row {
		value, err := valueString(cell)
		if err != nil {
			t.Fatalf("read text: %v", err)
		}
		values = append(values, value)
	}

	return values
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	return store
}

func TestOpenCreatesDatabaseAndSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mumu.db")

	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = store.Close() }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode %o, want 600", info.Mode().Perm())
	}

	version := queryInt(t, store.db, `SELECT MAX(version) FROM schema_migrations`)
	if version != 2 {
		t.Fatalf("version %d, want 2", version)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mumu.db")

	first, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	second, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = second.Close() }()

	count := queryInt(t, second.db, `SELECT COUNT(*) FROM schema_migrations`)
	if count != 2 {
		t.Fatalf("migration count %d, want 2", count)
	}
}

func TestOpenCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.db")
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Open(t.Context(), path)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Open error %v, want ErrCorrupt", err)
	}
}

func TestOpenNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mumu.db")

	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	execSQL(
		t,
		store.db,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, 'future', 0)`,
	)
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = Open(t.Context(), path)
	if !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("Open error %v, want ErrSchemaNewer", err)
	}
}

func TestOpenUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mumu.db")

	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	execSQL(t, db, `CREATE TABLE legacy (id INTEGER PRIMARY KEY)`)
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	_, err = Open(t.Context(), path)
	if !errors.Is(err, ErrUnknownSchema) {
		t.Fatalf("Open error %v, want ErrUnknownSchema", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	_, err := Open(t.Context(), "  ")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Open error %v, want ErrInvalid", err)
	}
}

func TestOpenAppliesPragmas(t *testing.T) {
	store := openTestStore(t)

	journal := queryStrings(t, store.db, `PRAGMA journal_mode`)
	if len(journal) == 0 || journal[0] != "wal" {
		t.Fatalf("journal_mode = %v, want wal", journal)
	}
	if value := queryInt(t, store.db, `PRAGMA foreign_keys`); value != 1 {
		t.Fatalf("foreign_keys = %d, want 1", value)
	}
	if value := queryInt(t, store.db, `PRAGMA synchronous`); value != 1 {
		t.Fatalf("synchronous = %d, want 1 (NORMAL)", value)
	}
}
