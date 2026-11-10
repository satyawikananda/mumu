# Watch experience and video presentation

Issue: [#11](https://github.com/satyawikananda/mumu/issues/11). Depends on the
[playback controller](player-controller.md), [native video surface](video-surface.md)
and the [per-source policy](adr/0007-per-source-stream-policy.md).

## Layout

```text
Watch
┌───────────────────────────────────────────────────────────┐
│ [LIVE] Channel name · playlist              [Favorites]   │
│ ┌───────────────────────────────────────────────────────┐ │
│ │ Native video box (16:9, letterboxed inside)           │ │
│ └───────────────────────────────────────────────────────┘ │
│ status · player detail                                    │
│ [Mute] Volume ────          [Channels] [PiP] [Fullscreen] │
└───────────────────────────────────────────────────────────┘
```

- The page owns the video box as a bound `ui.Handle`. It reports the committed
  box through `OnFrame`; the app converts it to a `video.Rect` and lets
  `video.Fit` letterbox 16:9 inside it.
- The native `NSOpenGLView` is a child of the window's content view, so it
  covers MyGo drawing inside that box. The placeholder states show before the
  first playback; after the surface is attached, status and errors live in the
  control strip rather than under the video.
- Controls sit in a reserved strip below the video. Overlaying MyGo controls on
  the native view is not proven and would need AppKit input/layering work.
- The channel drawer is a 280 pt list beside the video. It reuses the same
  source-scoped keys as Discover and switches channels through the same
  controller. `Enter` or a double click switches; arrows only move the choice.

## State mapping

| Player state | Watch state | Visible treatment |
|---|---|---|
| no active channel, no recent | `WatchEmpty` | Nothing playing, Browse Discover |
| no active channel, valid recent | `WatchResume` | Resume card, no autoplay |
| `loading` | `WatchLoading` | Loading status, stream not yet shown |
| `playing` | `WatchPlaying` | Minimal status text, real video |
| `buffering` | `WatchBuffering` | Buffering status, non-blocking |
| `error` | `WatchError` | Credential-free message, Retry |
| `stopping`, `stopped` | `WatchStopped` | Playback stopped, Browse Discover |

`LIVE` comes from the stream variant or the engine's `Capabilities.Live`. It
never claims that an upstream stream is online.

There is no seek bar. Live streams are not assumed seekable in v0.1.

## Resume and history

- `Recent.Last` supplies the resume card on launch. The card appears only when
  the source still exists, is enabled, and has a supported stream.
- Nothing autoplays. `Resume` calls the same play path as a channel selection.
- `Recent.MarkPlayed` runs once per playback generation, after the controller
  reports `playing`, using the stored `(sourceID, providerID)` pair.
- Favorites resolve the same stored channel identity, so a refresh keeps them.

## Threading and lifetime

- The UI only dispatches intents. `player.Controller` owns loading, generations
  and teardown; `internal/video` owns the native view and its render thread.
- Controller snapshots reach the page through `Window.Update`. The app publishes
  only presentational fields, so the 1 Hz metrics beat does not redraw the page
  every second.
- Shutdown order is fixed: `surface.Close()` then `controller.Close(ctx)`. The
  app stops playback before closing the local library.
- Renderer failures arrive on `Surface.Errors()` and become an actionable
  playback error; they never crash the app.

## Verification

Automated:

- `go test ./...` covers page states, resume, selection, favorites, volume/mute
  dispatch, drawer switching and the empty/error paths.
- `go test -race ./...`, `go vet ./...`, `go tool mygo vet .` and golangci-lint
  pass.
- Cross-builds for linux/amd64 and darwin/amd64 stay green without the native
  adapter.

Native macOS smoke test (manual, pending):

1. Build with `go build ./cmd/mumu` and run with the pinned libmpv runtime on
   `DYLD_LIBRARY_PATH` (or `MUMU_MPV_LIBRARY`). The adapter is part of the
   default `darwin/arm64` build; no build tag is needed.
2. Play a synthetic HLS fixture, then check: video and audio start, the surface
   sits in the box at 16:9, resize and fullscreen keep it aligned, rapid channel
   switches show only the newest selection, favorite and volume/mute persist,
   a dead URL shows Retry without a stale frame, and quit frees the process.
3. Record OS, architecture, libmpv version, commands and observations under
   `docs/architecture/evidence/`.

Until that run is recorded, Watch video presentation is **implemented but not
verified on device**.
