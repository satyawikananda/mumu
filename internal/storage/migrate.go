package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
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

func applyMigrations(ctx context.Context, db *sqlite.DB, migrations []migration) error {
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

func applyMigration(ctx context.Context, db *sqlite.DB, m migration) error {
	statements, err := splitSQL(m.sql)
	if err != nil {
		return fmt.Errorf("migration %d %s: %w", m.version, m.name, err)
	}

	appliedAt := time.Now().UTC().UnixMilli()
	statements = append(statements, sqlite.Statement{
		SQL:  `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		Args: []any{m.version, m.name, appliedAt},
	})

	if _, err := db.Transaction(ctx, statements); err != nil {
		return fmt.Errorf("migration %d %s: %w", m.version, m.name, classifyError(err))
	}

	return nil
}

func createMigrationTable(ctx context.Context, db *sqlite.DB) error {
	_, err := db.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", classifyError(err))
	}

	return nil
}

func currentVersion(ctx context.Context, db *sqlite.DB) (int, error) {
	row, found, err := queryRow(
		ctx,
		db,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if !found {
		return 0, nil
	}
	version, err := valueInt64(row[0])
	if err != nil {
		return 0, fmt.Errorf("scan schema version: %w", err)
	}

	return int(version), nil
}

func tableExists(ctx context.Context, db *sqlite.DB, name string) (bool, error) {
	row, found, err := queryRow(
		ctx,
		db,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	)
	if err != nil {
		return false, fmt.Errorf("inspect schema: %w", err)
	}
	if !found {
		return false, nil
	}
	count, err := valueInt64(row[0])
	if err != nil {
		return false, fmt.Errorf("scan table count: %w", err)
	}

	return count > 0, nil
}

func userTableCount(ctx context.Context, db *sqlite.DB) (int, error) {
	row, found, err := queryRow(ctx, db, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'schema_migrations'
	`)
	if err != nil {
		return 0, fmt.Errorf("inspect existing tables: %w", err)
	}
	if !found {
		return 0, nil
	}
	count, err := valueInt64(row[0])
	if err != nil {
		return 0, fmt.Errorf("scan existing table count: %w", err)
	}

	return int(count), nil
}

// splitSQL splits a migration body into single statements. It tracks quoted
// strings and comments so semicolons inside them stay intact.
func splitSQL(input string) ([]sqlite.Statement, error) {
	statements := []sqlite.Statement{}
	start := 0

	var (
		singleQuoted bool
		doubleQuoted bool
		lineComment  bool
		blockComment bool
	)

	for i := 0; i < len(input); i++ {
		current := input[i]

		switch {
		case lineComment:
			if current == '\n' {
				lineComment = false
			}
		case blockComment:
			if current == '*' && i+1 < len(input) && input[i+1] == '/' {
				blockComment = false
				i++
			}
		case singleQuoted:
			if current == '\'' {
				if i+1 < len(input) && input[i+1] == '\'' {
					i++
				} else {
					singleQuoted = false
				}
			}
		case doubleQuoted:
			if current == '"' {
				doubleQuoted = false
			}
		default:
			switch {
			case current == '-' && i+1 < len(input) && input[i+1] == '-':
				lineComment = true
				i++
			case current == '/' && i+1 < len(input) && input[i+1] == '*':
				blockComment = true
				i++
			case current == '\'':
				singleQuoted = true
			case current == '"':
				doubleQuoted = true
			case current == ';':
				if statement := strings.TrimSpace(input[start:i]); statement != "" {
					statements = append(statements, sqlite.Statement{SQL: statement})
				}
				start = i + 1
			}
		}
	}

	if singleQuoted || doubleQuoted || blockComment {
		return nil, fmt.Errorf("%w: unterminated SQL quote or comment", ErrInvalid)
	}
	if tail := strings.TrimSpace(input[start:]); tail != "" {
		statements = append(statements, sqlite.Statement{SQL: tail})
	}

	return statements, nil
}
