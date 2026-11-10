package app

import (
	"testing"

	"github.com/egoist/mygo"

	"mumu/internal/catalog"
	"mumu/internal/ui"
	"mumu/internal/video"
)

// fakeMain records the main-window calls the app needs.
type fakeMain struct {
	native uintptr
	shown  bool
}

func (m *fakeMain) Show()             { m.shown = true }
func (m *fakeMain) Focus()            {}
func (m *fakeMain) Restore()          {}
func (m *fakeMain) ToggleFullScreen() {}
func (m *fakeMain) NativeHandle() uintptr {
	return m.native
}
func (m *fakeMain) ContentBounds() mygo.Rectangle {
	return mygo.Rectangle{Width: 960, Height: 640}
}

// The video box reported before the engine exists is still attached on the
// first Play; this was the "stream cannot play" regression.
func TestAttachAfterLazySurfaceCreation(t *testing.T) {
	a, _ := newTestApp()
	main := &fakeMain{native: 99}
	a.win = main

	// The idle page reports the box before any channel or engine exists.
	a.applyVideoFrame(ui.Rect{X: 0, Y: 60, W: 800, H: 450})
	if a.media.attached {
		t.Fatal("attached without a surface")
	}

	a.media.channel = catalog.Channel{SourceID: "s", ProviderID: "p", Name: "News"}
	surface := &fakeSurface{}
	a.surface = surface
	a.attachVideoSurface()

	if surface.attaches != 1 {
		t.Fatalf("attaches = %d, want 1", surface.attaches)
	}
	if !a.media.attached {
		t.Fatal("surface was not marked attached")
	}
	if surface.frames[0].W != 800 || surface.frames[0].H != 450 {
		t.Fatalf("frame %+v, want the reported 800x450 box", surface.frames[0])
	}

	// A later layout only resizes the attached surface.
	a.applyVideoFrame(ui.Rect{X: 0, Y: 70, W: 640, H: 360})
	if surface.attaches != 1 {
		t.Fatalf("attaches = %d after a resize, want 1", surface.attaches)
	}
	if surface.setFrames != 1 {
		t.Fatalf("setFrames = %d, want 1", surface.setFrames)
	}
}

// Without an active channel the surface stays detached.
func TestAttachNeedsActiveChannel(t *testing.T) {
	a, _ := newTestApp()
	a.win = &fakeMain{native: 1}
	surface := &fakeSurface{}
	a.surface = surface
	a.mainFrame = video.Rect{W: 800, H: 450}

	a.attachVideoSurface()

	if surface.attaches != 0 {
		t.Fatalf("attaches = %d without a channel, want 0", surface.attaches)
	}
}

// Leaving Watch detaches the view; returning re-attaches without a reload.
func TestNavigationDetachesSurface(t *testing.T) {
	a, _ := newTestApp()
	a.win = &fakeMain{native: 3}
	a.media.channel = catalog.Channel{SourceID: "s", ProviderID: "p", Name: "News"}

	surface := &fakeSurface{}
	a.surface = surface
	a.applyVideoFrame(ui.Rect{X: 0, Y: 60, W: 800, H: 450})
	a.attachVideoSurface()

	if surface.attaches != 1 {
		t.Fatalf("attaches = %d, want 1", surface.attaches)
	}

	a.watchVisibilityChanged(false)
	if !surface.detached {
		t.Fatal("surface was not detached when leaving Watch")
	}
	if a.media.attached {
		t.Fatal("attached flag survived a detach")
	}

	a.watchVisibilityChanged(true)
	if surface.attaches != 2 {
		t.Fatalf("attaches = %d after returning, want 2", surface.attaches)
	}
}

// While the surface presents in PiP, leaving the main Watch page must not
// detach it from the floating window.
func TestNavigationKeepsPiPSurface(t *testing.T) {
	a, _ := newTestApp()
	surface := &fakeSurface{}
	a.surface = surface
	a.media.channel = catalog.Channel{SourceID: "s", ProviderID: "p"}
	a.media.attached = true
	a.presentation = video.PresentationPiP

	a.watchVisibilityChanged(false)

	if surface.detached {
		t.Fatal("detached the surface while PiP was presenting")
	}
	if !a.media.attached {
		t.Fatal("cleared the attached flag while PiP was presenting")
	}
}
