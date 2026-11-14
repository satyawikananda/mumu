# Mumu TV

A lightweight, local-first TV player for macOS. Discover supported public
channels or bring your own streams, watch them in a native window, and keep
watching in a floating Picture-in-Picture window.

Mumu TV is written in Go and draws its entire interface with
[MyGo](https://mygo.egoist.dev) native UI. It has no WebView, no HTML or
JavaScript frontend, and no hosted backend.

**Status:** native shell, local SQLite library, playback controller, M3U/catalog
pipeline, per-source network policy, Discover, Watch, PiP and Settings are
implemented. The macOS app bundles a pinned LGPL libmpv runtime. Playback and PiP
remain conditional on the tested configuration; see
[`docs/architecture/validation.md`](docs/architecture/validation.md) for verified
behavior and known gaps. The documentation index is
[`docs/README.md`](docs/README.md).

## Requirements

- macOS 13 or later. Development testing used macOS 27.0.1, Apple Silicon.
- Go 1.27.3 or later, as pinned in `go.mod`. MyGo 0.9.0 requires it.
- A libmpv runtime for playback. Release builds bundle an LGPL one inside the
  app, so nothing has to be installed to watch; running from source needs a
  local runtime (`brew install mpv`, or `MUMU_MPV_LIBRARY=<dir>`). The app and
  every non-playback screen start without it.

Check the toolchain with:

```sh
go tool mygo doctor
```

## Install

Download the Apple Silicon (arm64) disk image from
[GitHub Releases](https://github.com/satyawikananda/mumu/releases). The DMG
already contains the libmpv runtime. v0.1 has no Developer ID and no
notarization, so macOS asks for approval the first time you open Mumu TV; that is
expected for an unsigned open-source app, not a sign of a damaged download.

1. Download `mumu-<version>-arm64.dmg` and `SHA256SUMS` from the same release.
2. Verify the disk image:

   ```sh
   shasum -a 256 -c SHA256SUMS --ignore-missing
   ```

3. Open the disk image and drag `Mumu` into `Applications`.
4. Try to open Mumu TV once, then dismiss the warning.
5. Open System Settings > Privacy & Security, click **Open Anyway** next to
   Mumu TV, and confirm.

Right-click > Open no longer bypasses this reliably on macOS 15 and later. If
you already verified the checksum, you can clear the quarantine flag instead
and open the app normally:

```sh
xattr -dr com.apple.quarantine /Applications/Mumu.app
```

Do not disable Gatekeeper globally. Releases are arm64 only; Intel Macs are not
supported in v0.1.

## Develop

```sh
go tool mygo dev            # build, run and live-reload the native app
go test ./...               # unit tests, no window needed
go vet ./...
go tool mygo vet .          # MyGo native UI lifetime checks
```

`mygo dev` creates a development bundle named `Mumu Dev` with the
identifier `dev.satyawikananda.mumu.dev`, separate from an installed app.

### libmpv for source builds (developer-only)

A release bundles its own LGPL libmpv runtime in `Contents/Resources/libmpv`,
so an installed app needs nothing from Homebrew. Only a build made from source
needs a local runtime; the rest of this section does not apply to an installed
app.

The default build includes the libmpv adapter. Playback opens the runtime on the
first Play, in this order: `MUMU_MPV_LIBRARY`, the loader's own search path
(`DYLD_LIBRARY_PATH`), `/opt/homebrew/lib`, `/usr/local/lib`, then
`Contents/Resources/libmpv` and `Contents/Resources` next to the executable.

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

Without a runtime, Mumu TV starts normally and Watch reports the missing runtime
when you press Play, with a Retry action after you install it.

## Build

A release build bundles the LGPL runtime. The first run builds it from pinned
sources; later runs reuse `build/runtime`.

```sh
brew install meson pkg-config cmake ninja   # maintainer tools, once
make package                          # runtime + app + DMG + verification
```

`go tool mygo build` alone still works, but it only includes the runtime after
`make runtime` has staged it in `resources/darwin-arm64/libmpv`. The macOS
application and disk image are written to `build/`. See
[macOS packaging](docs/release/macos.md) for the gate and the pinned versions.

## Structure

```text
cmd/mumu/             application entry point
internal/app/         composition root: window, cancellation, background jobs, local library
internal/ui/          native MyGo shell, theme and shared views
internal/player/      playback controller, state machine and Engine port
internal/player/libmpv/  libmpv adapter, the default darwin/arm64 build
internal/player/libmpv/library/  libmpv runtime search order
third_party/go-mpv/   pinned go-mpv, patched for an explicit runtime loader
internal/video/       retained native video surface
internal/catalog/     channel domain and provider boundary
internal/catalog/iptvorg/  per-country community catalog and blocklist
internal/netguard/    bounded HTTP client and untrusted-URL policy
internal/playlist/    M3U parsing, content detection and source loading
internal/sources/     catalog/storage mapping, built-in catalog and source service
internal/logos/       bounded logo fetch, decode and cache
internal/storage/     local SQLite repositories and migrations
docs/architecture/    system design and validation notes
docs/ux/              shell and page behavior
docs/release/         packaging, testing and release checklist
packaging/macos/      LGPL runtime build, closure rewrite and bundle gate
resources/            app icon, third-party notices and license texts
tools/hlsfixture/     synthetic HLS fixture for local playback tests
```

Only `cmd/mumu` is packaged into the application. The local library lives at
`~/Library/Application Support/Mumu/mumu.db`. The libmpv adapter is part of the
default `darwin/arm64` build and is opened on the first Play. A release ships
its runtime in `Contents/Resources/libmpv`; a source build needs a local one and
reports an actionable error when it is missing. See
[playback](docs/architecture/playback.md).

Native-only rules and the engineering workflow are in
[`AGENTS.md`](AGENTS.md). The visual and interaction contract is in
[`DESIGN.md`](DESIGN.md).

## Content rights

Channels from iptv-org are presented as a **community list**, not a cleared or
verified catalog. The data project is CC0; the streams belong to their owners.
Discover labels the list, a local blocklist and the upstream iptv-org
NSFW/DMCA exclusions remove entries, and a setting can hide the community
catalog entirely. See [Sources](docs/architecture/sources.md).

User-supplied playlists are the user's responsibility. Mumu TV does not bypass
DRM, paywalls or geographic restrictions, and it does not re-host or proxy
third-party video. Local playback tests use an original synthetic HLS fixture
generated locally.
