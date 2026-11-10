package video

// drawable is the drawable size and redraw state shared by the AppKit callers
// and the render thread.
//
// The native adapter guards it with the same Go mutex that protects the CGL
// context, but the type itself is platform-free so the attach/detach contract
// is unit-testable without a window, a GL context or a render thread.
//
// A zero size means "no drawable": the render thread must skip every frame
// instead of guessing a size. Attach resizes it; Detach forgets it.
type drawable struct {
	width, height int
	dirty         bool
}

// resize records a new drawable size and asks for one redraw. A non-positive
// size is stored as zero, which keeps the state unrenderable.
func (d *drawable) resize(width, height int) {
	if width <= 0 || height <= 0 {
		width, height = 0, 0
	}

	d.width, d.height = width, height
	d.dirty = true
}

// detach forgets the drawable. It is idempotent and also clears the pending
// redraw, so a later Attach starts from a clean state.
func (d *drawable) detach() {
	d.width, d.height = 0, 0
	d.dirty = false
}

// size returns the current drawable size in backing pixels. A detached surface
// reports 0, 0.
func (d *drawable) size() (width, height int) {
	return d.width, d.height
}

// renderFrame decides one render-thread wakeup. It returns the backing pixels to
// draw into and whether to draw at all: only when the engine has an update or a
// redraw is pending, and only while a drawable exists. After Detach this is
// always false, so no GL or AppKit call touches a stale drawable and nothing is
// logged per frame.
func (d *drawable) renderFrame(updated bool) (width, height int, ok bool) {
	if d.width <= 0 || d.height <= 0 || (!updated && !d.dirty) {
		return 0, 0, false
	}

	return d.width, d.height, true
}

// clean clears the pending redraw after a successful swap.
func (d *drawable) clean() {
	d.dirty = false
}
