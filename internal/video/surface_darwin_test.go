//go:build darwin && arm64

package video

import "testing"

// The concrete adapter reports the same detached state the render thread reads.
// This runs without an NSWindow, a GL context or a render thread because it only
// touches the mutex-guarded state; a real native detach still needs the app's
// manual smoke run.
func TestNativeSurfaceDetachedState(t *testing.T) {
	surface := &nativeSurface{}

	if width, height := surface.Size(); width != 0 || height != 0 {
		t.Fatalf("Size = %dx%d before Attach, want 0x0", width, height)
	}

	surface.mu.Lock()
	surface.state.resize(1280, 720)
	surface.state.detach()
	surface.mu.Unlock()

	if width, height := surface.Size(); width != 0 || height != 0 {
		t.Fatalf("Size = %dx%d while detached, want 0x0", width, height)
	}

	surface.mu.Lock()
	defer surface.mu.Unlock()

	if width, height, ok := surface.renderFrameLocked(true); ok || width != 0 || height != 0 {
		t.Fatalf("renderFrameLocked = %dx%d, %v while detached, want no frame", width, height, ok)
	}
}
