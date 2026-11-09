package app

import (
	"errors"
	"testing"

	"github.com/egoist/mygo"

	"mumu/internal/catalog"
	"mumu/internal/player"
	"mumu/internal/ui"
	"mumu/internal/video"
)

// fakeSurface records attachment calls without native objects.
type fakeSurface struct {
	attaches int
	frames   []video.Rect
	fail     error
	errors   chan error
	closed   bool
}

func (f *fakeSurface) Attach(_ uintptr, frame video.Rect) error {
	f.attaches++
	f.frames = append(f.frames, frame)
	if f.fail != nil {
		return f.fail
	}

	return nil
}

func (f *fakeSurface) SetFrame(frame video.Rect) error {
	f.frames = append(f.frames, frame)

	return nil
}

func (f *fakeSurface) Size() (int, int) { return 640, 360 }

func (f *fakeSurface) Errors() <-chan error {
	if f.errors == nil {
		f.errors = make(chan error)
		close(f.errors)
	}

	return f.errors
}

func (f *fakeSurface) Close() { f.closed = true }

// fakePip records window operations without native objects.
type fakePip struct {
	shown, hidden, closed bool
	bounds                mygo.Rectangle
	native                uintptr
}

func (p *fakePip) Show()  { p.shown = true }
func (p *fakePip) Hide()  { p.hidden = true }
func (p *fakePip) Close() { p.closed = true }
func (p *fakePip) Focus() {}
func (p *fakePip) NativeHandle() uintptr {
	return p.native
}
func (p *fakePip) ContentBounds() mygo.Rectangle { return p.bounds }
func (p *fakePip) SetContentBounds(bounds mygo.Rectangle) {
	p.bounds = bounds
}
func (p *fakePip) OnResize(func())                {}
func (p *fakePip) OnClose(func(*mygo.CloseEvent)) {}

// pipTestApp wires one playing session and fakes for the surface and window.
func pipTestApp(t *testing.T, surface *fakeSurface, pip *fakePip) *App {
	t.Helper()

	a, _ := newTestApp()
	a.surface = surface
	a.pipWindow = pip
	a.media.channel = catalog.Channel{SourceID: "s", ProviderID: "p", Name: "News"}
	a.media.snapshot = player.Snapshot{State: player.StatePlaying, Detail: "Playing"}
	a.presentation = video.PresentationMain

	return a
}

// Entering PiP moves the one surface and marks the main window.
func TestEnterPiP(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)

	a.enterPiP()

	if a.presentation != video.PresentationPiP {
		t.Fatalf("presentation %s, want pip", a.presentation)
	}
	if surface.attaches != 1 {
		t.Fatalf("attaches = %d, want 1", surface.attaches)
	}
	if !pip.shown {
		t.Fatal("PiP window was not shown")
	}
	if surface.frames[0].W <= 0 || surface.frames[0].H <= 0 {
		t.Fatalf("frame %+v, want a positive video area", surface.frames[0])
	}
	if !a.shell.Watch().State().PiPActive {
		t.Fatal("Watch did not report PiP")
	}

	// A duplicate request must not attach again.
	a.enterPiP()
	if surface.attaches != 1 {
		t.Fatalf("attaches = %d after a duplicate, want 1", surface.attaches)
	}
}

// A failed attachment keeps the main presentation and reports an error.
func TestEnterPiPFailure(t *testing.T) {
	surface := &fakeSurface{fail: errors.New("no drawable")}
	pip := &fakePip{native: 7, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)

	a.enterPiP()

	if a.presentation != video.PresentationMain {
		t.Fatalf("presentation %s, want main after a failure", a.presentation)
	}
	if surface.attaches != 1 {
		t.Fatalf("attaches = %d, want one failed attempt", surface.attaches)
	}
	got := a.shell.Watch().State()
	if got.Kind != ui.WatchError || got.Error == "" {
		t.Fatalf("watch state %+v, want an actionable error", got)
	}
	if got.PiPActive {
		t.Fatal("Watch reported PiP after a failed attach")
	}
}

// The mini player never starts a second session: entering PiP touches only
// the surface and the window.
func TestEnterPiPDoesNotPlay(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 1, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)

	generation := a.media.snapshot.Generation
	a.enterPiP()

	if a.media.snapshot.Generation != generation {
		t.Fatalf("generation changed from %d to %d", generation, a.media.snapshot.Generation)
	}
	if a.controller != nil {
		t.Fatal("PiP created a controller")
	}
}
