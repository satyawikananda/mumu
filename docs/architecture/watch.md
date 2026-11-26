# Watch experience and video presentation

Watch is the full-bleed playback page. It depends on the
[playback controller](player-controller.md), the
[native video surface](video-surface.md) and the
[per-source stream policy](sources.md#per-source-stream-policy).

## Layout

```text
Watch (full window, video underneath MyGo's surface)
┌───────────────────────────────────────────────────────────┐
│ back · channel · LIVE/PAUSED · quality          top scrim │
│                                                           │
│                    live video / last frame                │
│                                                           │
│ category · source · country · channel title               │
│ ─────────────── live status line (no playhead) ────────── │
│ play · volume · mute       favorite · channels · quality  │
│                            PiP · fullscreen               │
└───────────────────────────────────────────────────────────┘
```

- Watch is full-bleed. The shell top bar, page padding and global hint strip are
  not drawn on this route; the page owns its top bar, hint bar and back action.
  The shell's `Themed` wrapper is transparent on Watch, and the page sets the
  window root transparent too: both layers otherwise paint the opaque canvas
  over the retained video.
- The video box is the whole content area and is reported through `OnFrame`.
  `video.Fit` letterboxes 16:9 inside it; MyGo chrome and scrims paint above the
  video because the retained view sits under `MyGoSurfaceView`.
- The live line is a non-interactive 4px status treatment. It has no playhead,
  elapsed time, seek or timeshift and is not exposed as a slider.
- The channel drawer is a 420 pt panel on the right with a dim overlay. It
  reflows the timeline and controls, never the video or the session. Its grid
  loads the next 20 tiles when the scroll reaches the bottom: no "Show more"
  button. Search filters the same tile grid and pages it the same way; there is
  no separate row-list mode. Rows follow the active channel's source, so the
  drawer never shows a channel list from the last Discover country.
- The quality control opens a popover of Auto plus the channel's real playlist
  labels. Unknown labels are not filled with a guessed resolution.
- Favorite has both a control in the right cluster and the `F` shortcut.
- Settings is reachable from Watch with `Cmd+,`; Esc opens Control Center,
  except during the initial connecting attempt where it cancels.

## Idle chrome

- While `playing` and the chrome is visible, pointer movement, clicks, scrolling
  and keys reset its idle deadline. After **3 seconds** without activity the
  chrome and the system pointer hide together: top bar, scrims, metadata, live
  line and controls are not built.
- **Pointer movement alone** brings the full chrome and the pointer back; no
  click is needed. The chevron-up stays as the idle affordance, but the move
  that reaches it reveals the controls first. Control Center remains on the top
  bar, on Esc, and on assistive-technology activation of the chevron.
- Chrome never hides while paused, loading, buffering/recovery or in error, or
  while the drawer, quality popover or Control Center is open, or while focus is
  inside it.
- The cursor is the stage's own: while the chrome is hidden the full-bleed
  stage carries `ui.CursorNone`, so MyGo hides the pointer only over the
  picture and restores it in the frame that brings the chrome back. Buttons and
  the chevron keep their own cursor. There is no global `NSCursor.hide`, so
  leaving Watch or losing focus cannot leave an invisible pointer behind.
- Activity is event-driven: a local `NSEvent` monitor reports real input, the
  page stores the last activity time and `c.After` wakes it at the deadline. No
  pointer polling and no frame per mouse move beyond MyGo's tracked pointer.

## Surface visibility

The app calls `syncSurfaceVisibility` inside the `publishWatch` closure, on the
UI thread, **before** `SetState`, so the page state and the drawable never
disagree for a painted frame. `Attach` rejects non-main-thread calls, which is
why it runs there.

| Playback state | Native view |
|---|---|
| `playing` | attached to the presenting window; chrome overlays it |
| `buffering` | attached: the last frame stays, the recovery overlay is above it |
| `paused` | attached: the held frame is the picture |
| `loading` | detached: the full-window connecting screen owns the picture |
| `error` | detached: the terminal panel replaces the stale frame |
| `stopping`, `stopped`, idle | detached |

- A main-window presentation needs `a.win != nil` and Watch as the current page:
  a stream that starts playing while Discover or Library is open must not cover
  it. A PiP presentation needs the floating window instead.
- Attach moves the retained view under `MyGoSurfaceView`, so MyGo keeps pointer
  input and paints scrims, panels and controls above live video. A window
  without that view falls back to the old above-content order.
- Leaving Watch detaches the view; returning re-attaches it without a stream
  reload. Moving the view to PiP clears the main-window attachment flag, so the
  navigation path cannot knock the floating player's drawable out.

## Platforms without a native surface

`windows/amd64` and `linux/amd64` build the libmpv engine for headless tests, but
they carry no native surface adapter. A Play request is refused before any
stream is loaded, and Watch shows a `WatchError` that names the missing video
presentation. It does not start audio, show a playing state, enter PiP or record
the channel as watched. Retry cannot turn headless support into a player
window; the message is a product limitation, not a missing runtime.

The table below describes the macOS presentation path.

## State mapping

| Player state | Watch state | Visible treatment | Native view |
|---|---|---|---|
| no active channel, no recent | `WatchEmpty` | Nothing playing, Browse Discover | detached |
| no active channel, valid recent | `WatchResume` | Resume card, no autoplay | detached |
| `loading`, attempt 1 | `WatchLoading` | Full-window connecting screen, Cancel / Other channels / Try again | detached |
| `playing` | `WatchPlaying` | Live chrome; idle recedes after 3 s | attached |
| `buffering` | `WatchBuffering` | Last frame, dim, reconnecting copy and attempt countdown | attached |
| `paused` | `WatchPaused` | Held frame, pause card, PAUSED pill and live line | attached |
| `error`, attempts remain | `WatchBuffering` | Reconnect overlay with attempt and countdown | attached |
| `error`, exhausted | `WatchError` | Terminal panel with Retry and Pick another channel | detached |
| `stopping`, `stopped` | `WatchStopped` | Playback stopped, Browse Discover | detached |

`LIVE` comes from the stream variant or the engine's `Capabilities.Live`. It
never claims that an upstream stream is online. `PAUSED` and `BUFFERING` come
from player state, not from the design mock.

There is no seek bar. Live streams are not assumed seekable in v0.1.

## Quality

- Auto keeps the supported-stream choice (live/HLS first). An explicit label
  moves that real variant to the front; the remaining variants stay available
  for recovery.
- The player menu lists Auto plus distinct playlist labels. Variants without a
  label still participate in fallback but are not offered as a choice.
- The snapshot reports the variant actually attempted, whether its label is
  known and whether it is a fallback. An unavailable selection falls back to
  Auto and is shown as such.
- A choice applies to the current channel session only. There is no persisted
  default quality in the current release.

## Recovery

- Three attempts total, including the initial one, with an 8-second no-progress
  deadline and an 8-second floor between attempt starts.
- A fast failure shows a truthful countdown; a stall that already used its
  deadline retries immediately. Exhaustion is terminal and never says
  "retrying".
- Retry is manual and starts a new episode. A channel switch, quality change,
  Pause (with a frame), Stop or Close cancels recovery. Variants never leave the
  current channel.
- The connecting screen offers Cancel (Esc), the channel drawer (C) and Try
  again (R) without claiming progress that does not exist.

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
- `Recent.MarkPlayed` runs once per playback episode, after the controller
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
  dispatch, drawer switching, quality options, pause/reconnect copy and the
  empty/error paths.
- Idle chrome is covered with the Tester clock: three seconds hide, activity
  reveals, and paused/loading/drawer/focus states never hide.
- Surface rules are covered by `TestSurfaceAttachesOnlyWhilePresenting`,
  `TestSurfaceDetachesWhenPlaybackLeavesVideo`,
  `TestPresentingStateIncludesPaused`,
  `TestPublishWatchDetachesBeforeSettingState` and
  `TestNavigationDetachesSurface`.
- Audio policy is covered by `TestLoadPlaybackSettingsNeverRestoresMute`,
  `TestLoadPlaybackSettingsVolumeDefault`,
  `TestOpenPlayerUsesUnmutedEffectiveAudio` and `TestToggleMuteIsNotPersisted`.
- Recovery, variant fallback, quality selection and pause are covered by the
  controller fake-clock tests.
- The previous-channel action is covered by
  `TestPreviousTracksOnlyPlayingChannel`, `TestBackToPreviousReplaysChannel`,
  `TestWatchStateOffersPreviousOnlyAfterFailure` and
  `TestWatchBackReplaysPreviousChannel`.
- `go test -race ./...`, `go vet ./...` and `go tool mygo vet .` pass locally;
  golangci-lint runs in CI, because it is not installed on the QA machine.
- Cross-builds for darwin/arm64, linux/amd64, windows/amd64 and the
  darwin/amd64, linux/arm64, windows/arm64 stubs stay green without a native
  runtime. Windows/Linux headless tests do not stand in for the macOS smoke
  test.

Native macOS smoke test:

1. Build with `go build ./cmd/mumu` and run with the pinned libmpv runtime on
   `DYLD_LIBRARY_PATH` (or `MUMU_MPV_LIBRARY`). The adapter is part of the
   default `darwin/arm64` build; no build tag is needed.
2. Play a synthetic HLS fixture, then check: video and audio start **unmuted**
   with the saved volume, the video fills the window with chrome above it,
   resize and fullscreen keep it aligned, rapid channel switches show only the
   newest selection, favorite and volume persist while mute does not.
3. Pause with Space: the frame holds, audio stops, the status reads PAUSED and
   read-ahead drops but does not reach zero. Resume reconnects at the live edge
   through the buffering state, without a seek or paused-position claim.
4. Kill the fixture: three attempts run with an honest countdown, variant
   fallback stays in the same channel, and the terminal panel offers Retry
   without saying retrying. A manual Retry starts attempt one again.
5. Leave the pointer still for three seconds: chrome, scrims and the pointer
   disappear together. Moving the pointer (no click) brings all of them back.
   Paused, drawer, quality menu and error states never hide.
6. Open the drawer and quality menu over live video: both stay interactive, the
   stream is not reloaded, the live line is not draggable, and unknown metadata
   stays absent.
7. Record OS, architecture, libmpv version, commands and observations in the
   release testing notes.

Watch video presentation is implemented and partially verified on device.
Resize and fullscreen alignment, dead-stream handling, buffering badges and quit
cleanup still need a fresh full run.
