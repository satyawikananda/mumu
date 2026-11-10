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

`Surface.Detach` (leaving Watch for another page) is a separate transition from a
PiP transfer: PiP always moves the view with `Attach`, and both paths reuse the
same retained view, context, render context and render thread.

`Transfer` commits a target only after `Attach` succeeds. A failed attach rolls
back to the previous presentation; stopped and reentrant transfers are rejected;
a duplicate transfer is a no-op. Unit tests cover rollback, duplicates, invalid
targets, stopped state and 20 rapid toggles.

## Window behavior

| Event | Behavior |
|---|---|
| Enter PiP | Show the floating window, attach the same view, keep the main window visible with a placeholder |
| Return to Window | Restore and show the main window, recreate it when it was closed, attach the view, hide PiP |
| Close PiP, main window exists | Return to the main window |
| Close main window during PiP | Keep PiP, audio and the session running |
| Close PiP, no main window | Stop playback, close the window, quit the app |
| Resize PiP | Keep the video area 16:9 above the 48 pt control strip |
| Drag PiP | Native window movement; the surface frame does not change |
| App quit | Close the surface, then the engine; close the PiP window |

The floating window is a native MyGo `Panel` with `AlwaysOnTop`, no parent
window, and native resize/move behavior. It is not a WebView or an external mpv
window.

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
  surface, rollback, the PiP marker and the absence of a second controller.
- `TestPiPControls`, `TestPiPMuted` and `TestPiPError` cover the mini player.
- `go test -race ./...`, `go vet ./...`, `go tool mygo vet .` and golangci-lint
  pass; linux/amd64 and darwin/amd64 cross-builds stay green.

Native macOS smoke test (manual, pending, same gate as Watch):

1. Play a channel, enter and leave PiP five times quickly, then twenty times.
2. Confirm one libmpv session, one retained view and one PiP window in the logs.
3. Check drag, resize, Spaces, another app's fullscreen, minimize/hide of the
   main window, closing the main window during PiP, Return after that close, and
   Close PiP without a main window.
4. Confirm audio continuity and that the main window never shows a second
   preview.
5. Record OS, libmpv version, commands and observations under
   `docs/architecture/evidence/`.

Sleep/wake and external-display recovery remain unverified, as recorded in the
feasibility report. Until the smoke run is recorded, production PiP is
**implemented but not verified on device**.
