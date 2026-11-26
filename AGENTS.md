# AGENTS.md — mumu

> Repository: https://github.com/satyawikananda/mumu  
> Status: **current implementation rules**. Reconcile this file with the actual codebase when the architecture changes.
> Scope: Instructions for AI coding agents and human contributors.

## 1. Project mission

**mumu** is a lightweight, open-source, macOS-first desktop TV player. Users can discover supported public live channels or bring their own streams/playlists, watch them using a **native video player**, and keep watching in a floating **Picture-in-Picture (PiP)** window.

Product principles:

1. **Open → discover → watch**: no sign-up, mandatory onboarding, or initial setup wizard.
2. **Native first**: MyGo `ui` renders the entire application interface; there is **no WebView**.
3. **One viewing session**: Watch and PiP are two presentations of the same playback session.
4. **Progressive disclosure**: simple controls by default, advanced IPTV features available when needed.
5. **Local first**: playlists, favorites, history, and preferences are kept on-device.
6. **Evidence over assumptions**: native graphics interop, performance, and stream compatibility must be tested on macOS.

## 2. Non-negotiable constraints

### MUST

- Use **Go** for the app and **MyGo native UI (`github.com/egoist/mygo/ui`)** for all screens and widgets.
- Target **macOS first**, with Apple Silicon as the initial QA target. Keep Windows/Linux engine builds headless and testable, but do not claim video presentation or release support there without native evidence.
- Use **libmpv** for playback (the initial Go binding candidate is `github.com/gen2brain/go-mpv`), subject to the playback validation.
- Ensure Watch and PiP refer to a **single logical player session**.
- Preserve the startup UX: fresh install opens **Discover with no country selected**; later launches offer **Home → Resume** when a valid recent channel exists; **never autoplay on launch**.
- Follow [`DESIGN.md`](./DESIGN.md) for layout, visual tokens, UX states and brand treatment.
- Prefer simple, cancellable, observable Go code with bounded network and file operations.
- Use legal/authorized streams for development and testing; respect license and copyright obligations.

### MUST NOT

- Add HTML/CSS/JS UI, WKWebView, browser-based video playback, Electron, Tauri, React, or a hosted frontend as a shortcut.
- Spawn an **external mpv player window/process** as a substitute for embedding video in MyGo.
- Create a new player/decoder just to enter PiP without an explicit architectural decision and product approval.
- Treat a playlist URL as proof that its content is online, licensed, or compatible.
- Add accounts, cloud sync, backend microservices, EPG, recording, or DVR/time-shift to the initial release without the corresponding approved issue.
- Invent MyGo APIs, native interop hooks, libmpv APIs, measured FPS/RAM/CPU claims, or claim feature completeness without tests.
- Circumvent DRM, paywalls, authentication, geographic access restrictions, or stream-provider protections.

## 3. Source of truth and precedence

When documents disagree, apply this order:

1. Latest **explicit product decisions** and maintainer direction.
2. Verified behavior and constraints in [`docs/architecture/`](docs/architecture/README.md).
3. This `AGENTS.md` for engineering rules, and `DESIGN.md` for the visual/interaction contract.
4. The active issue's Goal, Description, Acceptance Criteria, and Agent Prompt.
5. Pinned dependency documentation and **actual source code**.

If the issue conflicts with a hard constraint or a proven technical limitation, **stop, document the conflict, and propose an alternative** instead of silently changing scope.

## 4. Technology decisions (current vs pending)

### Playback validation

The playback and PiP work produced **CONDITIONAL GO** findings on macOS 27.0.1 /
Apple M3. The tested bridge uses a retained NSOpenGLView beside MyGo's Metal UI,
moved between native windows with one libmpv session. No MyGo fork was needed.
A retained view and CGL context locking are part of the current adapter.
OpenGL deprecation, presentation gaps, per-process GPU attribution, broader
platform QA and runtime redistribution remain constraints. See
[Playback](docs/architecture/playback.md) and
[Validation](docs/architecture/validation.md). Do not treat these findings as
production playback/PiP completion or as proof that mpv renders directly into
MyGo Metal. The root application entry point is `cmd/mumu`; the native shell is
in `internal/`.

