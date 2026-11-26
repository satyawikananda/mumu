# Testing and release

This page combines the repeatable QA matrix and the release checklist. Commands
are the record; local log files stay out of git.

## Test machine

| Item | Value |
|---|---|
| Hardware | MacBook Air, Apple M3 |
| Memory | 8 GB |
| OS | macOS 27.0.1 |
| Go | 1.27.3 |
| MyGo | 0.9.0 |
| libmpv | 0.42.0, LGPL build |
| FFmpeg | 9.0.2, LGPL build |
| Video output | retained OpenGL view |

Results apply to this machine only. They are not a claim about other Apple
Silicon models, Intel Macs or other macOS versions.

## Automated checks

| Check | Command |
|---|---|
| Unit tests | `go test ./...` |
| Race tests | `go test -race ./...` |
| Vet | `go vet ./...` |
| MyGo lifetime vet | `go tool mygo vet .` |
| Formatting | `gofmt -l ./cmd ./internal third_party/go-mpv` |
| Vendored binding | `cd third_party/go-mpv && go vet ./... && go test ./...` |
| Lint | `golangci-lint run ./cmd/... ./internal/...` |
| Headless engine and loader | `MUMU_MPV_LIBRARY=<dir> MUMU_MPV_INCLUDE=<dir> bash packaging/qa/test-headless.sh` |
| Bundle | `make package` |
| Bundle gate | `packaging/macos/verify-app.sh` |
| Cross-build | `GOOS=<os> GOARCH=<arch> CGO_ENABLED=0 go build ./...` |

`golangci-lint` may be absent on a maintainer machine; CI runs it. A skipped
integration test is not a passing result. `packaging/qa/test-headless.sh` fails
when a required native test is skipped or does not run.

The Linux AMD64 and Windows AMD64 headless jobs live in
`.github/workflows/headless.yml`. They build the pinned LGPL runtime from source,
verify its SHA-256 pins, run the required native tests and upload the evidence
artifacts. The macOS ARM64 job runs in `.github/workflows/ci.yml` after
`make package`.

## Manual QA

### Playback and PiP

- Play a permitted live channel: video and audio start unmuted with the saved
  volume.
- Pause holds the frame, stops audio and shows PAUSED. Resume reconnects at the
  live edge without a seek claim.
- Kill the source: bounded retries show an honest countdown; exhaustion offers
  Retry and does not say retrying.
- Enter and leave PiP repeatedly. Confirm one session, one retained view and one
  floating window.
- Close the main window during PiP, then Return. Confirm the main window opens,
  video returns and audio continues.
- Close PiP with no main window: playback stops and the app quits once.
- Quit with PiP open: every window closes and the process exits.
- Move, resize, hide/minimize, switch Spaces and test another app's fullscreen.
- Force a dead stream in PiP: the stale frame disappears and the error is
  readable.

### Watch and chrome

- Leave the pointer still for three seconds while playing: chrome and pointer
  hide together. Move the pointer to restore both.
- Paused, loading, error, drawer, quality menu and focused chrome states never
  hide.
- Open the channel drawer and quality menu over live video: the stream is not
  reloaded and controls stay interactive.
- Confirm the drawer follows the active channel's source, not the last Discover
  country.
- Check Back history, brand-to-Home and the icon-only Control Center action.

### Discover, Library and Settings

- Fresh install opens Discover with no country. No country playlist is fetched
  before a choice.
- Choose a country, scroll the category rail and channel grid, then search. Both
  scroll areas stay bounded.
- Import an M3U URL, a local file and a direct stream. Check preview warnings and
  atomic refresh.
- Toggle favorites, verify the same state in Discover, Watch and Library.
- Click a Library row, then click outside. Only the clicked row may stay active,
  and an outside press clears it.
- Check Dark, Light and System appearance, Start muted and About metrics.
- Run the manual update check. It opens a release page only when one exists.

### Packaging and distribution

- Install the built DMG into `/Applications`, launch it and play a permitted HLS
  channel.
- Run `packaging/qa/sample.sh <pid> 3600 build/qa/soak720` for a soak run and
  repeat at 1080p.
- Fetch the release DMG with `curl`, verify `SHA256SUMS`, attach quarantine with
  `xattr -w com.apple.quarantine 0081` and confirm the Open Anyway flow.
- Confirm a clean second Mac without Homebrew can launch the app.

## Known gaps

- No recorded 60-minute soak at 720p and 1080p.
- No per-process GPU, energy or thermal attribution.
- Sleep/wake, multi-display and network transitions are not established.
- The bundled runtime has no VideoToolbox interop and decodes in software.
- Downloaded-DMG Gatekeeper flow and a clean second Mac are not recorded.
- Full screen-reader, contrast and reduced-motion review remains open.

## Release checklist

### Before tagging

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `go tool mygo vet .`
- [ ] `gofmt -l ./cmd ./internal third_party/go-mpv` prints nothing
- [ ] `cd third_party/go-mpv && go vet ./... && go test ./...`
- [ ] `MUMU_MPV_LIBRARY=<runtime> MUMU_MPV_INCLUDE=<headers> bash packaging/qa/test-headless.sh`
- [ ] `golangci-lint run ./cmd/... ./internal/...`
- [ ] `make package`
- [ ] `packaging/macos/verify-app.sh` passes
- [ ] `Contents/Resources/libSqlitecore.dylib` exists next to `libmpv`
- [ ] `spctl -a` rejects the unsigned app, as expected
- [ ] Manual QA items that can run are recorded
- [ ] `THIRD_PARTY_NOTICES.md` matches the runtime manifest
- [ ] `packaging/macos/mpv-runtime.json` hashes still match
- [ ] No Homebrew path appears in `otool -L`
- [ ] The DMG mounts and the app inside passes `codesign --verify --deep --strict`

### Artifacts

- [ ] `mumu-<version>-arm64.dmg`
- [ ] `mumu-libmpv-<version>-arm64.tar.gz`
- [ ] `mumu-libmpv-<version>-arm64-src.tar.gz`
- [ ] `SHA256SUMS`
- [ ] `latest.json`
- [ ] `THIRD_PARTY_NOTICES.md`

### Release notes

- [ ] Version and minimum macOS 13.0
- [ ] Apple Silicon only; Intel is not supported in v0.1
- [ ] Unsigned: System Settings > Privacy & Security > Open Anyway
- [ ] SHA-256 verification instructions
- [ ] LGPL runtime source link
- [ ] Known limitation: no VideoToolbox interop in this runtime
- [ ] Known limitation: no notarization
