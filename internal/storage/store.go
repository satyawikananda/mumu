package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

const (
	openTimeout = 5 * 1000 // milliseconds, passed to SQLite busy_timeout
)

// Store owns one SQLite database and its repositories.
//
// It is safe for concurrent use. Writers serialize through writeMu so a
// read-modify-write sequence cannot interleave with another write.
type Store struct {
	db      *sqlite.DB
	path    string
	writeMu sync.Mutex
	once    sync.Once
	close   error
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
func (s *Store) Sources() *Sources { return &Sources{store: s} }

// Channels returns the channel repository.
func (s *Store) Channels() *Channels { return &Channels{store: s} }

// Favorites returns the favorite repository.
func (s *Store) Favorites() *Favorites { return &Favorites{store: s} }

// Recent returns the recent-playback repository.
func (s *Store) Recent() *RecentChannels { return &RecentChannels{store: s} }

// Settings returns the preference repository.
func (s *Store) Settings() *Settings { return &Settings{store: s} }

func openDatabase(ctx context.Context, path string) (*sqlite.DB, error) {
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{
		BusyTimeout: openTimeout * time.Millisecond,
	})
	if err != nil {
		return nil, fmt.Errorf("open local library: %w", classifyError(err))
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Execute(ctx, pragma); err != nil {
			_ = db.Close()

			return nil, fmt.Errorf("configure local library: %w", classifyError(err))
		}
	}

	return db, nil
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

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code {
		case 11, 26: // SQLITE_CORRUPT, SQLITE_NOTADB
			return fmt.Errorf("%w: %w", ErrCorrupt, err)
		case 3, 8, 14: // SQLITE_PERM, SQLITE_READONLY, SQLITE_CANTOPEN
			return fmt.Errorf("%w: %w", ErrPermission, err)
		case 19: // SQLITE_CONSTRAINT
			switch sqliteErr.ExtendedCode {
			case 1555, 2067: // primary key, unique
				return fmt.Errorf("%w: %w", ErrDuplicate, err)
			case 787: // foreign key
				return fmt.Errorf("%w: %w", ErrNotFound, err)
			case 1299: // not null
				return fmt.Errorf("%w: %w", ErrInvalid, err)
			}
		}
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