| Area | Direction | Status |
|---|---|---|
| Language | Go 1.27.3 | Decided, pinned in `go.mod` |
| UI | MyGo native `ui` | Decided |
| Design system | Semantic tokens and shared native components in `internal/ui` | Implemented: tokens, shell, Control Center and pages |
| Fonts | Lexend display + Inter Tight body, bundled OFL faces | Implemented; no runtime download |
| MyGo | `github.com/egoist/mygo` 0.9.0 | Pinned |
| Playback engine | libmpv | macOS arm64 presentation proven; windows/amd64 and linux/amd64 headless adapter implemented, gated CI evidence required |
| Go mpv binding | `gen2brain/go-mpv` 0.4.0, patched in `third_party/go-mpv` | Default adapter on darwin/arm64, windows/amd64 and linux/amd64; the runtime is opened explicitly, never in a package init |
| libmpv runtime | Bundled LGPL build in `Contents/Resources/libmpv` on macOS; per-platform search order reuses `MUMU_MPV_LIBRARY` and bundle directories | Implemented; a missing, wrong-architecture or incompatible runtime is an actionable error, not a panic |
| Renderer on macOS | MyGo Metal UI + separate native OpenGL video view | No direct Metal interop claim |
| Renderer on Windows/Linux | None | Headless engine only; the app refuses Play instead of playing audio or pretending video works |
| PiP | Floating native window with one retained view/session | Lifecycle/Spaces/stacking observed on the tested Mac |
| Persistence | SQLite via the MyGo plugin and its native module, local only | Implemented; native module loader since MyGo 0.9 |
| Built-in discovery | iptv-org as a labelled community list, not rights clearance | Implemented |
| Playlist format | M3U playlist and direct supported stream URLs | Implemented |
| Backend / auth | None for MVP | Decided |
| Go toolchain | Follow **pinned MyGo requirement**: Go 1.27.3, the minimum MyGo 0.9.0 accepts | Verified with the current toolchain |

**Important:** MyGo uses Metal on macOS, while common libmpv integration approaches expose OpenGL or software-render paths. Do not assume they are directly compatible. A PureGo/nocgo binding does **not** eliminate the dynamic native libmpv runtime dependency. Hardware decoding and zero-copy rendering are *goals*, not verified facts.

### libmpv runtime and the default build

The real adapter is compiled for `darwin/arm64`, `windows/amd64` and
`linux/amd64`; every other target compiles a stub, so cross-builds need no
native runtime. There is no build tag to remember, and the old `mumu_mpv` tag is
no longer used (passing it is harmless). CGO is not required: the binding is
purego.

Windows and Linux get the engine but no video presentation. There is no native
surface adapter on those targets, so the app refuses a Play request with an
actionable error before it opens a stream; it must not play audio or record
history as if watching succeeded. Headless tests use `vo=null`/`ao=null` and
prove decoding, control, headers and the media proxy. They do not prove a player
window, resizing or PiP.

The runtime is opened on the first Play, not at launch, so starting mumu on a
machine without mpv stays silent and the Watch page only reports a missing
runtime when the viewer asks to play. The packaged app prefers its bundled
runtime over any system copy. The search order is:

macOS:

1. `MUMU_MPV_LIBRARY` (a dylib file or `:`-separated directories);
2. `Contents/Resources/libmpv` and `Contents/Resources` next to the executable,
   when they already exist;
3. the dynamic loader's own search path, which honours `DYLD_LIBRARY_PATH`;
4. `/opt/homebrew/lib/libmpv.dylib`, then `/usr/local/lib/libmpv.dylib`
   (and the `.2.dylib` variant of each).

Linux (`libmpv.so.2`, then `libmpv.so`):

1. `MUMU_MPV_LIBRARY` (a file or `:`-separated directories);
2. `<executable-dir>/libmpv`, then the executable directory;
3. the dynamic loader's own search path, which honours `LD_LIBRARY_PATH`;
4. `/usr/local/lib`, `/usr/lib`, `/lib`.

Windows (`libmpv-2.dll`, `libmpv.dll`, `mpv-2.dll`, `mpv-1.dll`):

1. `MUMU_MPV_LIBRARY` (a file or `;`-separated directories); every entry must be
   absolute, and a relative entry is rejected with a clear error;
2. `<executable-dir>/libmpv`, then the executable directory.

Windows loading uses `LoadLibraryExW` with
`LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`, so a DLL's
dependencies come only from its own directory and System32. The loader never
searches the working directory or `PATH`, and the process search path is not
changed. The binding validates the binary architecture, requires every entry
point it registers, and accepts only client API major 2 with minor 1 or newer.
A failed registration releases the handle and can be retried.

The packaged macOS app therefore uses the runtime the release verified, and a
Homebrew or system copy cannot silently shadow it; a dev build without a bundle
keeps the loader-first order.

`MUMU_MPV_LIBRARY` exists because the `go` command removes `DYLD_LIBRARY_PATH`
from the environment of the binaries it runs, so `go test` cannot use the
loader's own search path. Packaging and redistribution ship a pinned LGPL
runtime, never a GPL build. See [macOS packaging](docs/release/macos.md).

