package video

import "testing"

// A fresh surface has no drawable: the render thread must skip frames until the
// first Attach.
func TestDrawableStartsDetached(t *testing.T) {
	var state drawable

	if width, height := state.size(); width != 0 || height != 0 {
		t.Fatalf("size = %dx%d, want 0x0 before Attach", width, height)
	}
	if width, height, ok := state.renderFrame(true); ok || width != 0 || height != 0 {
		t.Fatalf("renderFrame = %dx%d, %v, want no frame", width, height, ok)
	}
}

// Attach resizes the drawable and asks for exactly one redraw.
func TestDrawableResize(t *testing.T) {
	var state drawable

	state.resize(1280, 720)

	if width, height := state.size(); width != 1280 || height != 720 {
		t.Fatalf("size = %dx%d, want 1280x720", width, height)
	}

	width, height, ok := state.renderFrame(false)
	if !ok || width != 1280 || height != 720 {
		t.Fatalf("renderFrame = %dx%d, %v, want a pending redraw", width, height, ok)
	}

	state.clean()
	if _, _, ok := state.renderFrame(false); ok {
		t.Fatal("renderFrame drew again after clean without an update")
	}
	if _, _, ok := state.renderFrame(true); !ok {
		t.Fatal("an engine update must still draw a clean drawable")
	}
}

// A non-positive frame carries no drawable, so no frame is drawn even when the
// engine reports an update.
func TestDrawableResizeRejectsNonPositive(t *testing.T) {
	for _, size := range [][2]int{{0, 720}, {640, 0}, {-4, 360}, {640, -1}} {
		var state drawable
		state.resize(size[0], size[1])

		if width, height := state.size(); width != 0 || height != 0 {
			t.Fatalf("resize(%d, %d) = %dx%d, want 0x0", size[0], size[1], width, height)
		}
		if _, _, ok := state.renderFrame(true); ok {
			t.Fatalf("resize(%d, %d) allowed a frame without a drawable", size[0], size[1])
		}
	}
}

// Detach zeroes the size and the pending redraw, so Size reports 0, 0 and the
// render thread skips frames even when the engine keeps producing them.
func TestDrawableDetach(t *testing.T) {
	var state drawable
	state.resize(1280, 720)

	state.detach()

	if width, height := state.size(); width != 0 || height != 0 {
		t.Fatalf("size = %dx%d, want 0x0 while detached", width, height)
	}
	for _, updated := range []bool{false, true} {
		if width, height, ok := state.renderFrame(updated); ok || width != 0 || height != 0 {
			t.Fatalf("renderFrame(updated=%v) = %dx%d, %v, want no frame", updated, width, height, ok)
		}
	}
}

// Detach is idempotent, including before the first Attach, and a re-attach
// restores a renderable drawable with backing pixels and a pending redraw.
func TestDrawableDetachIdempotent(t *testing.T) {
	var state drawable

	// Never attached yet.
	state.detach()
	state.detach()
	if width, height := state.size(); width != 0 || height != 0 {
		t.Fatalf("size = %dx%d after detaching an unattached surface, want 0x0", width, height)
	}

	state.resize(1280, 720)
	state.clean()

	for range 3 {
		state.detach()
	}

	if width, height := state.size(); width != 0 || height != 0 {
		t.Fatalf("size = %dx%d after repeated detaches, want 0x0", width, height)
	}
	if _, _, ok := state.renderFrame(true); ok {
		t.Fatal("a repeatedly detached surface drew a frame")
	}

	// Re-attach is the same path Attach uses: the drawable returns with a
	// pending redraw, so frames resume without a new context.
	state.resize(2560, 1440)

	if width, height := state.size(); width != 2560 || height != 1440 {
		t.Fatalf("size = %dx%d after re-attach, want 2560x1440", width, height)
	}
	width, height, ok := state.renderFrame(false)
	if !ok || width != 2560 || height != 1440 {
		t.Fatalf("renderFrame = %dx%d, %v after re-attach, want a pending redraw", width, height, ok)
	}
}
