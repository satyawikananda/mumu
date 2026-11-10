# Release QA matrix

Issue: [#14](https://github.com/satyawikananda/mumu/issues/14). Packaging
notes: [macos](macos.md). Evidence:
[release-2026-11-10](../architecture/evidence/release-2026-11-10/).

Log files named below are local run artifacts; `.gitignore` excludes `*.log`.
The commands are the reproducible record.

## Test machine

| Item | Value |
| --- | --- |
| Model | MacBook Air Mac15,12 |
| Chip | Apple M3, 8 cores (4P + 4E) |
| Memory | 8 GB |
| OS | macOS 27.0.1, build 26A434 |
| Go | 1.27.3 |
| MyGo | 0.8.3 |
| libmpv | 0.42.0, LGPL build |
| FFmpeg | 9.0.2, LGPL build |
| libplacebo | 7.387.0 |
| Compiler | Apple clang 15.0.0, Command Line Tools 15.3 |

Results apply to this machine only. They are not a claim about other Apple
Silicon models, Intel Macs, or other macOS versions.

## Automated checks

| Check | Command | Result | Evidence |
| --- | --- | --- | --- |
| Unit tests | `go test ./...` | pass | `go-test.log` |
| Race tests | `go test -race ./...` | pass | `go-test-race.log` |
| Vet | `go vet ./...` | pass | `go-vet.log` |
| MyGo lifetime vet | `go tool mygo vet .` | pass | `mygo-vet.log` |
| Formatting | `gofmt -l ./cmd ./internal` | no output | `gofmt.log` |
| Lint | `golangci-lint run ./cmd/... ./internal/...` | 0 issues | `golangci-lint.log` |
| Bundle gate | `packaging/macos/verify-app.sh` | pass, 13 libraries | `verify-app.log` |
| Ad-hoc signature | `codesign -dv Mumu.app` | ad hoc, no Team ID | `codesign.log` |
| Gatekeeper | `spctl -a -vv Mumu.app` | rejected, expected | `spctl.log` |
| Runtime load | gated libmpv test, bundled runtime | pass | `libmpv-720.log` |
| HLS 720p | fixture, bundled runtime | pass | `libmpv-720.log` |
| HLS 1080p | fixture, bundled runtime | pass | `libmpv-1080.log` |
| Engine soak, 720p, 30 s | `MUMU_SOAK_SECONDS=30` | pass | `soak-720-30s.log` |
| Engine soak, 1080p, 180 s | `MUMU_SOAK_SECONDS=180` | pass | `soak-1080-180s.log` |
| Bundle search without Homebrew | test binary inside app, sandbox denies `/opt/homebrew` | pass | `bundle-sandbox.log` |
| App launch without Homebrew | app binary, same sandbox, 5 seconds | stayed alive, no sandbox errors | `app-sandbox.log`, `app-sandbox-status.txt` |
| Idle UI sample | `packaging/qa/sample.sh <pid> 20`, 12 s warm-up | 80.7 MiB mean RSS, 3.1% mean CPU; one run, not a baseline | `idle/summary.txt`, `idle/samples.csv` |

`golangci-lint` was not installed on this machine, so the lint gate was not
run locally. CI runs it.

## Package output

| Item | Value |
| --- | --- |
| `Mumu.app` | 39.8 MiB |
| `mumu-0.1.0-arm64.dmg` | 13.7 MiB |
| SHA-256 | `a51eee589de43079db0043937192009791cc570976a6729043fe64d5820caefa` |
| Runtime libraries | 13, 25.3 MiB |
| Architectures | arm64 only |

The DMG was mounted and the app inside passed signature verification. DMG bytes
are not reproducible between builds; verify the hash published with the
release, not a hash copied from another run.

## Manual checks

Maintainer-confirmed on 10 November 2026:

- Watch plays with video and audio.
- PiP enters, returns and closes without breaking the session.
- Some channels in the community list do not play. Their `m3u8` addresses are
  unavailable at the source; that is a catalog/stream availability issue, not
  evidence that the player is broken.

Still not run: 60-minute soak, rapid PiP toggles, sleep/wake, external
display, network transitions, invalid/oversized playlists, corrupted database,
and the Gatekeeper flow from a downloaded release.

## Playback checks

The 720p and 1080p cases use the synthetic local HLS fixture from
`experiments/hlsfixture`, not a third-party channel. They exercise libmpv
demuxing, decoding and the controller state machine with `vo=null` and
`ao=null`; they do not exercise the native video surface or speaker output.

Observed: load, playing state, volume and mute, stop, and clean teardown all
pass. The runtime now tears down in under a second; an earlier build with the
Cocoa backend hung in `mpv_terminate_destroy`, which is why that backend is
disabled.

### Decoder

This LGPL build has no VideoToolbox interop, so `hwdec=auto-safe` resolves to
software decoding. This is a known difference from the M0 Homebrew runtime,
which hardware-decoded. The app still reports `hwdec-current`; no hardware
decoding claim is made for this release.

## Not verified

These were not run and must not be reported as passing:

- Full app Watch playback with the native video surface and audible audio.
- PiP enter/return/close, rapid toggles, and window cleanup.
- 60-minute continuous playback at 720p and 1080p.
- Sleep/wake, display sleep, Spaces, minimize/hide/close, app quit during
  playback.
- External display.
- Network disconnect and reconnect.
- Channel switching in the app, 30 rapid switches.
- Invalid and oversized M3U, insecure or private URLs, unavailable streams,
  corrupted database, read-only database directory.
- Downloading the DMG from a published release and walking through the
  Gatekeeper flow.
- CPU, RAM, GPU and energy metrics for active playback at 720p, 1080p and
  PiP. The idle sample above is the only resource measurement recorded.
- A clean second Mac with no Homebrew installed.

The sandbox run and the closure check are strong evidence that the bundled
runtime does not need Homebrew, but they are not a substitute for a clean
machine.

## How to finish the matrix

1. Install the built DMG into `/Applications`, launch it, and play a permitted
   HLS channel.
2. Run `packaging/qa/sample.sh <pid> 3600 build/qa/soak720` for a 720p run and
   repeat at 1080p. Record `summary.txt` and `samples.csv`.
3. Exercise PiP, switching, sleep/wake, external display and network loss by
   hand; record observed behavior and timestamps.
4. Fetch the release DMG with `curl`, verify `SHA256SUMS`, attach quarantine
   with `xattr -w com.apple.quarantine 0081`, and confirm the Open Anyway flow.
5. Add the results to this file, including the exact commands and any failure.
