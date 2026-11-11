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
│ │ glass panel while loading or after a failure          │ │
│ └───────────────────────────────────────────────────────┘ │
│ status · player detail                                    │
│ [Mute] Volume ────          [Channels] [PiP] [Fullscreen] │
└───────────────────────────────────────────────────────────┘
```

- The page owns the video box as a bound `ui.Handle`. It reports the committed
  box through `OnFrame`; the app converts it to a `video.Rect` and lets
  `video.Fit` letterbox 16:9 inside it. `ResetFrame` makes a recreated window
  report the box again instead of reusing a stale one.
- The native `NSOpenGLView` is a child of the window's content view, so it sits
  above MyGo drawing inside that box. It is therefore attached **only while the
  player presents frames**; every other state detaches it (see below).
- Controls sit in a reserved strip below the video. Overlaying MyGo controls on
  the native view is not proven and would need AppKit input/layering work.
- The channel drawer is a 280 pt list beside the video. It reuses the same
  source-scoped keys as Discover and switches channels through the same
  controller. `Enter` or a double click switches; arrows only move the choice.

## Surface visibility

The app calls `syncSurfaceVisibility` inside the `publishWatch` closure, on the
UI thread, **before** `SetState`, so an overlay is never painted under a view
that is still attached. `surface.Attach` rejects non-main-thread calls, which is
why it runs there.

| Playback state | Native view |
|---|---|
| `playing` | attached to the presenting window |
| `buffering` | attached: the last frame stays, the badge is in the strip below |
| `loading`, `error`, `stopping`, `stopped`, idle | detached |

- A main-window presentation needs `a.win != nil` and Watch as the current page: a
  stream that starts playing while Discover or Library is open must not cover
  it. A PiP presentation needs the floating window instead.
- Loading and error draw a `glass.Glass` panel **inside** the stage box. The
  glass samples the charcoal stage, never a live frame, because the view is
  already gone.
- `buffering` stays attached on purpose: its spinner and detail live in the
  control strip, not in a full overlay, so dropping the last frame would only
  flash black.
- Leaving Watch detaches the view; returning re-attaches it without a stream
  reload. Moving the view to PiP clears the main-window attachment flag, so the
  navigation path cannot knock the floating player's drawable out.

## State mapping

| Player state | Watch state | Visible treatment | Native view |
|---|---|---|---|
| no active channel, no recent | `WatchEmpty` | Nothing playing, Browse Discover | detached |
| no active channel, valid recent | `WatchResume` | Resume card, no autoplay | detached |
| `loading` | `WatchLoading` | Glass loading panel in the stage | detached |
| `playing` | `WatchPlaying` | Minimal status text, real video | attached |
| `buffering` | `WatchBuffering` | Strip badge, last frame kept | attached |
| `error` | `WatchError` | Glass error panel, Retry and Back | detached |
| `stopping`, `stopped` | `WatchStopped` | Playback stopped, Browse Discover | detached |

`LIVE` comes from the stream variant or the engine's `Capabilities.Live`. It
never claims that an upstream stream is online.

There is no seek bar. Live streams are not assumed seekable in v0.1.

## A failed switch and the way back

A playback error has two actions, and they are different:

- **Retry** reloads the channel that failed.
- **Back to *channel*** appears only when a *different* channel really presented
  frames before the switch. The app remembers the previous channel, its stream
  and its source name in `playStream`, before the new channel replaces the
  active one. A channel that never played, the same channel, or a state that
  never showed video (`loading`, `error`) is not remembered.
- Returning restores that channel, its stream variant and its source, clears the
  remembered entry and plays it through the same controller.

## Audio policy

- Every launch starts **unmuted**. Mute is a session toggle: it changes the
  running player and is never written as a launch preference.
- The last **volume** is restored. A missing or unusable `playback.volume` value
  falls back to `player.DefaultVolume`; the zero value of an unset field is
  never used as a launch volume.
- The effective volume is also passed to the native engine configuration, not
  only to the controller snapshot: libmpv's own configuration defaults to
  volume `0`, which would play silently while the page shows another number.
- Older releases wrote `playback.muted`. It is read once at load only to delete
  it, so a stored `true` is not inherited.
- Evidence: the app logs one `debug` line at the first `playing` state of each
  generation with the effective `volume`, `muted`, `audio_output` and
  `video_output`, and the libmpv adapter logs the volume and mute it handed to
  mpv. No URL, header or credential is logged.

## Resume and history

- `Recent.Last` supplies the resume card on launch. The card appears only when
  the source still exists, is enabled, and has a supported stream.
- Nothing autoplays. `Resume` calls the same play path as a channel selection.
- `Recent.MarkPlayed` runs once per playback generation, after the controller
  reports `playing`, using the stored `(sourceID, providerID)` pair.
- Favorites resolve the same stored channel identity, so a refresh keeps them.
  The key includes the source ID: two playlists that publish the same provider
  ID keep independent favorite state.

## Threading and lifetime

- The UI only dispatches intents. `player.Controller` owns loading, generations
  and teardown; `internal/video` owns the native view and its render thread.
- Controller snapshots reach the page through `Window.Update`. The app publishes
  only presentational fields, so the 1 Hz metrics beat does not redraw the page
  every second.
- `publishWatch` runs `syncSurfaceVisibility` and the two `SetState` calls in
  one UI-thread closure, with the drawable change first. The page state and the
  native view can never disagree for a painted frame.
- Shutdown order is fixed: `surface.Close()` then `controller.Close(ctx)`. The
  app stops playback before closing the local library.
- Renderer failures arrive on `Surface.Errors()` and become an actionable
  playback error; they never crash the app. `failWatch` can run on that
  goroutine, so it posts navigation and the state update in order through
  `Window.Update` instead of writing shell state directly. `publish` also
  invalidates the PiP window after the callback, because the main window's own
  repaint stops once it is destroyed.

## Verification

Automated:

- `go test ./...` covers page states, resume, selection, favorites, volume/mute
  dispatch, drawer switching and the empty/error paths.
- Surface rules are covered by `TestSurfaceAttachesOnlyWhilePresenting`,
  `TestSurfaceDetachesWhenPlaybackLeavesVideo`,
  `TestPublishWatchDetachesBeforeSettingState` and
  `TestNavigationDetachesSurface`.
- Audio policy is covered by `TestLoadPlaybackSettingsNeverRestoresMute`,
  `TestLoadPlaybackSettingsVolumeDefault`,
  `TestOpenPlayerUsesUnmutedEffectiveAudio` and `TestToggleMuteIsNotPersisted`.
- The previous-channel action is covered by
  `TestPreviousTracksOnlyPlayingChannel`, `TestBackToPreviousReplaysChannel`,
  `TestWatchStateOffersPreviousOnlyAfterFailure` and
  `TestWatchBackReplaysPreviousChannel`.
- `go test -race ./...`, `go vet ./...` and `go tool mygo vet .` pass locally;
  golangci-lint runs in CI, because it is not installed on the QA machine.
- Cross-builds for linux/amd64 and darwin/amd64 stay green without the native
  adapter.

Native macOS smoke test (manual, partially recorded on 12 November 2026 with
MyGo 0.9.0; see
[mygo-0.9.0-upgrade.md](evidence/mygo-0.9.0-upgrade.md)):

1. Build with `go build ./cmd/mumu` and run with the pinned libmpv runtime on
   `DYLD_LIBRARY_PATH` (or `MUMU_MPV_LIBRARY`). The adapter is part of the
   default `darwin/arm64` build; no build tag is needed.
2. Play a synthetic HLS fixture, then check: video and audio start **unmuted**
   with the saved volume, the surface sits in the box at 16:9, resize and
   fullscreen keep it aligned, rapid channel switches show only the newest
   selection, favorite and volume persist while mute does not, a dead URL shows
   the glass error panel (Retry and Back) without a stale frame, buffering keeps
   the last frame with its strip badge, and quit frees the process with or
   without the floating player open.
3. Record OS, architecture, libmpv version, commands and observations under
   `docs/architecture/evidence/`.

Recorded on 12 November 2026: video and audio start, a rapid channel switch
shows only the newest selection, favorites and settings survive a restart, and
the packaged 0.9.0 build opens the existing library. Resize/fullscreen
alignment, the dead-URL glass panel, the buffering badge and quit cleanup were
not re-run in that session, so Watch video presentation is **implemented and
partially verified on device**.
