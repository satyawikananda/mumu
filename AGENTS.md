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
- Preserve the confirmed UX: first launch opens **Discover (Indonesia selected)**, later launches offer **Watch → Resume Last Channel** if there is a valid recent channel; **never autoplay on launch**.
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
| MyGo | `github.com/egoist/mygo` 0.8.3 | Pinned; latest release at bootstrap |
| Playback engine | libmpv | Conditional native child-surface path proven in M0 |
| Go mpv binding | `gen2brain/go-mpv` 0.4.0 | Pinned/tested in standalone spike; native runtime required |
| Renderer on macOS | MyGo Metal UI + separate native OpenGL video view | Conditional M0 result; no direct Metal interop claim |
| PiP | Floating native window with one retained view/session | Conditional M0 result; lifecycle/Spaces/stacking observed on tested Mac |
| Persistence | SQLite, local only | Implemented (#6) |
| Built-in discovery | iptv-org as an input to a reviewed/authorized catalog | Planned (#8) |
| Playlist format | M3U playlist and direct supported stream URLs | Planned (#7, #9) |
| Backend / auth | None for MVP | Decided |
| Go toolchain | Follow **pinned MyGo requirement**: Go 1.27.3, the minimum MyGo 0.8.3 accepts | Verified at bootstrap (#3) |

**Important:** MyGo uses Metal on macOS, while common libmpv integration approaches expose OpenGL or software-render paths. Do not assume they are directly compatible. A PureGo/nocgo binding does **not** eliminate the dynamic native libmpv runtime dependency. Hardware decoding and zero-copy rendering are *goals*, not verified facts.

## 5. Phases and implementation gates

- **M0 — Feasibility:** [#1](https://github.com/satyawikananda/mumu/issues/1) playback embedding; [#2](https://github.com/satyawikananda/mumu/issues/2) single-session PiP. Produce reproducible **GO / CONDITIONAL GO / NO-GO** findings.
- **M1 — Foundation:** [#3–#6](https://github.com/satyawikananda/mumu/issues) bootstrap, design shell, playback controller and SQLite. Finalize commands and directory layout after #3.
- **M2 — Sources:** #7–#9 parser, reviewed catalog and multi-playlist import.
- **M3 — Product UX:** #10–#13 Discover, Watch, PiP and Library.
- **M4 — Release:** #14 packaging, licensing, resource/performance QA.
- **After v0.1:** #15 recording, #16 time-shift/DVR; neither is an initial-release requirement.

If #1 or #2 yields NO-GO, dependent production issues are **blocked**, not considered complete by swapping in a WebView or separate playback window.

## 6. Repository boundaries

Actual tree after #3:

```text
cmd/mumu/             # Application entry point: wiring and Run only
internal/app/         # Composition root, window, cancellation, background jobs
internal/ui/          # Native shell, theme and shared views
internal/player/      # Playback controller, state machine and Engine port (#5)
internal/player/libmpv/  # libmpv adapter behind the mumu_mpv build tag
internal/video/       # Native surface presentation boundary; adapter in #11
internal/catalog/     # Channel model and provider boundary; #7/#8
internal/playlist/    # M3U parsing and source classification; #7
internal/storage/     # Local repositories and migrations; #6
docs/architecture/    # ADRs, feasibility reports, evidence
docs/ux/              # Shell wireframes, states and interaction notes
experiments/          # M0 spikes and the synthetic HLS fixture
resources/            # Packaged app icon
```

`internal/player`, `video`, `catalog`, `playlist` and `storage` contain
package documentation only. Their APIs are declared by the issues that
implement them; do not invent signatures ahead of that work.

Architectural boundaries:

- UI **dispatches intents**; it does not own or call libmpv directly.
- Player controller owns **playback lifetime and state transitions**; video surface adapter owns **presentation**.
- Catalog providers yield normalized channel/source data; **same model** for built-in and user-imported playlists.
- Storage persists user intent; network catalog cache remains replaceable.
- Recording (future) is a **separate service**; do not couple recording lifetime to the currently viewed channel.

## 7. Player and PiP invariants

- Exactly **one active viewing session** and at most one current primary stream in v0.1.
- Player state must distinguish: `idle`, `loading`, `playing`, `buffering`, `error`, `stopping`, `stopped` (adapt naming only with tests/docs).
- Events from previous channel loads must never overwrite the active channel; use request/generation identifiers or equivalent cancellation rules.
- All native UI changes happen through MyGo's verified main-thread / UI-safe update mechanism. Never block the event loop on networking or decoder waits.
- PiP state should distinguish main presentation, transitioning, PiP presentation and stopped state.
- PiP **Return to Window** and **Close PiP** are separate actions. Define deterministic behavior when the main window is hidden/closed.
- Rapid channel switches, window resize, enter/leave PiP, error recovery, and app shutdown must release resources predictably.
- Do not render a fake live seek bar: ordinary HLS live streams are not necessarily seekable; time-shift is a later milestone.

## 8. Native UI and UX implementation rules

- Read `DESIGN.md` **before creating or changing UI**.
- All interface elements are MyGo native widgets or custom native drawing. Avoid browser metaphors and mock HTML compositions.
- Use tomato red as a **sparse accent**, charcoal surfaces for prolonged viewing, and semantic colors for states.
- Prefer native text rendering, accessibility labels, keyboard focus and sensible pointer targets.
- Design for small/resized windows and macOS titlebar/safe area constraints; no layout that only works at a single screenshot size.
- Differentiate: **LIVE** (stream type), **playing/buffering/error** (player state), **verified/unknown** (availability/rights checks).
- Support empty/loading/offline/error, including Retry and Add Playlist. Failed network calls must not freeze navigation.
- No welcome screen. Avoid unexpected audio on app launch.

## 9. Security, privacy and content rights

- Only include **reviewed and authorized** sources in the built-in catalog. iptv-org is a source-discovery dataset, **not** blanket content licensing.
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
  - `go tool mygo build` for a packaged macOS app.
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
- **After #4:** Reconcile UI conventions with the implemented theme and update `DESIGN.md`; replace the placeholder shell. Implemented in this change: tokens, Dark/Light/System themes, wide sidebar, 72 pt rail and shared status views.
- **After #5/#6:** Implemented: local SQLite repositories and migrations, the playback controller/state machine, and the libmpv adapter behind the `mumu_mpv` build tag. The app wires the local library at launch; Watch/PiP surface and UI wiring remain #11/#12.
- Changes to player/rendering boundaries or startup UX require updating both relevant docs and acceptance tests.

## 12. Reference material

- MyGo native UI: https://mygo.egoist.dev/docs/ui
- MyGo architecture: https://mygo.egoist.dev/docs/architecture
- MyGo getting started: https://mygo.egoist.dev/docs/getting-started
- go-mpv binding: https://github.com/gen2brain/go-mpv
- mpv libmpv manual: https://mpv.io/manual/master/#libmpv-render-api
- Project issues: https://github.com/satyawikananda/mumu/issues

**If uncertain, verify, prototype and report. Never manufacture a successful native playback result.**
