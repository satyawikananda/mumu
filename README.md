# mumu

A lightweight, local-first TV player for macOS. Discover supported public
channels or bring your own streams, watch them in a native window, and keep
watching in a floating Picture-in-Picture window.

mumu is written in Go and draws its entire interface with
[MyGo](https://mygo.egoist.dev) native UI. It has no WebView, no HTML or
JavaScript frontend, and no hosted backend.

**Status:** M2 sources. The native shell, design tokens and Dark/Light/System themes are implemented. Local SQLite stores, the playback controller and the M3U/catalog pipeline are implemented. Discover shows the cached community catalog with search, category and source filters; Library manages multiple playlists and custom streams. Watch and PiP screens remain #11/#12. The player and PiP paths remain conditional on the M0 findings in [`docs/architecture/`](docs/architecture/).

## Requirements

- macOS 12 or later. M0 was verified on macOS 27.0.1, Apple Silicon.
- Go 1.27.3 or later, as pinned in `go.mod`. MyGo 0.8.3 requires it.
- No Bun, Node.js or libmpv for the bootstrap shell. libmpv becomes a
  runtime requirement when playback lands in issue #5.

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
internal/player/libmpv/  libmpv adapter behind the mumu_mpv build tag
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
`~/Library/Application Support/Mumu/mumu.db`. The libmpv adapter requires the
`mumu_mpv` build tag and an external runtime; see
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
