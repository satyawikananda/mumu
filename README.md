# mumu

A lightweight, local-first TV player for macOS. Discover supported public
channels or bring your own streams, watch them in a native window, and keep
watching in a floating Picture-in-Picture window.

mumu is written in Go and draws its entire interface with
[MyGo](https://mygo.egoist.dev) native UI. It has no WebView, no HTML or
JavaScript frontend, and no hosted backend.

**Status:** M1 foundation. The native shell, design tokens and Dark/Light/System themes are implemented; playback, discovery, playlists, the library and PiP are not implemented yet. The player and PiP paths are conditional on the M0 findings in [`docs/architecture/`](docs/architecture/).

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
internal/app/         composition root: window, cancellation, background jobs
internal/ui/          native MyGo shell, theme and shared views
internal/player/      playback boundary; libmpv adapter arrives with #5
internal/video/       native video surface boundary; adapter with #5/#11
internal/catalog/     channel model and provider boundary (#7, #8)
internal/playlist/    M3U parsing and source classification (#7)
internal/storage/     local SQLite repositories (#6)
docs/architecture/    feasibility reports, evidence and ADRs
docs/ux/              shell wireframes, states and interaction notes
experiments/          M0 playback/PiP spikes and the test stream fixture
```

Only `cmd/mumu` is packaged into the application. The packages under
`internal/` currently declare boundaries with package documentation; their
APIs arrive with the issues above rather than being invented up front.

Native-only rules and the engineering workflow are in
[`AGENTS.md`](AGENTS.md). The visual and interaction contract is in
[`DESIGN.md`](DESIGN.md).

## Content rights

Built-in discovery must use reviewed and authorized sources. iptv-org is a
source-discovery dataset, not blanket content licensing. User-supplied
playlists are the user's responsibility, and mumu does not bypass DRM,
paywalls or geographic restrictions.

The M0 spikes use only an original synthetic HLS fixture generated locally.
