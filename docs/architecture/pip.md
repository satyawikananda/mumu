# Picture-in-Picture presentation

Issue: [#12](https://github.com/satyawikananda/mumu/issues/12). Evidence and
constraints: [PiP feasibility](pip-feasibility.md). Rendering bridge:
[video surface](video-surface.md). Watch states: [watch](watch.md).

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
attached while the player reports `playing` or `buffering`, and detached for
`loading`, `error`, `stopping` and `stopped`. A detached floating panel shows
its channel name and status text instead of a stale frame. The transfer itself
still attaches once, so entering PiP while loading leaves the panel's status
visible until the first frame arrives.

`Transfer` commits a target only after `Attach` succeeds. A failed attach rolls
back to the previous presentation; stopped and reentrant transfers are rejected;
a duplicate transfer is a no-op. Unit tests cover rollback, duplicates, invalid
targets, stopped state and 20 rapid toggles.

## Window behavior

| Event | Behavior |
|---|---|
| Enter PiP | Show the floating window, attach the same view, keep the main window visible with a placeholder |
| Return to Window | Restore and show the main window, recreate it when it was closed, show Watch, report the video box again, attach the view, hide PiP |
| Close PiP, main window exists | Return to the main window |
| Close main window during PiP | Keep PiP, audio and the session running; forget the destroyed window and mark the surface unattached |
| Return when the window cannot be recreated | Stay in PiP, keep playing, log the reason; no half transition and no playback error |
| Return when the attach fails | Transfer rolls back to PiP; PiP stays visible and the session keeps playing |
| Close PiP, no main window | Stop playback while the floating view is still alive, release the close listener, close the window once, quit once |
| App quit (Cmd+Q) with PiP open | `Stop` releases the PiP close listener, then the quit sequence closes the floating window normally; the quit is not cancelled |
| Resize PiP | Keep the video area 16:9 above the 48 pt control strip |
| Drag PiP | Native window movement; the surface frame does not change |
| App quit | Close the surface, then the engine; close the PiP window |

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

- Visible **Return to Window** and **Close** actions, plus mute.
- The main Watch window shows `Playing in Picture-in-Picture` and offers Return;
  it never renders a second preview.
- The PiP button on Watch is enabled only while a channel is loading, playing or
  buffering.

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
  `TestClosePiPWithMainWindowReturns` cover the duplicate close, the quit path
  and the Return path.
- `TestPiPControls`, `TestPiPMuted` and `TestPiPError` cover the mini player.
- `go test -race ./...`, `go vet ./...` and `go tool mygo vet .` pass locally;
  golangci-lint runs in CI; linux/amd64 and darwin/amd64 cross-builds stay green.

Native macOS smoke test (manual, partially recorded on 12 November 2026 with
MyGo 0.9.0; same gate as Watch, see
[mygo-0.9.0-upgrade.md](evidence/mygo-0.9.0-upgrade.md)):

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
7. Record OS, libmpv version, commands and observations under
   `docs/architecture/evidence/`.

Recorded on 12 November 2026 with MyGo 0.9.0: entering PiP and pressing
Return/P moved the same session back to the main window, a volume or mute change
made in PiP did not overwrite the main audio intent after Return, closing PiP
while playing left the session recoverable in the main window, and a relaunch
kept favorites, recent history and settings.

Sleep/wake and external-display recovery remain unverified, as recorded in the
feasibility report. The repeated-toggle, Spaces, foreign-fullscreen, Cmd+Q and
dead-stream steps above were not re-run in that session, so production PiP is
**implemented and partially verified on device**.
