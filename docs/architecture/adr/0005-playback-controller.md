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
one event pump. The real adapter is selected by the `mumu_mpv` build tag on
`darwin/arm64`; other builds compile a stub. This avoids requiring libmpv
headers or dylibs in `go test ./...` and the CI cross-build.

Playback and rendering stay separate. A render target may be added to the
adapter or composed around it when #11 implements Watch. No WebView, external
mpv process or second decoder is introduced.

## Consequences

- Unit tests use a fake engine and require no native runtime.
- The controller is usable headlessly with `vo=null`; the smoke test proves
  load/state/volume/teardown, not visible video.
- The app does not import the adapter yet. #11 wires the controller and native
  surface through `cmd/mumu` and the app composition root.
- The `mumu_mpv` tag and external libmpv runtime are development constraints
  until packaging (#14) resolves redistribution and loader UX.
- A future render target must preserve one session and one generation stream;
  it must not call `loadfile` or create a second decoder.
