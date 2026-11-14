# Picture-in-Picture presentation

PiP is a presentation change of the existing playback session. It moves the
retained video view into a floating native window; it does not create a second
player, decoder or preview.

Related code: `internal/video`, `internal/app/pip.go`, `internal/ui/pip.go`.
Rendering bridge: [video surface](video-surface.md). Watch states:
[watch](watch.md). Playback runtime: [playback](playback.md).

## One session, one surface

PiP is a presentation change, not a second player. Entering or leaving PiP:

- never calls `Play`, `loadfile` or `New`;
- never creates a second controller, decoder, render context or native view;
- moves the retained view with `Surface.Attach` and keeps the same libmpv
  session, stream and audio.

`internal/video.Presentation` is the only state:

```text
main ⇄ transitioning → pip
stopped is terminal during shutdown
```

`Surface.Detach` (leaving Watch for another page, or a state that shows an
overlay) is a separate transition from a PiP transfer: PiP always moves the view
with `Attach`, and both paths reuse the same retained view, context, render
context and render thread.

Inside PiP the view follows the same playback rule as the main window: it is
attached while the player reports `playing`, `buffering` or `paused`, and
detached for `loading`, `error`, `stopping` and `stopped`. A detached floating
panel shows its channel name and status text instead of a stale frame. The transfer itself
still attaches once, so entering PiP while loading leaves the panel's status
visible until the first frame arrives.

`Transfer` commits a target only after `Attach` succeeds. A failed attach rolls
back to the previous presentation; stopped and reentrant transfers are rejected;
a duplicate transfer is a no-op. Unit tests cover rollback, duplicates, invalid
targets, stopped state and 20 rapid toggles.

## Design rationale

A retained native view is used instead of a second player because it preserves
stream position, audio and decoder state. The floating window is a native MyGo
`Panel` without a parent window, so hiding or minimizing the main window does not
hide PiP. If the platform can no longer migrate this drawable, PiP must be
blocked on that configuration. Do not fall back to a WebView, an external mpv
window or a silent stream reload.

## Window behavior

| Event | Behavior |
|---|---|
| Enter PiP | Show the frameless floating window, attach the same view, keep the main window with a placeholder |
| Return to Window / P | Restore and show the main window, recreate it when it was closed, show Watch, report the video box again, attach the view, hide PiP, keep the paused state, restore the main mute intent |
| Close or Esc | Stop playback and close PiP, even when the main window exists; the main window stays on the stopped Watch page |
| Close with no main window | Stop playback, release PiP once, then quit once |
| Main window closed during PiP | Keep PiP, audio and the session running; forget the destroyed window and mark the surface unattached |
| Return when the window cannot be recreated | Stay in PiP, keep playing, log the reason; no half transition and no playback error |
| Return when the attach fails | Transfer rolls back to PiP; PiP stays visible and the session keeps playing |
| App quit (Cmd+Q) with PiP open | `Stop` releases the PiP close listener, then the quit sequence closes the floating window normally; the quit is not cancelled |
| Move/resize | Frameless 16:9 window, remembered per display and clamped into the work area |
| App quit | Close the surface, then the engine; close the PiP window |

## Borderless chrome

- PiP is a frameless, always-on-top panel with no title bar or traffic lights.
  The video fills the whole window; there is no permanent control strip.
- Default 640×360, compact 480×270, mini 360×203. The window stays 16:9,
  scales its icons/chrome and uses a 14 pt frameless corner; below 360 points
  the chrome collapses to close only. The current MyGo window option fixes the
  corner radius at creation, so the 14/12/10 design scale is approximated by
  the 14 pt corner at every size.
- Pointer movement shows chrome; **2 seconds** without activity hides it while
  the session plays. Paused, muted or signal-lost states keep it visible.
- Top row: LIVE/PAUSED pill, channel, quality, close. Bottom row: play/pause,
  mute, return. Scrims keep the labels legible over bright frames.
- The top bar and empty video area drag the window; buttons keep their clicks.

## Audio

- PiP mute is a temporary override on the same controller. Entering PiP starts
  the local mute from the main intent; the floating panel can mute or unmute
  without rewriting that intent. Returning restores the main intent and clears
  the override, so a later session never inherits an accidental PiP mute.
- Pause is local to the session and survives Return. Resume still reconnects to
  the live edge through `buffering`; moving windows never calls `Play` or
  `loadfile`.

## Recovery in PiP

- The floating panel uses the same three-attempt, 8-second recovery as Watch and
  shows the attempt/countdown while reconnecting.
- Exhaustion is terminal: no endless retrying. Retry, Return and Close remain
  reachable, and Esc/P keep working in every state.

The app stores the native `*mygo.Window` next to the interface it presents
through, and the close callback compares that pointer. Comparing the interface
with the concrete window never matched, which used to leave a destroyed window
installed and made Return show a window that no longer existed.

