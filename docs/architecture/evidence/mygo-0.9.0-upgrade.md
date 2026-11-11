# MyGo 0.9.0 upgrade evidence

Issue: [#31](https://github.com/satyawikananda/mumu/issues/31). Tracker:
[#30](https://github.com/satyawikananda/mumu/issues/30).

Recorded 12 November 2026, Asia/Makassar.

## Scope

Upgrade the pinned MyGo from 0.8.3 to 0.9.0 without touching playback, storage
semantics or view design. libmpv, the patched `third_party/go-mpv` binding, the
single-session invariant and the macOS packaging pipeline stay as they are.
MyGo `ui/video` is not imported and no second decoder was added.

## Environment

| Item | Value |
| --- | --- |
| Machine | Apple M3, 8 GB (same release machine as the #14 QA matrix) |
| OS | macOS 27.0.1 |
| Go | 1.27.3 |
| MyGo before | 0.8.3 |
| MyGo after | 0.9.0 |
| go-mpv | 0.4.0, vendored in `third_party/go-mpv` |
| libmpv runtime | 0.42.0, bundled LGPL build |

MyGo 0.9.0 still declares `go 1.27.3`, so the Go toolchain requirement did not
change.

## Change

- `go.mod` pins `github.com/egoist/mygo v0.9.0`.
- `go mod tidy` adds six indirect modules that the pinned `mygo` CLI needs:
  `go.digitalxero.dev/rpm`, `github.com/ProtonMail/go-crypto`,
  `github.com/cloudflare/circl`, `github.com/klauspost/compress`,
  `github.com/ulikunitz/xz`, `golang.org/x/crypto`. They are build-time tool
  dependencies and are not linked into the app.
- No file under `internal/` or `cmd/` changed.

The only breaking change in the 0.9.0 release notes is the SQLite loader:
`sqlite.LibraryPath` and `MYGO_SQLITE_LIBRARY` were removed in favour of the
`Sqlitecore` native module and the `MYGO_NATIVE_SQLITECORE` override. The Go
API used by `internal/storage` (`Open`, `Execute`, `Query`, `Transaction`,
`Error`) is unchanged.

## Source audit for 0.9 UI lifetimes

| Check | Result |
| --- | --- |
| `go tool mygo migrate-ui .` preview | no diff; the app already uses checked `ui.Element` values and fluent constructor keys |
| Elements stored in app state | none; the only `ui.Element` outside a view body is a `ui.DialogBase` callback parameter |
| Zero/nil element comparisons | none |
| Persistent focus identity | `ui.Handle` in the Watch view (`video`, `overlay`) |
| Derived bound values | Library tabs, Settings appearance, Discover region/category/source and Library add-playlist inputs read a local copy, then commit with `Changed()` in the same pass, which 0.9 supports |
| Shortcut scoping | no `OnShortcut` or `c.OnShortcut` usage yet |
| `Window.Update` dispatch | `internal/app/dispatch.go` publishes on the UI thread; unchanged |
| `go tool mygo vet .` | pass |

## Automated checks

Run from the repository root after the bump.

| Command | Result |
| --- | --- |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go tool mygo vet .` | pass |
| `go tool mygo migrate-ui .` | pass, no diff |
| `gofmt -l ./cmd ./internal` | no output |
| `go test ./...` | pass, every package |
| `CGO_ENABLED=1 go test -race ./...` | pass, every package |
| `GOOS=darwin GOARCH=amd64 go build ./...` | pass |
| `GOOS=linux GOARCH=amd64 go build ./...` | pass |
| `GOOS=windows GOARCH=amd64 go build ./...` | pass |
| `make package` | pass: runtime stage, `go tool mygo build`, `verify-app.sh` |
| `packaging/macos/verify-app.sh` | `OK ... (mumu 0.1.0, 13 libraries)` |

`golangci-lint` is not installed on this machine, so `make lint` was not run
locally. The CI `lint` job covers it.

### Packaging gate

`verify-app.sh` now fails when `Contents/Resources/libSqlitecore.dylib` is
missing, because the app opens its local library during `WhenReady`. The gate
was proven in both directions:

- packaged build passes with the module present;
- a copy of the bundle with the module moved aside fails with
  `verify-app: missing bundled SQLite module: .../Contents/Resources/libSqlitecore.dylib`.

The bundled module is 1,970,160 bytes, arm64, `minos` 12.0 (below the app's
13.0 minimum) and depends only on `/usr/lib/libSystem.B.dylib`.

## Measured size

| Artifact | MyGo 0.8.3 | MyGo 0.9.0 |
| --- | --- | --- |
| `Mumu.app/Contents/MacOS/Mumu` | 14,094,224 bytes | 14,297,920 bytes |
| `Mumu.app` | 41.5 MB | 41.5 MB |
| DMG | 14.2 MB | 14.2 MB |

## Existing database

The upgrade ran against the maintainer's pre-existing library at
`~/Library/Application Support/Mumu/mumu.db`; nothing was reset or migrated
by hand.

| Item | Before launch | After launch |
| --- | --- | --- |
| Sources | 12 | 12 |
| Channels | 848 | 848 |
| Favorites | 3 | 3 |
| Recent channels | 25 | 25 |
| `schema_migrations` | 1, 2, 3 | 1, 2, 3 |

The 0.9.0 build created `mumu.db-wal` and `mumu.db-shm`, which shows it opened
the database through the bundled native module with WAL enabled.

## Manual macOS smoke

Run by the maintainer on 12 November 2026 against the packaged build from
`make package` (`build/darwin-arm64/Mumu.app`, MyGo 0.9.0, bundled LGPL libmpv
0.42.0, Apple M3, macOS 27.0.1). The app opened the existing library before the
run and kept it afterwards.

| Step | Result |
| --- | --- |
| Play a channel from Home/Discover | pass: video and audio |
| Rapid channel switch | pass: only the newest selection plays |
| Enter PiP, then Return/P | pass: the same session continues in the main window |
| Change volume or mute in PiP, then Return | pass: main audio intent is restored |
| Close PiP while playing, reopen, quit, relaunch | pass: favorites, recent history and settings persist |

Still not re-run in this session: resize and fullscreen alignment, the dead-URL
error panel, the buffering badge, repeated PiP toggles, Spaces and
foreign-fullscreen, Cmd+Q while PiP is open, and sleep/wake. Those stay
unverified and are listed below.

## Not verified

- Intel Macs, other macOS versions and non-Apple-Silicon Apple hardware.
- Linux and Windows runtime behaviour; only cross-compilation was checked.
- The M0 native playback and PiP findings still apply unchanged; this file does
  not claim production playback completion.
- `ui/video` behaviour, since it is out of scope for the upgrade.
