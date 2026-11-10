# Native video surface

Issue: [#11](https://github.com/satyawikananda/mumu/issues/11). Evidence:
[native playback](native-playback-feasibility.md) (CONDITIONAL GO) and
[PiP feasibility](pip-feasibility.md). This document covers the render port in
`internal/player` and the darwin adapter in `internal/video`; Watch screens stay
in `internal/ui` and `internal/app`.

The bridge is the M0 spike, promoted with production-shaped lifetime rules. It
is still **conditional**: macOS OpenGL is deprecated, and no production-quality
render run is claimed here. A native smoke run belongs to the Watch review.

## Ownership

```text
ui (MyGo)        dispatches intents, never touches GL
  └─ app         composition root: creates engine, controller, surface
       ├─ player.Controller        playback state machine (one session)
       ├─ player.Renderer/FrameRenderer   render port
       └─ video.Surface            retained native view + one render thread
```

- `player` declares the port and imports nothing native.
- `libmpv` implements the port on the existing client. One engine keeps one
  client and at most one render context; nothing creates a second decoder, a
  second stream load or an external process.
- `video` owns the retained `NSOpenGLView`, its `NSOpenGLContext` and the drawing
  thread. It never calls the engine synchronously.

## Threading

| Goroutine | Responsibility |
|---|---|
| main (AppKit) | `video.New`, `Attach`, `Detach`, `SetFrame`, `Close`, all widget work |
| render thread (`runtime.LockOSThread`) | GL context current, `Render`, `ReportSwap`, `Update`, `Free` |
| libmpv pump (existing) | every client call, event wait, property poll |
| mpv callback thread | `SetUpdateCallback` fn: one non-blocking channel send |

The render callback may only signal. `render.h` forbids client calls or waits
from the render thread, so drawing and control never share a goroutine.

Two locks guard the drawable: a Go mutex and the native `CGLLockContext`.
AppKit also touches the drawable, and M0 capture produced a native SIGSEGV
with the Go mutex alone. Lock order is always Go mutex, then CGL.

The context is made current before each render, and after every drawable
change, because `clearDrawable`/`setView:` detach it.

## Attach and Detach

`Surface` has two drawable transitions, and neither changes the session:

```text
Attach(window, frame)  add retained view → clearDrawable → setView: → apply frame
Detach()               removeFromSuperview → clearDrawable → drawable size 0
```

- `Detach` keeps the view object, the `NSOpenGLContext`, the render context and
  the render thread alive. It is the navigation path: when the viewer leaves
  Watch, the surface stops covering Discover, Library or Settings without a
  stream reload.
- A later `Attach` rebinds the same view with `setView:`, recomputes the backing
  pixels from the new window's `backingScaleFactor` and signals the render
  thread, so frames resume. No second view, context, render context or player is
  created.
- The drawable size becomes `0, 0`, so the render thread skips every frame:
  nothing calls GL or AppKit and nothing is logged per frame. Audio keeps
  playing; the stream is untouched.
- `Detach` is idempotent and a no-op before the first `Attach`. `SetFrame` while
  detached returns `ErrNotAttached` and does not remember the frame; the next
  `Attach` applies the frame it is given.
- `Close` still works from a detached state: it stops the render thread, frees
  the render context on that thread, then releases the view and context.
- Both transitions take the Go mutex and then the CGL lock, so a detach waits for
  the frame in flight instead of racing it. A blocked detach lasts at most one
  frame.

## Lifetime

Teardown order is not negotiable:

```text
surface.Close()      stop render thread → free render context on its thread
                     → remove view, clear drawable, release view and context
controller.Close(ctx) → engine.Close → client.TerminateDestroy
```

Freeing the render context after the client is destroyed is undefined
behaviour, so the app must close the surface first. `Surface.Close` is
idempotent; it blocks only until the render thread has finished.

A render failure does not kill the app. The render thread reports one error on
`Surface.Errors()` and stops; the app shows a playback error and tears down in
the order above. If the CGL lock cannot be taken during cleanup, the context is
logged as unreleased instead of crashing or hanging the process.

## App seam

The surface takes an `NSWindow` handle and a frame in window DIPs. The frame's
origin is the **top-left of the content view**, y growing downwards; the darwin
adapter flips it for AppKit's bottom-left space. `mygo.Rectangle` is already in
DIPs, so no manual scaling is needed.

```go
// main thread, inside mygo.App.WhenReady or the Watch page's first layout
output := "null" // headless: control, events and metrics still work
if video.Supported() {
    output = "libmpv" // the only value that owns a frame loop
}

engine, err := libmpv.New(libmpv.Config{VideoOutput: output}) // tagged build
if err != nil {
    // show a playback error; audio-only is a degraded mode, not a crash
}

surface, err := video.New(engine) // fails with player.ErrRenderDisabled or ErrUnsupported
if err != nil {
    // keep the engine, drop the surface; nothing native is retained
}

content := win.ContentBounds() // device-independent pixels
area := video.Rect{X: 0, Y: 0, W: float32(content.Width), H: float32(content.Height) - controlsHeight}
if err := surface.Attach(win.NativeHandle(), video.Fit(area, 16.0/9.0)); err != nil {
    // the window is closing or has no content view
}

// resize (main thread, from Window.OnResize)
_ = surface.SetFrame(video.Fit(newArea, 16.0/9.0))

// leaving Watch (main thread, from the shell's navigation intent)
_ = surface.Detach() // no view over Discover, Library or Settings; session lives

// returning to Watch (main thread)
_ = surface.Attach(win.NativeHandle(), video.Fit(area, 16.0/9.0)) // same view

// renderer failures, any goroutine
select {
case err, ok := <-surface.Errors():
    if ok {
        // show the error, then tear down in order
    }
default:
}

// shutdown (main thread)
surface.Close()
controller.Close(ctx)
```

`Attach` reparents the retained view and rebinds the drawable, so calling it
again on a re-shown window is supported. `SetFrame` returns `ErrNotAttached`
before the first `Attach` and while detached. Off-main calls return
`ErrNotMainThread`; they never touch AppKit.

Attach before the first `Play`, or on the same layout pass that reveals the
video area: with no drawable the render thread skips frames and nothing is
presented, even though audio continues.

`Detach` is the navigation exit, not a shutdown: the player keeps playing and
the next `Attach` reuses the same session. On a build without a native adapter
`New` fails with `ErrUnsupported`, so there is nothing to detach; the headless
engine is unaffected.

`vo=null` remains a valid headless engine. Rendering requires
`Config.VideoOutput == "libmpv"`; anything else fails with
`player.ErrRenderDisabled` before any native object is created.

## Scope

Not in #11: a second surface, PiP presentation state or window lifecycle (#12),
a software/CPU fallback, Metal interop, and any claim about sustained frame
delivery. `Fit` is a helper only; mpv keeps its own aspect handling inside the
returned frame.
