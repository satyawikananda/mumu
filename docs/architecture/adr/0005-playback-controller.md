# ADR 0005: Playback controller and engine boundary

- Status: accepted
- Date: 2026-11-09
- Issues: #5, #11

## Context

M0 proved one libmpv session rendering into a retained native OpenGL view.
The production app still has no Watch screen or video surface. Issue #5 needs
reusable playback control, not a second renderer or an external player.

Per AGENTS.md, UI dispatches intents and never calls libmpv. The player
controller owns playback lifetime and state transitions; the video surface
adapter owns presentation. Rapid channel switching must not let stale events
overwrite the newest channel.

## Decision

`internal/player` contains:

- a comparable `Snapshot` state machine;
- a bounded intent queue with newest-play-wins coalescing;
- generation identifiers on every intent and engine event;
- a loading timeout and bounded teardown;
- an `Engine` port that knows nothing about MyGo.

`internal/player/libmpv` implements `Engine` with one pinned go-mpv client and
one event pump. The real adapter is part of the default `darwin/arm64` build;
other targets compile a stub. The binding is patched in `third_party/go-mpv` so
that libmpv is opened explicitly on the first Play instead of in a package
`init()` that panicked when the dylib was missing. This keeps `go test ./...`
and the CI cross-build free of libmpv headers and dylibs, and turns a missing
runtime into an error the UI can explain.

Playback and rendering stay separate. A render target may be added to the
adapter or composed around it when #11 implements Watch. No WebView, external
mpv process or second decoder is introduced.

## Consequences

- Unit tests use a fake engine and require no native runtime.
- The controller is usable headlessly with `vo=null`; the smoke test proves
  load/state/volume/teardown, not visible video.
- The app does not import the adapter yet. #11 wires the controller and native
  surface through `cmd/mumu` and the app composition root.
- The `mumu_mpv` tag is gone: the adapter is the default `darwin/arm64` build,
  and the runtime is loaded on the first Play (see the amendment below).
- A future render target must preserve one session and one generation stream;
  it must not call `loadfile` or create a second decoder.

## Amendment (10 November 2026): lazy runtime loading

The `mumu_mpv` gate made the default build report "native playback
unavailable" and log a warning at launch, while the tagged build still panicked
inside `go-mpv`'s package `init()` on a machine without libmpv. Both problems
came from the binding, not the controller:

- `third_party/go-mpv` is v0.4.0 with `mpv.Load(candidates)` replacing the
  init-time `dlopen`, an `mpv_client_api_version` check and no cgo backend.
- `internal/player/libmpv/library` searches `MUMU_MPV_LIBRARY`, the loader path,
  `/opt/homebrew/lib`, `/usr/local/lib`, then `Contents/Resources`.
- The app creates the engine, controller and surface on the first Play, so a
  launch without playback stays silent and Watch reports the missing runtime
  only when the viewer asks to play.

The controller boundary and one-session/generation rules are unchanged.
