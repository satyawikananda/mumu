# ADR 0004: Local SQLite storage

- Status: accepted
- Date: 2026-11-09
- Issues: #6

## Context

mumu is local-first and has no account or hosted backend. It must keep
sources, channels, favorites, recent playback and preferences across
restarts, while staying portable across the CI matrix. CI cross-builds with
`CGO_ENABLED=0` for darwin, linux and windows, so a cgo-only SQLite driver
would break the build.

Existing data must survive schema changes. A corrupt or newer library must
never be deleted automatically.

## Decision

Use `modernc.org/sqlite`, a pure-Go `database/sql` driver, with:

- one connection per store (`SetMaxOpenConns(1)`) to serialize writes;
- WAL, `foreign_keys=ON`, `busy_timeout` and `synchronous=NORMAL` set through
  the DSN;
- an embedded, versioned migration runner (`schema_migrations`);
- additive migrations applied one transaction at a time;
- deterministic source IDs derived from kind and a credential-free identity;
- `UNIQUE(source_id, provider_id)` for source-scoped channel identity;
- no plaintext auth headers or header tokens in SQLite;
- source URLs with userinfo rejected with `ErrCredentialsUnsupported`.

The database lives at `mygo.App.Path(mygo.PathUserData)/mumu.db` and is
created with mode `0600`. The app opens it during `WhenReady`, before the
first window frame. A failure keeps the window open and shows an actionable
banner instead of exiting.

## Consequences

- No cgo and no system SQLite dependency. Binary size grows; measure it in
  the M1 baseline.
- Migrations are append-only. Destructive migrations need a separate review,
  backup and roll-forward plan.
- Favorites and recent entries survive source re-import because the source ID
  is stable when the credential-free identity is unchanged.
- Network catalog payload caching is out of scope for #6. #8/#10 may add a
  bounded cache table through a later migration; it must remain disposable
  and separate from user-owned data.
- Keychain-backed credentials are deferred until an authenticated-source
  issue needs them.
