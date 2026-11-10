package video

import (
	"errors"
	"math"

	"mumu/internal/player"
)

// Surface errors.
var (
	// ErrUnsupported reports a platform without a native video surface.
	ErrUnsupported = errors.New("video: native video surface is unsupported on this platform")
	// ErrNotMainThread reports a surface call from another goroutine. AppKit
	// widget work must run on the main thread.
	ErrNotMainThread = errors.New("video: native surface must be used from the main thread")
	// ErrClosed reports use after Close.
	ErrClosed = errors.New("video: surface is closed")
	// ErrNoContentView reports a window handle without a content view, which
	// can happen when the window is already closing.
	ErrNoContentView = errors.New("video: window has no content view")
	// ErrNotAttached reports SetFrame before Attach or while the surface is
	// detached.
	ErrNotAttached = errors.New("video: surface is not attached to a window")
)

// Rect is a rectangle in window device-independent pixels, the points AppKit
// lays out in. X and Y are the top-left origin of the window's content view,
// measured downwards; the platform adapter converts to AppKit's bottom-left
// coordinate space.
type Rect struct {
	X, Y, W, H float32
}

// Surface presents one player session's video inside a native window.
//
// A Surface owns the retained native view and the single render thread that
// draws the player's frames. It is not a second player: the same engine, stream
// and decoder keep running, so a future PiP presentation can reuse it.
//
// Threading contract: New, Attach, Detach, SetFrame and Close must run on the
// main thread, because they touch native widgets. Errors reports renderer
// failures from the render thread and closes when the render thread stops.
// Close is idempotent and must run before the player engine closes, otherwise
// the render context is freed too late.
type Surface interface {
	// Attach moves the retained view into window and applies frame. The window
	// is the native handle of an NSWindow; frame is in window DIPs. Attach
	// clears and rebinds the drawable, so it is safe on a re-shown window and
	// after Detach: it reuses the same view, context and render context.
	Attach(window uintptr, frame Rect) error
	// Detach removes the retained view from its window but keeps the surface,
	// its OpenGL context, its render context and the render thread alive, so a
	// later Attach resumes without a stream reload or a new render context.
	// The drawable size becomes zero and the render thread skips frames while
	// detached. Detach is idempotent, and a no-op before the first Attach.
	Detach() error
	// SetFrame resizes the attached view; the drawable is updated immediately.
	// It returns ErrNotAttached before Attach or while detached, and does not
	// change the frame that a later Attach applies.
	SetFrame(frame Rect) error
	// Size returns the current drawable size in backing pixels, not DIPs. A
	// detached surface reports 0, 0.
	Size() (width, height int)
	// Errors reports renderer failures. It closes when the render thread stops.
	Errors() <-chan error
	// Close stops the render thread, then releases the native view and
	// context. It is idempotent and must run on the main thread.
	Close()
}

// New creates the native surface and its render thread.
//
// It fails with player.ErrRenderDisabled when the engine cannot render, and
// with ErrUnsupported on platforms without a native adapter. The caller must
// Close the surface before closing the player engine.
func New(renderer player.Renderer) (Surface, error) {
	return newSurface(renderer)
}

// Supported reports whether this build carries a native video surface. Callers
// use it to choose the engine's video output before creating the engine,
// because "libmpv" is only meaningful with a drawable.
func Supported() bool {
	return surfaceSupported
}

// Fit returns the largest rectangle of the given aspect ratio centred in area.
// The app uses it to letterbox video inside a content area; mpv keeps its own
// aspect handling inside the returned frame.
func Fit(area Rect, aspect float64) Rect {
	if area.W <= 0 || area.H <= 0 || aspect <= 0 || math.IsNaN(aspect) || math.IsInf(aspect, 0) {
		return Rect{}
	}

	width := area.W
	height := float32(float64(width) / aspect)
	if height > area.H {
		height = area.H
		width = float32(float64(height) * aspect)
	}

	return Rect{
		X: area.X + (area.W-width)/2,
		Y: area.Y + (area.H-height)/2,
		W: width,
		H: height,
	}
}

// PixelSize converts a frame in DIPs to the backing-pixel drawable size. A
// non-positive frame or a non-positive scale yields no drawable, and the caller
// must skip rendering instead of guessing a size.
func PixelSize(frame Rect, scale float32) (int, int) {
	if frame.W <= 0 || frame.H <= 0 {
		return 0, 0
	}
	if scale <= 0 {
		scale = 1
	}

	width := int(math.Round(float64(frame.W * scale)))
	height := int(math.Round(float64(frame.H * scale)))

	return max(width, 1), max(height, 1)
}

// originY converts a DIP y coordinate to AppKit's bottom-left frame origin.
func originY(frame Rect, containerHeight float32, flipped bool) float32 {
	if flipped {
		return frame.Y
	}

	return containerHeight - frame.Y - frame.H
}