Closing the main window clears the main-window reference, the adapter and the
main attachment flag, and detaches the view when the main window presented it.
The one-shot generation rules are unaffected: closing a window is not a playback
event, and no `OnWindowAllClosed` handler is registered, because MyGo only quits
when the last window closes and the floating player is still one.

### Closing the floating window

The floating window's close handler is a **user close** handler: it prevents the
default and asks the app to close PiP, so Return/Close share one path. That means
the close the app itself asks for must not pass through it, or the request would
reenter `closePiP` forever and the quit it posts would always be cancelled by the
window refusing to close.

The handler also **posts** the teardown instead of running it inline. AppKit
calls it from `windowShouldClose:`, so closing the window from inside it would
destroy the window reentrantly inside its own close request. The request is
prevented, `closePiP` is published, and the teardown runs on the next main-thread
turn once the handler has returned.

- `OnClose` returns the function that removes the handler.
- `releasePiPWindow` removes the handler **first**, then closes the window.
- `closePiP` is idempotent: once the window is released there is nothing left to
  close, so a second request does not stop playback or quit again.
- The no-main-window teardown order is fixed: stop playback while the floating
  drawable is still alive, release the window, then request quit once. Releasing
  the native view from an already destroyed window is a hang risk avoided here.
- `Stop` (from `OnBeforeQuit`) also removes the handler, so MyGo's quit sequence
  can close every window instead of being refused by PiP. Without it, quitting
  with PiP open never finishes.

The handler is registered once per floating window; a later PiP session creates
a new window and a new handler.

The floating window is a native MyGo `Panel` with `AlwaysOnTop`, no parent
window, and native resize/move behavior. It is not a WebView or an external mpv
window.

The app keeps the floating window as a **second redraw target**. `publish` runs
the state callback once through the main window and then invalidates the PiP
window: a destroyed main window still runs the callback on the main thread, but
its own repaint is a no-op, so without the second target mute, status and
buffering changes would not appear until an unrelated window event. Releasing
PiP clears the target together with the window reference, so no later publish
invalidates a destroyed window.

## Interaction

- Chrome appears on pointer movement and recedes after two idle seconds while
  playing; paused, muted and signal-lost states keep it visible.
- Top row: channel, LIVE/PAUSED, quality and Close. Bottom row: play/pause, mute
  and Return. Esc stops and closes; P returns to the main window.
- The main Watch window shows `Playing in Picture-in-Picture` and offers Return;
  it never renders a second preview.
- The PiP button on Watch is enabled while a channel is loading, playing,
  buffering or paused.

## Verification

Automated:

- `TestTransfer*` in `internal/video` cover the state machine.
- `TestEnterPiP`, `TestEnterPiPFailure` and `TestEnterPiPDoesNotPlay` cover one
  surface, rollback, the PiP marker, the attachment flags and the absence of a
  second controller.
- `TestMainWindowClosedClearsWindowAndSurface` covers forgetting a destroyed
  window and detaching its view.
- `TestReturnFromPiPRecreatesMainWindow` covers recreating the window, showing
  Watch and attaching exactly once;
  `TestReturnFromPiPWithoutWindowStaysInPiP` and
  `TestReturnFromPiPKeepsPiPOnAttachFailure` cover staying in PiP with the
  session intact.
- `TestPiPDetachesForOverlayStates` covers the loading/error/stopped detach.
- `TestClosePiPWithoutMainWindowClosesOnce` covers a user close with no main
  window: one destroyed window, the listener removed before the app's close, one
  playback teardown and one quit.
- `TestClosePiPTwiceIsIdempotent`, `TestStopReleasesPiPCloseListener` and
  `TestClosePiPStopsWithMainWindow` cover the duplicate close, the quit path and
  the new stop-with-main-window contract.
- `TestPiPMuteIsTemporary`, `TestPiPCloseClearsTemporaryMute` and the PiP UI
  tests cover the local mute override, restoration and chrome states.
- `go test -race ./...`, `go vet ./...` and `go tool mygo vet .` pass locally;
  golangci-lint runs in CI; linux/amd64 and darwin/amd64 cross-builds stay green.

Native macOS smoke test with the current build:

1. Play a channel, enter and leave PiP five times quickly, then twenty times.
2. Confirm one libmpv session, one retained view and one PiP window in the logs.
3. Check drag, resize, Spaces, another app's fullscreen, minimize/hide of the
   main window, closing the main window during PiP, Return after that close, and
   Close PiP without a main window. That close must destroy the window and quit
   once, without a spinning or frozen UI.
4. With PiP open, press Cmd+Q (and quit from the menu): the app must exit, not
   stay alive with a dead PiP window.
5. Force a dead stream in PiP: the stale frame must disappear and the panel's
   error text must be readable.
6. Confirm audio continuity and that the main window never shows a second
   preview.
7. Record OS, libmpv version, commands and observations in the release testing
   notes.

The borderless drag/resize, Spaces, foreign-fullscreen, Cmd+Q and dead-stream
steps still need a fresh native run. Production PiP remains implemented and
partially verified on device.
