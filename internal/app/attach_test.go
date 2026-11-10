package app

import (
	"testing"

	"github.com/egoist/mygo"

	"mumu/internal/catalog"
	"mumu/internal/player"
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

// presentingApp wires a playing channel, a main window and a fake surface, so a
// test can move through playback states.
func presentingApp(t *testing.T) (*App, *fakeSurface) {
	t.Helper()

	a, _ := newTestApp()
	a.win = &fakeMain{native: 99}
	a.media.channel = catalog.Channel{SourceID: "s", ProviderID: "p", Name: "News"}
	a.media.snapshot = player.Snapshot{State: player.StatePlaying, Detail: "Playing"}
	a.mainFrame = video.Rect{X: 0, Y: 60, W: 800, H: 450}
	a.shell.ShowWatch()

	surface := &fakeSurface{}
	a.surface = surface

	return a, surface
}

// The video box reported before the engine exists is still attached on the
// first frame; this was the "stream cannot play" regression.
func TestAttachAfterLazySurfaceCreation(t *testing.T) {
	a := New()
	a.dispatcher = &fakeDispatcher{}
	a.win = &fakeMain{native: 99}
	a.media.channel = catalog.Channel{SourceID: "s", ProviderID: "p", Name: "News"}
	a.media.snapshot = player.Snapshot{State: player.StatePlaying}
	a.shell.ShowWatch()

	// The idle page reports the box before any channel or engine exists.
	a.applyVideoFrame(ui.Rect{X: 0, Y: 60, W: 800, H: 450})
	if a.media.attached {
		t.Fatal("attached without a surface")
	}

	surface := &fakeSurface{}
	a.surface = surface

	// The first frame arrives before the page reported a new box, so the
	// surface follows the state, not the layout.
	a.syncSurfaceVisibility()

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
	a, surface := presentingApp(t)
	a.media.channel = catalog.Channel{}

	a.syncSurfaceVisibility()

	if surface.attaches != 0 {
		t.Fatalf("attaches = %d without a channel, want 0", surface.attaches)
	}
	if a.media.attached {
		t.Fatal("attached without a channel")
	}
}

// The native view may cover the page only while the player presents frames.
// Buffering keeps the last frame, so it stays attached; the UI shows its badge
// in the control strip below the video.
func TestSurfaceAttachesOnlyWhilePresenting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state player.State
		want  bool
	}{
		{name: "idle", state: player.StateIdle},
		{name: "loading", state: player.StateLoading},
		{name: "error", state: player.StateError},
		{name: "stopping", state: player.StateStopping},
		{name: "stopped", state: player.StateStopped},
		{name: "playing", state: player.StatePlaying, want: true},
		{name: "buffering", state: player.StateBuffering, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, surface := presentingApp(t)
			a.media.snapshot = player.Snapshot{State: tc.state}

			a.syncSurfaceVisibility()

			if got := surface.attaches == 1; got != tc.want {
				t.Fatalf("attached = %t, want %t", got, tc.want)
			}
			if a.media.attached != tc.want {
				t.Fatalf("attached flag = %t, want %t", a.media.attached, tc.want)
			}
		})
	}
}

// Leaving the presenting states releases the last frame, so MyGo's loading and
// error overlays are not painted under the native view.
func TestSurfaceDetachesWhenPlaybackLeavesVideo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state player.State
	}{
		{name: "loading", state: player.StateLoading},
		{name: "error", state: player.StateError},
		{name: "stopping", state: player.StateStopping},
		{name: "stopped", state: player.StateStopped},
		{name: "idle", state: player.StateIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, surface := presentingApp(t)
			a.syncSurfaceVisibility()
			if surface.attaches != 1 || !a.media.attached {
				t.Fatalf("attaches = %d attached = %t, want one attach", surface.attaches, a.media.attached)
			}

			a.media.snapshot = player.Snapshot{State: tc.state}
			a.syncSurfaceVisibility()

			if !surface.detached {
				t.Fatal("surface stayed attached")
			}
			if a.media.attached {
				t.Fatal("attached flag survived a detach")
			}
		})
	}
}

// publishWatch moves the surface before the page state, so the overlay is
// never drawn under a view that is still attached.
func TestPublishWatchDetachesBeforeSettingState(t *testing.T) {
	a, surface := presentingApp(t)
	a.syncSurfaceVisibility()
	a.shell.Watch().SetState(ui.WatchState{Kind: ui.WatchPlaying})

	var kindAtDetach ui.WatchKind
	surface.onDetach = func() {
		kindAtDetach = a.shell.Watch().State().Kind
	}

	a.media.snapshot = player.Snapshot{State: player.StateError, Detail: "Stream failed", Error: "Stream failed"}
	a.publishWatch()

	if kindAtDetach != ui.WatchPlaying {
		t.Fatalf("surface detached while the page showed %v, want the previous state", kindAtDetach)
	}
	if got := a.shell.Watch().State().Kind; got != ui.WatchError {
		t.Fatalf("kind = %v, want Error", got)
	}
	if !surface.detached {
		t.Fatal("surface was not detached for the error overlay")
	}
}

// Leaving Watch detaches the view; returning re-attaches without a reload.
func TestNavigationDetachesSurface(t *testing.T) {
	a, surface := presentingApp(t)
	a.syncSurfaceVisibility()

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
	a, surface := presentingApp(t)
	a.presentation = video.PresentationPiP
	a.pipWindow = &fakePip{native: 7, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a.media.pipAttached = true

	a.watchVisibilityChanged(false)

	if surface.detached {
		t.Fatal("detached the surface while PiP was presenting")
	}
	if !a.media.pipAttached {
		t.Fatal("cleared the PiP flag while PiP was presenting")
	}
}