```sh
# Run the app with a libmpv runtime in a non-standard directory. The go
# command drops DYLD_* from the binaries it runs, so MUMU_MPV_LIBRARY is the
# reliable choice for `go run` and `go test`.
MUMU_MPV_LIBRARY=/private/tmp/mumu-runtime/lib go run ./cmd/mumu

# Run the gated integration tests.
MUMU_MPV_LIBRARY=/private/tmp/mumu-runtime/lib MUMU_MPV_TEST=1 \
  go test -run TestIntegration -v ./internal/player/libmpv
```

### SQLite native module

Since MyGo 0.9 the SQLite plugin is a native module (`Sqlitecore`) written in
Zig, not a bare dylib loaded through `sqlite.LibraryPath`. `sqlite.LibraryPath`
and `MYGO_SQLITE_LIBRARY` no longer exist; the replacement override is
`MYGO_NATIVE_SQLITECORE`.

- `mygo dev` and `mygo build` place `libSqlitecore.dylib` among the app's
  resources at `Contents/Resources`, next to `libmpv`, and sign it with the
  app. `verify-app.sh` fails when it is missing, because the app opens its
  local library at launch.
- `go run ./cmd/mumu` and `go test ./...` download the module for the host
  platform into the user cache (`~/Library/Caches/mygo/natives` on macOS) on
  first use. That download needs network; an offline or air-gapped build must
  pre-stage the library and point `MYGO_NATIVE_SQLITECORE` at it.
- The Go API of the plugin (`Open`, `Execute`, `Query`, `Transaction`,
  `Error`) is unchanged, so `internal/storage` needs no source change for the
  upgrade. Existing databases keep their schema, favorites, recent history and
  settings.

### Native video overlay

The retained `NSOpenGLView` is inserted **under** MyGo's surface
(`NSWindowBelow` of `MyGoSurfaceView`) and the Watch window root is transparent,
so scrims, the drawer and player controls paint above live video while MyGo
keeps all pointer input. See [Video surface](docs/architecture/video-surface.md).

- `ui.NativeView` is **not** the production path for the video: MyGo gives a
  placed native view the pointer across its whole box, which swallows clicks on
  chrome drawn over it.
- `video.Surface` exposes `NativeViewHandle` and `SyncDrawable` for a host that
  lays the view out itself. The app must call `SyncDrawable` after layout, a
  window move or a scale change.
- `libmpv.Engine.SetPaused` is the product pause path. Pause holds
  the displayed frame and stops audio; the adapter also sets
  `demuxer-readahead-secs=0`, but a live stream keeps a small residual read
  (playlist refresh and in-flight segments). Do not claim a silent network or a
  zero read-ahead; measure and document it. `Resume` reloads at the live edge,
  never seeks to the paused position.
- `SetReadAhead` remains an experiment seam. Do not detach live video to show an
  overlay, and do not add a second view, decoder or player for PiP. See
  [Validation](docs/architecture/validation.md).

## 5. Delivery gates

- **Playback feasibility:** native embedding and single-session PiP must have a
  recorded GO, CONDITIONAL GO or NO-GO result before production UI depends on
  them.
