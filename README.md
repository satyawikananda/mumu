# mumu

A lightweight, local-first TV player for macOS. Discover supported public
channels or bring your own streams, watch them in a native window, and keep
watching in a floating Picture-in-Picture window.

mumu is written in Go and draws its entire interface with
[MyGo](https://mygo.egoist.dev) native UI. It has no WebView, no HTML or
JavaScript frontend, and no hosted backend.

**Status:** M3 core UX in progress. The native shell, design tokens and Dark/Light/System themes are implemented. Local SQLite stores, the playback controller, the M3U/catalog pipeline and the per-source network policy are implemented. Discover shows the cached community catalog with search, category and source filters. Watch wires one shared player to a native surface, with explicit resume, favorites, volume/mute and error states. Picture-in-Picture moves the same surface into a floating native window. Native playback and PiP remain conditional on the M0 findings in [`docs/architecture/`](docs/architecture/); the manual macOS Watch and PiP smoke tests are still pending.

## Requirements

- macOS 12 or later. M0 was verified on macOS 27.0.1, Apple Silicon.
- Go 1.27.3 or later, as pinned in `go.mod`. MyGo 0.8.3 requires it.
- libmpv for playback (`brew install mpv`, or `MUMU_MPV_LIBRARY=<dir>`). The
  app and every non-playback screen start without it.

Check the toolchain with:

```sh
go tool mygo doctor
```

## Develop

```sh
go tool mygo dev            # build, run and live-reload the native app
go test ./...               # unit tests, no window needed
go vet ./...
go tool mygo vet .          # MyGo native UI lifetime checks
```

`mygo dev` creates a development bundle named `Mumu Dev` with the
identifier `dev.satyawikananda.mumu.dev`, separate from an installed app.

### Watching with libmpv

The default build includes the libmpv adapter. Playback opens the runtime on the
first Play, in this order: `MUMU_MPV_LIBRARY`, the loader's own search path
(`DYLD_LIBRARY_PATH`), `/opt/homebrew/lib`, `/usr/local/lib`, then
`Contents/Resources` next to the executable.

```sh
brew install mpv                                        # the usual install
go build -o /tmp/mumu ./cmd/mumu                        # go run drops DYLD_*
DYLD_LIBRARY_PATH=/path/to/lib /tmp/mumu                # a runtime elsewhere
MUMU_MPV_LIBRARY=/path/to/lib go run ./cmd/mumu         # same, survives go run
```

The `go` command removes `DYLD_LIBRARY_PATH` from the binaries it runs, so use
`MUMU_MPV_LIBRARY` for `go test`:

```sh
MUMU_MPV_LIBRARY=/private/tmp/mumu-runtime/lib MUMU_MPV_TEST=1 \
  go test -run TestIntegration -v ./internal/player/libmpv
```

Without a runtime, mumu starts normally and Watch reports the missing runtime
when you press Play, with a Retry action after you install it.

## Build

```sh
go tool mygo build
```

The macOS application and disk image are written to `build/`.

## Structure

```text
cmd/mumu/             application entry point
internal/app/         composition root: window, cancellation, background jobs, local library
internal/ui/          native MyGo shell, theme and shared views
internal/player/      playback controller, state machine and Engine port
internal/player/libmpv/  libmpv adapter, the default darwin/arm64 build
internal/player/libmpv/library/  libmpv runtime search order
third_party/go-mpv/   pinned go-mpv, patched for an explicit runtime loader
internal/video/       native video surface boundary; adapter with #11
internal/catalog/     channel domain and provider boundary (#7, #8)
internal/catalog/iptvorg/  per-country community catalog and blocklist
internal/netguard/    bounded HTTP client and untrusted-URL policy (#7)
internal/playlist/    M3U parsing, content detection and source loading (#7)
internal/sources/     catalog/storage mapping, built-in catalog and source service (#8, #9)
internal/logos/       bounded logo fetch, decode and cache (#8)
internal/storage/     local SQLite repositories and migrations (#6)
docs/architecture/    feasibility reports, evidence and ADRs
docs/ux/              shell wireframes, states and interaction notes
experiments/          M0 playback/PiP spikes and the test stream fixture
```

Only `cmd/mumu` is packaged into the application. The local library lives at
`~/Library/Application Support/Mumu/mumu.db`. The libmpv adapter is part of the
default `darwin/arm64` build and needs an installed libmpv runtime; it is
opened on the first Play and reports an actionable error when missing. See
[playback controller](docs/architecture/player-controller.md).

Native-only rules and the engineering workflow are in
[`AGENTS.md`](AGENTS.md). The visual and interaction contract is in
[`DESIGN.md`](DESIGN.md).

## Content rights

Channels from iptv-org are presented as a **community list**, not a cleared or
verified catalog. The data project is CC0; the streams belong to their owners.
Discover labels the list, a local blocklist and the upstream iptv-org
NSFW/DMCA exclusions remove entries, and a setting can hide the community
catalog entirely. See
[ADR 0006](docs/architecture/adr/0006-iptv-org-discovery.md).

User-supplied playlists are the user's responsibility. mumu does not bypass
DRM, paywalls or geographic restrictions, and it does not re-host or proxy
third-party video. The M0 spikes use only an original synthetic HLS fixture
generated locally.
