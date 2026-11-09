package storage

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("migration count %d, want 2", len(migrations))
	}
	if migrations[0].version != 1 || migrations[0].name != "init" {
		t.Fatalf("migration = %+v, want version 1 init", migrations[0])
	}
	if migrations[1].version != 2 || migrations[1].name != "streams" {
		t.Fatalf("migration = %+v, want version 2 streams", migrations[1])
	}
}

func TestParseMigrationName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int
		label   string
		wantErr bool
	}{
		{name: "0001_init.sql", version: 1, label: "init"},
		{name: "0012_add_cache.sql", version: 12, label: "add_cache"},
		{name: "init.sql", wantErr: true},
		{name: "0000_init.sql", wantErr: true},
		{name: "0001.sql", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version, label, err := parseMigrationName(tc.name)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if version != tc.version || label != tc.label {
				t.Fatalf("got %d %q, want %d %q", version, label, tc.version, tc.label)
			}
		})
	}
}

func TestMigrationFailureRollsBack(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()

	migrations := []migration{
		{
			version: 1,
			name:    "create",
			sql: `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
			      INSERT INTO items (name) VALUES ('kept');`,
		},
		{
			version: 2,
			name:    "broken",
			sql:     `INSERT INTO missing_table (id) VALUES (1);`,
		},
	}

	if err := applyMigrations(t.Context(), db, migrations); err == nil {
		t.Fatal("applyMigrations succeeded, want failure")
	}

	version := queryInt(t, db, `SELECT MAX(version) FROM schema_migrations`)
	if version != 1 {
		t.Fatalf("version %d, want 1", version)
	}

	count := queryInt(t, db, `SELECT COUNT(*) FROM items`)
	if count != 1 {
		t.Fatalf("items %d, want 1", count)
	}
}

func TestMigrationPreservesExistingData(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()

	first := []migration{{
		version: 1,
		name:    "create",
		sql:     `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
	}}
	if err := applyMigrations(t.Context(), db, first); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	execSQL(t, db, `INSERT INTO items (name) VALUES ('existing')`)

	second := slices.Concat(first, []migration{{
		version: 2,
		name:    "add_note",
		sql:     `ALTER TABLE items ADD COLUMN note TEXT NOT NULL DEFAULT '';`,
	}})
	if err := applyMigrations(t.Context(), db, second); err != nil {
		t.Fatalf("apply v2: %v", err)
	}

	values := queryStrings(t, db, `SELECT name, note FROM items`)
	if len(values) != 2 || values[0] != "existing" || values[1] != "" {
		t.Fatalf("row = %v, want existing ''", values)
	}
}

func TestMigrationDuplicateVersion(t *testing.T) {
	migrations := []migration{
		{version: 1, name: "one"},
		{version: 1, name: "two"},
	}
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()

	// applyMigrations does not sort; loadMigrations validates duplicates.
	if err := applyMigrations(t.Context(), db, migrations); err == nil {
		t.Fatal("applyMigrations accepted duplicate versions")
	}
}

func TestApplyMigrationsRequiresMigrations(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := applyMigrations(t.Context(), db, nil); err != nil {
		t.Fatalf("empty migrations: %v", err)
	}
}
