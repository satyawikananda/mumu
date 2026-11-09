package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

	var version int
	if err := store.db.QueryRowContext(t.Context(), `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 1 {
		t.Fatalf("version %d, want 1", version)
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

	var count int
	if err := second.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration count %d, want 1", count)
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
	if _, err := store.db.ExecContext(
		t.Context(),
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, 'future', 0)`,
	); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}
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
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE legacy (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
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

func TestDataSourceNameContainsPragmas(t *testing.T) {
	dsn := dataSourceName("/tmp/mumu.db")
	for _, want := range []string{"foreign_keys", "journal_mode", "busy_timeout", "synchronous"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN %q missing %q", dsn, want)
		}
	}
}
