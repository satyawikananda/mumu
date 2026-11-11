# ADR 0004: Local SQLite storage

- Status: accepted
- Date: 2026-11-09
- Refactor: 2026-11-10 — replaced `modernc.org/sqlite` with the MyGo plugin
- Update: 2026-11-12 — MyGo 0.9 loads SQLite as a native module (#31)
- Issues: #6, #31

## Context

mumu is local-first and has no account or hosted backend. It must keep
sources, channels, favorites, recent playback and preferences across
restarts, while staying portable across the CI matrix. CI cross-builds with
`CGO_ENABLED=0` for darwin, linux and windows, so a cgo-only SQLite driver
would break the build.

Existing data must survive schema changes. A corrupt or newer library must
never be deleted automatically.

## Decision

Use **`github.com/egoist/mygo/plugins/sqlite`**, the official MyGo plugin, as
the only SQLite implementation. The plugin ships prebuilt SQLite libraries and
loads them through purego, so the app stays cgo-free and reads the same SQLite
file format as before.

The storage adapter:

- opens one connection per store with `sqlite.Open` and a five second busy
  timeout;
- applies `PRAGMA journal_mode=WAL`, `synchronous=NORMAL` and
  `foreign_keys=ON` after open;
- serializes read-modify-write sequences with a store-level write mutex;
- runs multi-statement writes through `DB.Transaction` with a bounded batch;
- splits embedded migration SQL into single statements and records the
  migration in the same transaction;
- keeps the embedded, versioned `schema_migrations` runner;
- keeps deterministic source IDs and `UNIQUE(source_id, provider_id)`;
- stores no plaintext auth headers or header tokens;
- rejects source URLs with userinfo with `ErrCredentialsUnsupported`.

The database lives at `mygo.App.Path(mygo.PathUserData)/mumu.db` and is
created with mode `0600`. The app opens it during `WhenReady`, before the
first window frame. A failure keeps the window open and shows an actionable
banner instead of exiting.

The plugin has no `database/sql`, prepared statements or interactive
transactions. Compound writes therefore compute their statements before
`DB.Transaction`; stream rows use `INSERT ... SELECT` on the channel's
`(source_id, provider_id)` identity instead of generated channel IDs.

## Consequences

- No cgo and no system SQLite dependency. The MyGo plugin bundles the native
  library through `mygo build`; `modernc.org/sqlite` is no longer a dependency.
- Binary size and startup are measured in the M1 baseline.
- Migrations are append-only. Destructive migrations need a separate review,
  backup and roll-forward plan.
- Favorites and recent entries survive source re-import because the source ID
  is stable when the credential-free identity is unchanged.
- Network catalog payload caching is out of scope for #6. #8/#10 may add a
  bounded cache table through a later migration; it must remain disposable
  and separate from user-owned data.
- Keychain-backed credentials are deferred until an authenticated-source
  issue needs them.

## Update 2026-11-12 (MyGo 0.9, #31)

The plugin's SQLite is now a **native module** (`Sqlitecore`, written in Zig)
rather than a prebuilt library loaded through `purego` and found with
`sqlite.LibraryPath`. `sqlite.LibraryPath` and `MYGO_SQLITE_LIBRARY` were
removed in MyGo 0.9; the replacement override is `MYGO_NATIVE_SQLITECORE`.

Nothing above changes for mumu's code: the Go API (`Open`, `Execute`, `Query`,
`Transaction`, `Error`), the database file, WAL settings, transaction batching
and migrations are unchanged, and the readers/writers see the same SQLite file
format. What changed is delivery:

- `mygo build` copies `libSqlitecore.dylib` into `Contents/Resources` and signs
  it with the app, so the packaged app never downloads native code;
- `go run ./cmd/mumu` and `go test ./...` download the host library into the
  user cache on first use, which needs network access or a pre-staged
  `MYGO_NATIVE_SQLITECORE`;
- `packaging/macos/verify-app.sh` fails when the packaged module is missing.

The alternative of pinning MyGo 0.8.3 to keep the old loader was rejected: the
M5 revamp needs 0.9's native UI, and an older plugin would be unsupported.
