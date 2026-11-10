package player

import (
	"errors"
	"unsafe"
)

// Render errors.
var (
	// ErrRenderDisabled reports an engine that was configured without
	// on-screen video output. Headless engines (vo=null) keep control, events
	// and metrics working but cannot produce frames.
	ErrRenderDisabled = errors.New("player rendering is not enabled")
	// ErrRenderActive reports a second render context for one engine. An
	// engine carries at most one context; it is created once and freed once.
	ErrRenderActive = errors.New("player render context already exists")
)

// Renderer creates the one render context that draws an engine's frames into
// a native graphics surface.
//
// An Engine configured for on-screen video implements Renderer. Implementations
// must reuse the existing client and decoder: creating a render context must
// never create a second player, a second stream load or an external process.
//
// Threading contract:
//
//   - NewRenderContext runs once per engine, on the render thread that owns
//     the native graphics context, with that context already current and
//     locked by the caller.
//   - getProcAddress is only called while creating the context. It resolves
//     entry points for that same context, and must not call back into the
//     engine or block.
//   - A second call for the same engine fails with ErrRenderActive.
type Renderer interface {
	NewRenderContext(getProcAddress func(name string) unsafe.Pointer) (FrameRenderer, error)
}

// FrameRenderer draws the current video frame into the native surface and
// reports presentation progress back to the engine.
//
// Threading contract:
//
//   - Render, ReportSwap, Update and Free run on the render thread, which is
//     the thread that called Renderer.NewRenderContext. The native context
//     must be current and locked before each of those calls.
//   - SetUpdateCallback is called from the render thread, but fn is invoked by
//     an engine-owned thread. fn may only signal another goroutine over a
//     bounded channel: it must not call the engine, GL, AppKit or the UI.
//   - Render must be skipped while the drawable has no positive size.
//   - Free is idempotent and must run on the render thread before the engine
//     is closed.
type FrameRenderer interface {
	SetUpdateCallback(fn func())
	Update() bool
	Render(width, height int) error
	ReportSwap()
	Free()
}