- **Headless engine (#54):** `windows/amd64` and `linux/amd64` must pass the
  native loader, ABI, header and address-policy tests on their own runners
  before the adapter is claimed. Headless success does not satisfy the macOS
  presentation gate, and no Windows/Linux video presentation exists yet.
- **Foundation:** bootstrap, native shell, playback controller and SQLite.
- **Sources:** M3U parser, bounded network policy, community catalog and source
  management.
- **Product UX:** Discover, Home, Watch, PiP and Library.
- **Release:** packaging, licensing, resource checks and manual macOS QA.
- **UI foundation:** MyGo upgrade, native overlay gate, tokens and shared
  components, Control Center, page integration, accessibility and release QA.

Playlist collections remain v0.2.0. Recording and time-shift stay after v0.1.
If playback feasibility yields NO-GO, dependent production work is **blocked**,
not completed by swapping in a WebView or a separate playback window.

## 6. Repository boundaries

Repository tree:

```text
cmd/mumu/             # Application entry point: wiring and Run only
internal/app/         # Composition root, window, cancellation, background jobs
internal/ui/          # Native shell, semantic tokens, fonts, icons and shared components
internal/player/      # Playback controller, state machine and Engine port
internal/player/libmpv/  # libmpv adapter (darwin/arm64, windows/amd64, linux/amd64) and runtime discovery
internal/player/libmpv/library/  # libmpv search order, testable without a dylib
third_party/go-mpv/   # pinned binding, patched for an explicit runtime loader
internal/video/       # Native surface presentation boundary
internal/catalog/     # Channel domain, identity, validation and provider boundary
internal/netguard/    # Bounded HTTP client and untrusted-URL policy
internal/playlist/    # M3U parsing, content detection and source loading
internal/sources/     # Catalog-to-storage mapping, built-in catalog and import service
internal/logos/       # Bounded logo fetch, decode, cache and dominant artwork tint
internal/storage/     # Local repositories and migrations
docs/architecture/    # System design and validation notes
docs/ux/              # Shell and page behavior; screenshots stay out of git
docs/release/         # Packaging, testing and release checklist
resources/            # Packaged icon, licenses, third-party notices
packaging/            # LGPL runtime build and macOS release gates
tools/hlsfixture/     # Synthetic HLS fixture for local playback tests
```

`internal/player`, `video`, `catalog`, `netguard`, `playlist`, `sources` and
`storage` declare their own public interfaces. Do not invent signatures ahead of
that work.

Architectural boundaries:

- UI **dispatches intents**; it does not own or call libmpv directly.
- Player controller owns **playback lifetime and state transitions**; video surface adapter owns **presentation**.
- Catalog providers yield normalized channel/source data; **same model** for built-in and user-imported playlists.
- Storage persists user intent; network catalog cache remains replaceable.
- Recording (future) is a **separate service**; do not couple recording lifetime to the currently viewed channel.

## 7. Player and PiP invariants

- Exactly **one active viewing session** and at most one current primary stream in v0.1.
- Player state distinguishes: `idle`, `loading`, `playing`, `buffering`,
  `paused`, `error`, `stopping`, `stopped` (adapt naming only with tests/docs).
  Recovery is bounded to three attempts with an 8-second no-progress deadline and
  an 8-second floor between attempt starts; variants stay inside one channel.
- Events from previous channel loads must never overwrite the active channel; use request/generation identifiers or equivalent cancellation rules.
- All native UI changes happen through MyGo's verified main-thread / UI-safe update mechanism. Never block the event loop on networking or decoder waits.
- PiP state should distinguish main presentation, transitioning, PiP presentation and stopped state.
- PiP **Close/Esc stops playback**; **Return/P** moves the same session to the main window and preserves pause. PiP mute is temporary and restores the main audio intent on Return. The panel is frameless 16:9 with hover chrome and per-display geometry.
- Rapid channel switches, window resize, enter/leave PiP, error recovery, and app shutdown must release resources predictably.
- Do not render a fake live seek bar: ordinary HLS live streams are not necessarily seekable; time-shift is a later milestone.

## 8. Native UI and UX implementation rules

- Read `DESIGN.md` **before creating or changing UI**.
- All interface elements are MyGo native widgets or custom native drawing. Avoid browser metaphors and mock HTML compositions.
- Use semantic tokens from `internal/ui/tokens.go`: charcoal surfaces, sparse `#F82028` brand accent, `brand-fill` for fills behind white text, and a separate keyboard ring (`#FFFFFF` dark, `#18181B` light).
- Use Lexend for display and Inter Tight for body/controls, both bundled under `internal/ui/fonts/` with OFL licenses. Fall back to the system font if registration fails.
- Prefer native text rendering, accessibility labels, keyboard focus and sensible pointer targets. Hover never draws the focus ring.
- Design for the three reference sizes with `layoutFor(width, height)`: 1470x956, 1280x720 and 1024x640. Read both width and height; no layout that only works at one screenshot size.
- Use the shared components in `internal/ui` for tiles, rows, control cards, badges, chips, keycaps and status states. Do not fork a second visual vocabulary per screen.
- Keep the permanent sidebar removed. Control Center is an overlay, not a destination; opening it must not detach the native video surface or change the current page. Settings and category rails are screen-local.
- Differentiate: **LIVE** (stream type), **playing/buffering/error** (player state), **verified/unknown** (availability/rights checks).
- Support empty/loading/offline/error, including Retry and Add Playlist. Failed network calls must not freeze navigation.
- No welcome screen. Avoid unexpected audio on app launch.

## 9. Security, privacy and content rights

- Only include **reviewed and authorized** sources in the built-in catalog. iptv-org is a source-discovery dataset, **not** blanket content licensing.
- The iptv-org exception is recorded in [Sources](docs/architecture/sources.md): it is labelled a community list, never verified or cleared, filtered by the local blocklist and the upstream NSFW/DMCA blocklist, and can be hidden in Settings. This is risk reduction, not a rights review.
- Custom playlist URLs are user-provided; do not advertise that every imported stream is legal or accessible.
- Remote source loading requires timeouts, maximum response sizes, redirect/scheme controls, and cancellation. Do not automatically fetch untrusted local/private-network addresses referenced by remote catalogs.
- Use user intent for local file or LAN imports; avoid leaking local files, private playlist credentials or sensitive headers into logs.
- Prefer Keychain or another justified system credential store if authenticated sources ever become necessary.
- Cache only needed metadata; do not proxy/re-host third-party video or silently upload watch history.
- Document libmpv/FFmpeg build-time and redistribution licensing; the Go binding license does not cover native media libraries.
- Recording/time-shift must be opt-in and authorized before implementation; no access-control bypass.

## 10. Engineering workflow per issue

Before coding:

1. Read this file, `DESIGN.md` if UI-related, the active issue, and the relevant architecture docs.
2. Inspect the real repository tree and **pinned** MyGo/mpv APIs; do not rely on remembered signatures.
3. Identify dependencies, blocking assumptions, test strategy and out-of-scope work.
4. For uncertain native calls, first create a minimal reproducible experiment (or a small failing test).

While coding:

- Make the smallest coherent change for that issue. Avoid speculative abstractions, huge refactors or editing unrelated files.
- Use idiomatic Go, explicit errors with context (`fmt.Errorf("...: %w", err)`), bounded goroutines and cancellable contexts for I/O.
- Add table-driven tests for parser/storage logic; fake adapters for player state and network failures; real macOS smoke tests for rendering/PiP.
- Separate test fixtures from copyrighted streams. No hard-coded non-licensed channel URLs.
- Keep logs useful but redact URLs containing credentials and authentication headers.

Before claiming completion:

- Run the project commands. These are authoritative:
  - `go tool mygo dev` for a live-reloading native run.
  - `go test ./...`; use `go test -race ./...` before delivery (race needs cgo).
  - `go vet ./...` and `go tool mygo vet .` for MyGo UI lifetime checks.
  - `gofmt -l ./cmd ./internal third_party/go-mpv` (must print nothing).
  - `golangci-lint run ./cmd/... ./internal/...` when the tool is installed, or `make lint`.
  - `cd third_party/go-mpv && go vet ./... && go test ./...` because it is a separate module.
  - `make package` for the full release: LGPL runtime, `go tool mygo build`,
    and `packaging/macos/verify-app.sh`.
  - On a machine with libmpv, `MUMU_MPV_LIBRARY=<dir>` plus `MUMU_MPV_TEST=1`/`MUMU_MPV_HEADER_TEST=1` for the gated `internal/player/libmpv` integration tests. A skip is not a pass.
  - `MUMU_MPV_LIBRARY=<dir> MUMU_MPV_INCLUDE=<dir> bash packaging/qa/test-headless.sh` for the loader, ABI, header and address-policy evidence. It fails when a required test is skipped.
- For native media work, record **OS, architecture, MyGo/libmpv versions, command, observable output, and tested stream type**.
- Report a clear summary: changed files, tests actually run, constraints met, unverified items, and follow-up risks.
- Do not mark acceptance criteria complete based solely on static code review or compilation if runtime video/PiP is required.

### Suggested PR / agent delivery format

```markdown
## Goal achieved
...
## Implementation
...
## Verification
- Commands + results
- Manual macOS steps + observed behavior
## Risks / Not verified
...
## Issue acceptance criteria
- [ ] / [x] with evidence
```

## 11. Documentation lifecycle

- Keep [`docs/architecture/`](docs/architecture/README.md) accurate when module
  boundaries, playback, storage, source policy or platform constraints change.
- Keep [`docs/ux/`](docs/ux/shell.md) accurate when layout, navigation, states or
  interaction change.
- Keep [`docs/release/`](docs/release/README.md) accurate when packaging,
  testing or release steps change.
- Keep [`DESIGN.md`](DESIGN.md) as the visual and interaction contract.
- Changes to player or rendering boundaries, startup UX or the public UI require
  updating both the relevant doc and its tests.
- Record validation results in [`docs/architecture/validation.md`](docs/architecture/validation.md).
  Do not claim a manual or native result that was not run.

## 12. Reference material

- MyGo native UI: https://mygo.egoist.dev/docs/ui
- MyGo architecture: https://mygo.egoist.dev/docs/architecture
- MyGo getting started: https://mygo.egoist.dev/docs/getting-started
- go-mpv binding: https://github.com/gen2brain/go-mpv
- mpv libmpv manual: https://mpv.io/manual/master/#libmpv-render-api
- Project issues: https://github.com/satyawikananda/mumu/issues

**If uncertain, verify, prototype and report. Never manufacture a successful native playback result.**
