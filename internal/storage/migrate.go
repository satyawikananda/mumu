package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

var defaultMigrations = sync.OnceValues(loadMigrations)

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, name, err := parseMigrationName(entry.Name())
		if err != nil {
			return nil, err
		}

		body, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    name,
			sql:     string(body),
		})
	}

	slices.SortFunc(migrations, func(a, b migration) int {
		return a.version - b.version
	})

	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].version == migrations[i].version {
			return nil, fmt.Errorf("duplicate migration version %d", migrations[i].version)
		}
	}

	return migrations, nil
}

func parseMigrationName(name string) (version int, label string, err error) {
	base := strings.TrimSuffix(name, ".sql")
	prefix, rest, found := strings.Cut(base, "_")
	if !found {
		return 0, "", fmt.Errorf("migration %q has no version prefix", name)
	}

	version, err = strconv.Atoi(prefix)
	if err != nil || version <= 0 {
		return 0, "", fmt.Errorf("migration %q has an invalid version", name)
	}
	if rest == "" {
		return 0, "", fmt.Errorf("migration %q has no name", name)
	}

	return version, rest, nil
}

func applyMigrations(ctx context.Context, db *sql.DB, migrations []migration) error {
	if len(migrations) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].version >= migrations[i].version {
			return fmt.Errorf(
				"migrations are not strictly increasing: %d then %d",
				migrations[i-1].version,
				migrations[i].version,
			)
		}
	}

	exists, err := tableExists(ctx, db, "schema_migrations")
	if err != nil {
		return err
	}
	if !exists {
		other, err := userTableCount(ctx, db)
		if err != nil {
			return err
		}
		if other > 0 {
			return ErrUnknownSchema
		}
		if err := createMigrationTable(ctx, db); err != nil {
			return err
		}
	}

	current, err := currentVersion(ctx, db)
	if err != nil {
		return err
	}

	latest := migrations[len(migrations)-1].version
	if current > latest {
		return fmt.Errorf("%w: database version %d, app supports %d", ErrSchemaNewer, current, latest)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return err
		}
	}

	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.version, err)
	}
	defer func() {
		_ = tx.Rollback() // rollback after commit is a no-op
	}()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("migration %d %s: %w", m.version, m.name, err)
	}

	appliedAt := time.Now().UTC().UnixMilli()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version,
		m.name,
		appliedAt,
	); err != nil {
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.version, err)
	}

	return nil
}

func createMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	return nil
}

func currentVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}

	return version, nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect schema: %w", err)
	}

	return count > 0, nil
}

func userTableCount(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'schema_migrations'
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("inspect existing tables: %w", err)
	}

	return count, nil
}
