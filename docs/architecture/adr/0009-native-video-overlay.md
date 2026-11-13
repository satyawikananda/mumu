# ADR 0009: MyGo chrome above the native libmpv video surface

- Status: accepted; production integration landed in #38 (see [Watch](../watch.md))
- Date: 2026-11-12
- Issues: [#32](https://github.com/satyawikananda/mumu/issues/32), tracker [#30](https://github.com/satyawikananda/mumu/issues/30)
- Evidence: [overlay-2026-11-12](../evidence/overlay-2026-11-12/README.md)
- Related: [video surface](../video-surface.md), [PiP](../pip.md), [local storage](0004-local-sqlite-storage.md)

## Context

mumu plays live channels with libmpv on a retained `NSOpenGLView`
(`internal/video`). MyGo draws the application interface itself, in a
layer-backed `MyGoSurfaceView` whose layer is a `CAMetalLayer`.

The M0/M1 bridge added the retained view as a subview of the window's content
view, which puts it above MyGo's surface. Every panel, scrim and control built
in MyGo therefore sits behind live video. The product works around that by
detaching the view whenever it must show loading, error or stopped UI, and by
placing buffering feedback below the video box. The M5 revamp needs the
opposite: live video with interactive chrome, a channel drawer and player
controls above it, without hiding or reloading the stream.

The revamp also needs a paused frame that stays visible while audio stops,
because #37 builds live pause on this seam. The tracker forbids detaching live
video to simulate an overlay, a WebView, and a migration to MyGo's `ui/video`
system engine.

## Decision

**Insert the retained video view under MyGo's surface and keep MyGo as the
owner of pointer input.** Concretely, the production path is:

1. Attach the retained `NSOpenGLView` to the window content view as today.
2. Move it under the surface view with
   `addSubview:positioned:relativeTo: NSWindowBelow`.
3. Leave the window root transparent on the Watch page and let MyGo paint
   chrome with alpha scrims over the video.
4. Refresh the drawable from the hosted view's own bounds instead of applying
   frames from the app layout.

Status is **CONDITIONAL GO**: composition, input, the single session, twenty
repeated window transfers, resize/fullscreen and the container radius are
recorded, as is the pause seam. The remaining checks are listed under
"Not verified" in the evidence file and gate #38/#39, not this ADR.

### Why not `ui.NativeView`

MyGo 0.9 hosts native views under its surface through `ui.NativeView`, which
places and clips the view in an element. It cannot carry interactive chrome:
`MyGoSurfaceView.hitTest:` returns nil wherever a placed native view shows
(`internal/darwin/surface.go`), and `webViewAt` decides that by geometry alone
(`internal/darwin/webview.go`). A full-bleed video element therefore takes every
click in the window, including clicks on our control bar. The spike keeps a
`nativeview` mode to record that, and the product path does its own insertion
so no placed view exists and MyGo keeps the pointer.

### Rejected alternatives

- **Chrome in a separate transparent window above the main window.** Keeps the
  engine, but adds a second `ui.Context`, a second window to position, focus and
  order, and duplicate layout for every overlay state. Kept as the fallback if
  under-surface composition regresses on another macOS version.
- **Copy each frame into a MyGo texture.** One window and no AppKit ordering,
  but a per-frame readback and upload; it contradicts the retained native
  surface this project already ships and was not measured as viable.
- **Patch MyGo to make placed views input-transparent.** Out of scope for a
  spike, and the M0 work explicitly avoided a MyGo fork.

## Threading and ownership

| Work | Thread | Notes |
| --- | --- | --- |
| Create, attach, insert below, detach, close | main (AppKit) | `video.Surface` already enforces `ErrNotMainThread` |
| Read hosted bounds, update drawable, signal one redraw | main | `Surface.SyncDrawable`; bounded work, no goroutine |
| Draw frames, lock CGL, swap | render thread | unchanged from #11 |
| libmpv commands, pause, read-ahead | engine pump goroutine | unchanged |

`SyncDrawable` is the seam this ADR adds: the app no longer applies a frame to
the view, so nothing else would refresh the drawable size after layout, a
window move or a scale change. It reads `bounds` and the window's
`backingScaleFactor`, stores the pixel size, and asks the render thread for one
redraw. It detaches the drawable state when the view has no window, so the
render thread skips frames instead of guessing.

Teardown order, in one UI-thread closure: remove the element or page that shows
the video, detach the view, stop the render thread, release the view and
context, then close the engine. `Close` already keeps this order; the insertion
path must not release the view while a window still hosts it.

## Integration boundary

#32 proves the seam; it does not redesign Watch or PiP.

- `internal/video` keeps `Attach`/`Detach`/`SetFrame` and gains
  `NativeViewHandle` plus `SyncDrawable`. `Attach` inserts below the MyGo
  surface in the production path, with the old above-content ordering only as a
  fallback when that view is absent.
- #38 owns the production attach, and #37/#38/#39 use it: insert below the
  surface, transparent root on Watch and PiP, chrome scrims and the drawer, and
  the per-layout `SyncDrawable` call.
- #39 owns PiP: the same retained view moves between two windows, so the panel
  needs the same insertion and a drawable refresh after the move, plus drag and
  radius checks.
- #37 owns pause behavior. `libmpv.Engine.SetPaused` and `SetReadAhead` are the
  product/an experiment seam respectively; see [ADR 0010](0010-live-pause-and-recovery.md).

## Measured cost

`MYGO_FRAME_STATS=1` with live 720p video and full chrome: 8 frames over 8 ms in
20 seconds; build 0.1-0.2 ms, layout 0.0-4.4 ms, paint 0.1-2.7 ms, present
3.1-14.5 ms. The idle window draws nothing. Presenting waits on the display, so
this is not a GPU cost measurement.

## Consequences

- The product can show loading, buffering and error UI over live video instead
  of detaching the view, which removes the current detach dance from the Watch
  page once #38 lands.
- MyGo keeps all pointer and keyboard input, so overlays, focus rings and
  accessibility keep working as they do on every other page.
- The app owns the view's geometry and clipping again; corner radius and scroll
  clipping need explicit checks in #38/#39 rather than being inherited from an
  element.
- The seam is AppKit-specific. A future Windows or Linux adapter needs its own
  composition decision; nothing here claims portability.
- MyGo's native view hosting may change in a later release. The product path
  depends only on the surface view staying a sibling of the content view, which
  the spike asserts by logging the subview order.
