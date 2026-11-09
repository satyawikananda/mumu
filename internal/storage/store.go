package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // register the sqlite driver
)

const (
	driverName  = "sqlite"
	openTimeout = 5 * 1000 // milliseconds, passed to SQLite busy_timeout
)

// Store owns one SQLite database and its repositories.
//
// It is safe for concurrent use. Callers must close it during shutdown.
type Store struct {
	db    *sql.DB
	path  string
	once  sync.Once
	close error
}

// Open creates or opens the local library at path and applies migrations.
//
// Missing parent directories are created. A corrupt file, an unrecognised
// schema or a newer schema returns a sentinel error without deleting data.
func Open(ctx context.Context, path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: storage path is empty", ErrInvalid)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create storage directory: %w", classifyError(err))
		}
	}

	db, err := openDatabase(ctx, path)
	if err != nil {
		return nil, err
	}

	migrations, err := defaultMigrations()
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("load migrations: %w", err)
	}
	if err := applyMigrations(ctx, db, migrations); err != nil {
		_ = db.Close()

		return nil, err
	}

	if path != ":memory:" {
		// SQLite creates files with the process umask. Keep the library
		// private even when the umask is permissive.
		if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = db.Close()

			return nil, fmt.Errorf("protect storage file: %w", classifyError(err))
		}
	}

	return &Store{db: db, path: path}, nil
}

// Close releases the database. It is idempotent.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}

	s.once.Do(func() {
		s.close = s.db.Close()
	})

	return s.close
}

// Path returns the database path passed to Open.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}

	return s.path
}

// Sources returns the source repository.
func (s *Store) Sources() *Sources { return &Sources{db: s.db} }

// Channels returns the channel repository.
func (s *Store) Channels() *Channels { return &Channels{db: s.db} }

// Favorites returns the favorite repository.
func (s *Store) Favorites() *Favorites { return &Favorites{db: s.db} }

// Recent returns the recent-playback repository.
func (s *Store) Recent() *RecentChannels { return &RecentChannels{db: s.db} }

// Settings returns the preference repository.
func (s *Store) Settings() *Settings { return &Settings{db: s.db} }

func openDatabase(ctx context.Context, path string) (*sql.DB, error) {
	dsn := dataSourceName(path)

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open local library: %w", classifyError(err))
	}

	// One connection keeps SQLite writes serialized and avoids SQLITE_BUSY
	// between repositories. The app is local and low-volume.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("open local library: %w", classifyError(err))
	}

	return db, nil
}

func dataSourceName(path string) string {
	if path == ":memory:" {
		return ":memory:"
	}

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", openTimeout))
	query.Add("_pragma", "synchronous(NORMAL)")
	u.RawQuery = query.Encode()

	return u.String()
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCorrupt) || errors.Is(err, ErrPermission) {
		return err
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %w", ErrPermission, err)
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "file is not a database"),
		strings.Contains(message, "malformed"),
		strings.Contains(message, "database disk image is malformed"):
		return fmt.Errorf("%w: %w", ErrCorrupt, err)
	case strings.Contains(message, "permission denied"),
		strings.Contains(message, "read-only"),
		strings.Contains(message, "unable to open database file"):
		return fmt.Errorf("%w: %w", ErrPermission, err)
	default:
		return err
	}
}
