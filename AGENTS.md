# AGENTS.md — mumu

> Repository: https://github.com/satyawikananda/mumu  
> Status: **v0.1 / pre-implementation rules**. Reconcile this file with the actual codebase after issue #3.  
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
- Target **macOS first**, with Apple Silicon as the initial QA target. Keep future Windows/Linux adapters possible without claiming they work today.
- Use **libmpv** for playback (the initial Go binding candidate is `github.com/gen2brain/go-mpv`), subject to the results of issue #1.
- Ensure Watch and PiP refer to a **single logical player session**.
- Preserve the M5 UX: fresh install opens **Discover with no country selected**; later launches offer **Home → Resume** when a valid recent channel exists; **never autoplay on launch**.
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

1. Latest **explicit product decisions** in accepted project issues / maintainer direction.
2. Verified results and ADRs from [#1 — native playback](https://github.com/satyawikananda/mumu/issues/1) and [#2 — PiP feasibility](https://github.com/satyawikananda/mumu/issues/2).
3. This `AGENTS.md` for engineering rules, and `DESIGN.md` for the visual/interaction contract.
4. The specific issue's Goal, Description, Acceptance Criteria, and Agent Prompt.
5. Pinned dependency documentation and **actual source code**.

If the issue conflicts with a hard constraint or a proven technical limitation, **stop, document the conflict, and propose an alternative** instead of silently changing scope.

## 4. Technology decisions (current vs pending)

### M0 evidence (9 November 2026)

The standalone spikes produced **CONDITIONAL GO** findings on macOS 27.0.1 / Apple M3:
[native playback](docs/architecture/native-playback-feasibility.md) and
[single-session PiP](docs/architecture/pip-feasibility.md).
The tested bridge uses a retained NSOpenGLView beside MyGo's Metal UI, moved between native windows with one libmpv session.
No MyGo fork was needed. Follow-up QA records capture-based audio/video measurements, system-wide GPU observations, and maintainer-confirmed drag, stacking, Spaces and foreign-fullscreen behavior on this Mac.
OpenGL deprecation, capture/presentation gaps, per-process GPU attribution, broader platform QA and runtime redistribution remain constraints.
The bridge now serializes rendering and drawable changes with both a Go mutex and native CGL context locking after capture exposed a native crash.
Do not treat these findings as production playback/PiP completion or as proof that mpv renders directly into MyGo Metal.
The spike commands live in [its README](experiments/nativeplayback/README.md). The root application was bootstrapped in #3: `cmd/mumu` is the entry point and the native shell is in `internal/`.

| Area | Direction | Status |
|---|---|---|
| Language | Go 1.27.3 | Decided, pinned in `go.mod` |
| UI | MyGo native `ui` | Decided |
| Design system | Semantic tokens and shared native components in `internal/ui` | Foundation #33; shell/Control Center #34; page integration #35–#40 |
| Fonts | Lexend display + Inter Tight body, bundled OFL faces | Implemented #33; no runtime download |
| MyGo | `github.com/egoist/mygo` 0.9.0 | Pinned; upgraded in #31 for the M5 revamp |
| Playback engine | libmpv | Conditional native child-surface path proven in M0 |
| Go mpv binding | `gen2brain/go-mpv` 0.4.0, patched in `third_party/go-mpv` | Default darwin/arm64 adapter; the runtime is opened explicitly, never in a package init |
| libmpv runtime | Bundled LGPL build in `Contents/Resources/libmpv`; discovered at runtime through `MUMU_MPV_LIBRARY`, the loader's own search path (`DYLD_LIBRARY_PATH`), `/opt/homebrew/lib`, `/usr/local/lib`, then `Contents/Resources/libmpv` and `Contents/Resources` | Implemented (#14); a missing runtime is an actionable error, not a panic |
| Renderer on macOS | MyGo Metal UI + separate native OpenGL video view | Conditional M0 result; no direct Metal interop claim |
| PiP | Floating native window with one retained view/session | Conditional M0 result; lifecycle/Spaces/stacking observed on tested Mac |
| Persistence | SQLite via the MyGo plugin and its native module, local only | Implemented (#6, refactor); native module loader since (#31) |
| Built-in discovery | iptv-org as a labelled community list, not rights clearance | Implemented (#8) |
| Playlist format | M3U playlist and direct supported stream URLs | Implemented (#7, #9) |
| Backend / auth | None for MVP | Decided |
| Go toolchain | Follow **pinned MyGo requirement**: Go 1.27.3, the minimum MyGo 0.9.0 accepts | Verified at bootstrap (#3) and the #31 upgrade |

**Important:** MyGo uses Metal on macOS, while common libmpv integration approaches expose OpenGL or software-render paths. Do not assume they are directly compatible. A PureGo/nocgo binding does **not** eliminate the dynamic native libmpv runtime dependency. Hardware decoding and zero-copy rendering are *goals*, not verified facts.

### libmpv runtime and the default build

The real adapter is compiled by the default `darwin/arm64` build; every other
target compiles a stub, so cross-builds need no native runtime. There is no
build tag to remember, and the old `mumu_mpv` tag is no longer used (passing it
is harmless). CGO is not required: the binding is purego.

The runtime is opened on the first Play, not at launch, so starting mumu on a
machine without mpv stays silent and the Watch page only reports a missing
runtime when the viewer asks to play. The search order is:

1. `MUMU_MPV_LIBRARY` (a dylib file or `:`-separated directories);
2. the dynamic loader's own search path, which honours `DYLD_LIBRARY_PATH`;
3. `/opt/homebrew/lib/libmpv.dylib`, then `/usr/local/lib/libmpv.dylib`
   (and the `.2.dylib` variant of each);
4. `Contents/Resources/libmpv` and `Contents/Resources` next to the executable,
   when they already exist.

`MUMU_MPV_LIBRARY` exists because the `go` command removes `DYLD_LIBRARY_PATH`
from the environment of the binaries it runs, so `go test` cannot use the
loader's own search path. Packaging and redistribution are implemented in
#14: the app ships a pinned LGPL runtime, never a GPL build. See
[ADR 0008](docs/architecture/adr/0008-bundled-libmpv-runtime.md) and
[macOS packaging](docs/release/macos.md).

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

ADR 0009 decides how MyGo chrome meets live video. The retained `NSOpenGLView`
is inserted **under** MyGo's surface (`NSWindowBelow` of `MyGoSurfaceView`) and
the Watch window root is transparent, so scrims, the drawer and player controls
paint above live video while MyGo keeps all pointer input.

- `ui.NativeView` is **not** the production path for the video: MyGo gives a
  placed native view the pointer across its whole box, which swallows clicks on
  chrome drawn over it.
- `video.Surface` exposes `NativeViewHandle` and `SyncDrawable` for a host that
  lays the view out itself. The app must call `SyncDrawable` after layout, a
  window move or a scale change.
- `libmpv.Engine.SetPaused` is the product pause path (ADR 0010). Pause holds
  the displayed frame and stops audio; the adapter also sets
  `demuxer-readahead-secs=0`, but a live stream keeps a small residual read
  (playlist refresh and in-flight segments). Do not claim a silent network or a
  zero read-ahead; measure and document it. `Resume` reloads at the live edge,
  never seeks to the paused position.
- `SetReadAhead` remains an experiment seam. Do not detach live video to show an
  overlay, and do not add a second view, decoder or player for PiP. Evidence:
  `docs/architecture/evidence/overlay-2026-11-12/`.

## 5. Phases and implementation gates

- **M0 — Feasibility:** [#1](https://github.com/satyawikananda/mumu/issues/1) playback embedding; [#2](https://github.com/satyawikananda/mumu/issues/2) single-session PiP. Produce reproducible **GO / CONDITIONAL GO / NO-GO** findings.
- **M1 — Foundation:** [#3–#6](https://github.com/satyawikananda/mumu/issues) bootstrap, design shell, playback controller and SQLite. Finalize commands and directory layout after #3.
- **M2 — Sources:** #7–#9 parser, reviewed catalog and multi-playlist import.
- **M3 — Product UX:** #10–#13 Discover, Watch, PiP and Library.
- **M4 — Release:** #14 packaging, licensing, resource/performance QA.
- **M5 — UI revamp:** [#30](https://github.com/satyawikananda/mumu/issues/30) tracker. #31 MyGo 0.9.0, #32 overlay gate, #33 tokens/components, #34 Control Center, #35 Home, #36 Discover, #37 playback controls, #38 Watch, #39 PiP, #40 Settings, #41 accessibility/release QA. Playlist collections remain v0.2.0.
- **After v0.1:** #15 recording, #16 time-shift/DVR; neither is an initial-release requirement.

If #1 or #2 yields NO-GO, dependent production issues are **blocked**, not considered complete by swapping in a WebView or separate playback window.

## 6. Repository boundaries

Actual tree after #3:

```text
cmd/mumu/             # Application entry point: wiring and Run only
internal/app/         # Composition root, window, cancellation, background jobs
internal/ui/          # Native shell, semantic tokens, fonts, icons and shared components
internal/player/      # Playback controller, state machine and Engine port (#5)
internal/player/libmpv/  # libmpv adapter (default on darwin/arm64) and runtime discovery
internal/player/libmpv/library/  # libmpv search order, testable without a dylib
third_party/go-mpv/   # pinned binding, patched for an explicit runtime loader
internal/video/       # Native surface presentation boundary; adapter in #11
internal/catalog/     # Channel domain, identity, validation and provider boundary; #7/#8
internal/netguard/    # Bounded HTTP client and untrusted-URL policy; #7
internal/playlist/    # M3U parsing, content detection and source loading; #7
internal/sources/     # Catalog-to-storage mapping, built-in catalog and import service; #8/#9
internal/logos/       # Bounded logo fetch, decode, cache and dominant artwork tint; #8/#33
internal/storage/     # Local repositories and migrations; #6
docs/architecture/    # ADRs, feasibility reports, evidence
docs/ux/              # Shell, library and M5 revamp handoff; screenshots and evidence
experiments/          # M0 spikes and the synthetic HLS fixture
resources/            # Packaged icon, licenses, third-party notices
packaging/            # LGPL runtime build and macOS release gates (#14)
docs/release/         # Packaging notes, QA matrix and release checklist
```

`internal/player`, `video`, `catalog`, `netguard`, `playlist`, `sources` and
`storage` declare the APIs their implementing issues own. Do not invent
signatures ahead of that work.

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
- PiP **Close/Esc stops playback**; **Return/P** moves the same session to the main window and preserves pause. PiP mute is temporary and restores the main audio intent on Return. Implemented in #39 with a frameless 16:9 panel, hover chrome and per-display geometry.
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
- ADR 0006 records the accepted iptv-org exception: it is labelled a community list, never verified or cleared, filtered by the local blocklist and the upstream NSFW/DMCA blocklist, and can be hidden in Settings. This is risk reduction, not a rights review.
- Custom playlist URLs are user-provided; do not advertise that every imported stream is legal or accessible.
- Remote source loading requires timeouts, maximum response sizes, redirect/scheme controls, and cancellation. Do not automatically fetch untrusted local/private-network addresses referenced by remote catalogs.
- Use user intent for local file or LAN imports; avoid leaking local files, private playlist credentials or sensitive headers into logs.
- Prefer Keychain or another justified system credential store if authenticated sources ever become necessary.
- Cache only needed metadata; do not proxy/re-host third-party video or silently upload watch history.
- Document libmpv/FFmpeg build-time and redistribution licensing; the Go binding license does not cover native media libraries.
- Recording/time-shift must be opt-in and authorized before implementation; no access-control bypass.

## 10. Engineering workflow per issue

Before coding:

1. Read this file, `DESIGN.md` if UI-related, the issue, and relevant spike/ADR conclusions.
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

- Run the project commands. These are authoritative after #3:
  - `go tool mygo dev` for a live-reloading native run.
  - `go test ./...`; use `go test -race ./...` before delivery (race needs cgo).
  - `go vet ./...` and `go tool mygo vet .` for MyGo UI lifetime checks.
  - `gofmt -l ./cmd ./internal` (must print nothing).
  - `golangci-lint run ./cmd/... ./internal/...` when the tool is installed, or `make lint`.
  - `make package` for the full release: LGPL runtime, `go tool mygo build`,
    and `packaging/macos/verify-app.sh`.
  - On a machine with libmpv, `MUMU_MPV_LIBRARY=<dir>` plus `MUMU_MPV_TEST=1`/`MUMU_MPV_HEADER_TEST=1` for the gated `internal/player/libmpv` integration tests. A skip is not a pass.
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

- **Now:** #3 bootstrapped the repository. Commands, tree and pinned versions above are verified; see the ADRs in `docs/architecture/adr/`.
- **After #1/#2:** M0 feasibility reports and evidence are recorded under `docs/architecture/`.
- **After #3:** Completed: `cmd/mumu`, `internal/app`, native shell, CI, README and ADRs.
- **After #4:** Reconcile UI conventions with the implemented theme and update `DESIGN.md`; replace the placeholder shell. Implemented in this change: tokens, Dark/Light/System themes, wide sidebar, 72 pt rail and shared status views. Historical: M5 #34 replaces the sidebar/rail with Control Center; do not reintroduce it.
- **After #5/#6:** Implemented: local SQLite repositories and migrations, the playback controller/state machine, and the libmpv adapter. The app wires the local library at launch; Watch/PiP surface and UI wiring remain #11/#12. The adapter is now the default `darwin/arm64` build with an explicit, lazy runtime loader (`third_party/go-mpv`), so no build tag and no launch-time playback error.
- **After #7/#8/#9:** Implemented: M3U domain/parser with bounded network policy, variant streams, the iptv-org community catalog with cache/blocklist/attribution, source management (import, preview, atomic refresh, toggle, remove), and native Discover/Library source UI. See `docs/architecture/sources.md` and ADR 0006.
- **After #10/#11a/#11b:** Implemented: Discover search, category/source filters and channel selection; per-source private-host consent and scheme allowlist (ADR 0007); Watch with one shared controller, native video surface, resume, favorites, volume/mute and explicit error states. See `docs/architecture/watch.md` and `docs/architecture/video-surface.md`.
- **After #12:** Implemented: PiP as a presentation change of the same surface and session, with a native floating panel, rollback on a failed attach, Return/Close behavior and unit-tested rapid toggles. The manual macOS Watch and PiP smoke tests are still pending. See `docs/architecture/pip.md`.
- **After #13:** Implemented: `internal/library` for favorites, recent viewing and the resume reference; Library with Favorites, Recently watched and Playlists tabs; one source-scoped favorite cache shared by Watch, Discover and Library; provider-ID remapping on refresh in `internal/storage`. See `docs/architecture/library.md` and `docs/ux/library.md`.
- **v0.1.0 release blockers:** the native view now follows the playback state in both windows (attached only for `playing`/`buffering`, detached for loading/error/stopped so the glass overlay is visible); returning from PiP recreates a closed main window by native pointer identity; a failed switch offers Back to the previous channel; a launch always starts unmuted while the last volume is restored. PiP behavior therefore changed from #12. See `docs/architecture/watch.md`, `video-surface.md` and `pip.md`.
- **After #31 (M5 revamp baseline):** MyGo upgraded to 0.9.0 with the same Go 1.27.3 toolchain. No view code changed: the app already used checked `ui.Element` values, `ui.Handle` identity and same-pass `Changed` commits, and `go tool mygo migrate-ui .` reports no diff. SQLite now loads through the `Sqlitecore` native module; `verify-app.sh` gates `Contents/Resources/libSqlitecore.dylib`. Evidence: `docs/architecture/evidence/mygo-0.9.0-upgrade.md`.
- **After #32 (M5 revamp, overlay gate):** CONDITIONAL GO for composing MyGo chrome above the retained libmpv view by inserting the view under MyGo's surface. Recorded: live video with clickable chrome, one session through PiP transfer, paused frame with audio stopped. Not recorded: radius clipping, PiP drag/resize details, repeated toggles, dead-stream recovery. See [ADR 0009](docs/architecture/adr/0009-native-video-overlay.md).
- **After #33 (M5 design foundation):** semantic Dark/Light/System tokens, Lexend + Inter Tight bundled fonts, Lucide subset, responsive layout policy, shared native components and a portable handoff under `docs/ux/revamp/`. Home/Discover/Control Center page integration remains #34–#36. Evidence: `docs/ux/revamp/` and `internal/ui/*_test.go`.
- **After #34 (M5 native Control Center):** the permanent sidebar/rail is removed. A top bar exposes Control Center and Settings; Control Center overlays the current page with real context cards and five approved icons. Home is a distinct route and startup contract; Watch remains the only native video page. One shortcut registry feeds handlers, hints and Settings. Evidence: `docs/ux/shell.md` and shell/attach tests.
- **After #35 (M5 Home feed):** Home is a data-backed feed built from `internal/library` through the shared `libraryCache`: synthesized hero art, explicit Resume, Recent and Favorites rails, counts and `See all`. Compact keeps both rails and scrolls (approved deviation from `R7ZZ2`). Library Favorites/Recently Watched use the shared `ChannelRow` with bounded artwork. Resume resolves its key at click time, so disabled or removed sources never play. Evidence: `docs/ux/home.md`, Home capture test and app feed tests.
- **After #36 (M5 Discover):** a fresh install opens the approved no-country state; `sources.Service.Region` no longer defaults to Indonesia and `BuiltinSource`/`RefreshBuiltin` reject an empty region with `ErrNoRegion` before any network call. Country metadata and imported sources may load, but no country playlist is fetched until the viewer chooses. Discover adds a category rail beside the channel grid (a third at wide, 236 pt at medium, chips at compact), a native search field, a Country filter with a searchable country modal, infinite scrolling in batches of 40 rows with a loading skeleton, and the virtualized row list for search. Discover tiles show LIVE as a tag attached to the top-right corner; Home rails keep the bottom-left badge. The source and Live only chips were removed as no longer relevant. A saved country is kept on restart; an unknown saved code returns to the no-country state with an explicit message instead of being replaced. Evidence: `docs/ux/discover.md`, discover capture tests and app/source tests.
- Changes to player/rendering boundaries or startup UX require updating both relevant docs and acceptance tests.
- **After #37 (live pause, recovery and quality):** `player` gains `StatePaused`, `PlayRequest`, `Pause`, `Resume`, `SelectQuality`, episode identity, bounded recovery and truthful variant metadata; libmpv bounds read-ahead while paused. Resume reloads at the live edge. See [ADR 0010](docs/architecture/adr/0010-live-pause-and-recovery.md).
- **After #38 (immersive Watch):** the retained video view is inserted under `MyGoSurfaceView`, Watch is full-bleed with scrims, a non-interactive live line, a 420 pt channel drawer, real quality selection and 3-second idle chrome; the system pointer hides with the chrome and a pointer move restores both, while shell chrome stays hidden on that route. See [Watch](docs/architecture/watch.md).
- **After #39 (PiP):** the floating panel is frameless, 16:9, remembers its frame per display, keeps temporary mute local, stops on Close/Esc and preserves pause on Return. See [PiP](docs/architecture/pip.md).

## 12. Reference material

- MyGo native UI: https://mygo.egoist.dev/docs/ui
- MyGo architecture: https://mygo.egoist.dev/docs/architecture
- MyGo getting started: https://mygo.egoist.dev/docs/getting-started
- go-mpv binding: https://github.com/gen2brain/go-mpv
- mpv libmpv manual: https://mpv.io/manual/master/#libmpv-render-api
- Project issues: https://github.com/satyawikananda/mumu/issues

**If uncertain, verify, prototype and report. Never manufacture a successful native playback result.**
