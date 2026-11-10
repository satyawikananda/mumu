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
	attaches  int
	setFrames int
	frames    []video.Rect
	fail      error
	errors    chan error
	detached  bool
	closed    bool
	closes    int
	onDetach  func()
}

func (f *fakeSurface) Attach(_ uintptr, frame video.Rect) error {
	f.attaches++
	f.frames = append(f.frames, frame)
	if f.fail != nil {
		return f.fail
	}

	return nil
}

func (f *fakeSurface) Detach() error {
	f.detached = true
	if f.onDetach != nil {
		f.onDetach()
	}

	return nil
}

func (f *fakeSurface) SetFrame(frame video.Rect) error {
	f.setFrames++
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

func (f *fakeSurface) Close() {
	f.closed = true
	f.closes++
}

// fakePip records window operations without native objects.
//
// It mirrors the close-listener semantics the app relies on: Close fires the
// registered handler, which prevents the default and asks the app to close PiP.
// Close therefore only destroys the window once the listener is removed.
type fakePip struct {
	shown, hidden bool
	invalidates   int
	bounds        mygo.Rectangle
	native        uintptr

	onClose      func(*mygo.CloseEvent)
	handlerCalls int
	offs         int
	closeCalls   int
	closedCount  int
	closed       bool
	events       []string
}

func (p *fakePip) Show()  { p.shown = true }
func (p *fakePip) Hide()  { p.hidden = true }
func (p *fakePip) Focus() {}
func (p *fakePip) Invalidate() {
	p.invalidates++
}
func (p *fakePip) NativeHandle() uintptr {
	return p.native
}
func (p *fakePip) ContentBounds() mygo.Rectangle { return p.bounds }
func (p *fakePip) SetContentBounds(bounds mygo.Rectangle) {
	p.bounds = bounds
}
func (p *fakePip) OnResize(func()) {}

func (p *fakePip) OnClose(fn func(*mygo.CloseEvent)) func() {
	p.onClose = fn

	removed := false

	return func() {
		p.events = append(p.events, "off")
		if removed {
			return
		}
		removed = true
		p.offs++
		p.onClose = nil
	}
}

// Close is the native close request: the handler runs first, and the window is
// destroyed only when no handler prevents it.
func (p *fakePip) Close() {
	p.events = append(p.events, "close")
	p.closeCalls++

	if p.onClose != nil {
		p.events = append(p.events, "handler")
		p.handlerCalls++
		p.onClose(&mygo.CloseEvent{})

		return
	}

	p.closedCount++
	p.closed = true
}

// offBeforeLastClose reports that the listener was removed before the close
// that actually destroyed the window, so it could not reenter closePiP.
func (p *fakePip) offBeforeLastClose() bool {
	lastClose := -1
	off := -1
	for i, event := range p.events {
		switch event {
		case "close":
			lastClose = i
		case "off":
			off = i
		}
	}

	return off >= 0 && off < lastClose
}

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
	if a.media.attached {
		t.Fatal("the main window was still marked attached after moving to PiP")
	}
	if !a.media.pipAttached {
		t.Fatal("the PiP window was not marked attached")
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

// Closing the main window during PiP forgets the destroyed window, clears the
// surface flags and never leaves the app pointing at a dead handle.
func TestMainWindowClosedClearsWindowAndSurface(t *testing.T) {
	a, surface := presentingApp(t)
	a.syncSurfaceVisibility()
	if !a.media.attached {
		t.Fatal("surface was not attached before the close")
	}

	native := &mygo.Window{}
	a.mainWin = native

	a.mainWindowClosed(native)

	if a.mainWin != nil {
		t.Fatal("mainWin survived the close")
	}
	if a.win != nil {
		t.Fatal("window adapter survived the close")
	}
	if a.media.attached {
		t.Fatal("attached flag survived the close")
	}
	if !surface.detached {
		t.Fatal("surface was not detached from the destroyed window")
	}

	// A late close from an older window must not clear the replacement.
	replacement := &fakeMain{native: 5}
	a.win = replacement
	a.mainWin = &mygo.Window{}
	a.mainWindowClosed(native)
	if a.win == nil || a.mainWin == nil {
		t.Fatal("a stale close cleared the current window")
	}
}

// Return after the viewer closed the main window creates a new one, shows
// Watch, re-reports its video box and attaches exactly once.
func TestReturnFromPiPRecreatesMainWindow(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true
	a.shell.ShowDiscover()

	created := 0
	a.newMainWindow = func() (mainWindow, *mygo.Window) {
		created++

		return &fakeMain{native: 11}, nil
	}

	a.returnFromPiP()

	if created != 1 {
		t.Fatalf("windows created = %d, want 1", created)
	}
	if a.win == nil {
		t.Fatal("the recreated window was not installed")
	}
	if a.presentation != video.PresentationMain {
		t.Fatalf("presentation %s, want main", a.presentation)
	}
	if surface.attaches != 1 {
		t.Fatalf("attaches = %d, want 1", surface.attaches)
	}
	if !a.media.attached {
		t.Fatal("the recreated window was not marked attached")
	}
	if a.media.pipAttached {
		t.Fatal("the PiP window was still marked attached")
	}
	if !pip.hidden {
		t.Fatal("PiP stayed visible after the return")
	}
	if a.shell.Destination() != "/watch" {
		t.Fatalf("destination %q, want Watch", a.shell.Destination())
	}
	if !a.win.(*fakeMain).shown {
		t.Fatal("the recreated window was not shown")
	}
}

// A window that cannot be created leaves the viewer in PiP with the session
// playing instead of a half transition.
func TestReturnFromPiPWithoutWindowStaysInPiP(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true

	a.newMainWindow = func() (mainWindow, *mygo.Window) { return nil, nil }

	a.returnFromPiP()

	if a.win != nil {
		t.Fatal("installed a window that was not created")
	}
	if a.presentation != video.PresentationPiP {
		t.Fatalf("presentation %s, want pip", a.presentation)
	}
	if surface.attaches != 0 {
		t.Fatalf("attaches = %d, want 0", surface.attaches)
	}
	if !a.media.pipAttached || a.media.attached {
		t.Fatalf("attached = %t pipAttached = %t, want only PiP", a.media.attached, a.media.pipAttached)
	}
	if pip.hidden {
		t.Fatal("PiP was hidden without a main window")
	}
}

// PiP drops the stale frame for loading and error states, so the floating
// panel's status text is readable.
func TestPiPDetachesForOverlayStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state player.State
	}{
		{name: "loading", state: player.StateLoading},
		{name: "error", state: player.StateError},
		{name: "stopped", state: player.StateStopped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			surface := &fakeSurface{}
			pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
			a := pipTestApp(t, surface, pip)
			a.presentation = video.PresentationPiP
			a.media.pipAttached = true
			a.media.snapshot = player.Snapshot{State: tc.state}

			a.syncSurfaceVisibility()

			if !surface.detached {
				t.Fatal("the PiP surface stayed attached")
			}
			if a.media.pipAttached {
				t.Fatal("the PiP flag survived a detach")
			}
		})
	}
}

// The return path never leaves the presentation mid-transition, even when the
// attach fails and rolls back.
func TestReturnFromPiPKeepsPiPOnAttachFailure(t *testing.T) {
	surface := &fakeSurface{fail: errors.New("no drawable")}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true
	a.win = &fakeMain{native: 11}

	a.returnFromPiP()

	if a.presentation != video.PresentationPiP {
		t.Fatalf("presentation %s, want pip after the rollback", a.presentation)
	}
	if !a.media.pipAttached || a.media.attached {
		t.Fatalf("attached = %t pipAttached = %t, want the PiP state", a.media.attached, a.media.pipAttached)
	}
	if pip.hidden {
		t.Fatal("PiP was hidden after a failed return")
	}
	if err := a.watchState().Error; err != "" {
		t.Fatalf("watch error %q, want none: a failed return is not a playback failure", err)
	}
}

// After the main window is destroyed its redraw is a no-op, but the floating
// player must still repaint: publish runs the state callback once and then
// invalidates the PiP window.
func TestPublishInvalidatesPiPAfterMainWindowClosed(t *testing.T) {
	a, _ := presentingApp(t)
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a.pipWindow = pip
	a.pipDispatcher = pip

	native := &mygo.Window{}
	a.mainWin = native
	a.mainWindowClosed(native)

	if a.win != nil {
		t.Fatal("main window survived the close")
	}
	if a.media.attached {
		t.Fatal("surface stayed attached to the closed window")
	}

	a.publishWatch()

	if pip.invalidates != 1 {
		t.Fatalf("PiP invalidations = %d, want 1", pip.invalidates)
	}
	if got := a.shell.PiP().State().Channel; got != "News" {
		t.Fatalf("PiP channel %q, want the published state", got)
	}
}

// A state update without a floating player is still a no-op there.
func TestPublishWithoutPiP(t *testing.T) {
	a, _ := newTestApp()

	ran := 0
	a.publish(func() { ran++ })

	if ran != 1 {
		t.Fatalf("callback ran %d times, want 1", ran)
	}
}

// The floating player adds a repaint, never a second state callback.
func TestPublishRunsCallbackOnceWithPiP(t *testing.T) {
	a, _ := newTestApp()
	pip := &fakePip{}
	a.pipDispatcher = pip

	ran := 0
	a.publish(func() { ran++ })

	if ran != 1 {
		t.Fatalf("callback ran %d times, want 1", ran)
	}
	if pip.invalidates != 1 {
		t.Fatalf("invalidations = %d, want 1", pip.invalidates)
	}
}

// Releasing the floating player stops invalidating the destroyed window.
func TestClosePiPStopsInvalidatingTheClosedWindow(t *testing.T) {
	a := pipTestApp(t, &fakeSurface{}, &fakePip{})
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a.pipWindow = pip
	a.pipDispatcher = pip

	// A unit test cannot reach App.Quit: RunOnMain blocks without an event
	// loop, so the teardown closePiP performs is exercised directly.
	a.releasePiPWindow()

	if !pip.closed {
		t.Fatal("the floating window was not closed")
	}
	if a.pipWindow != nil || a.pipDispatcher != nil {
		t.Fatalf("pipWindow = %v pipDispatcher = %v, want both cleared", a.pipWindow, a.pipDispatcher)
	}

	before := pip.invalidates
	a.publish(func() {})
	if pip.invalidates != before {
		t.Fatalf("invalidations = %d, want none after the close", pip.invalidates-before)
	}
}

// registerPiPClose installs the handler newPiPWindow registers, so a test can
// simulate a user close through the fake window.
func registerPiPClose(a *App, pip *fakePip) {
	a.pipOffClose = pip.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		a.closePiP()
	})
}

// A user close with no main window stops playback once, closes the window once
// and quits once. The listener must go before the app's own close, or the close
// would reenter closePiP and never finish.
func TestClosePiPWithoutMainWindowClosesOnce(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true

	quits := 0
	a.quit = func() { quits++ }
	registerPiPClose(a, pip)

	// The viewer closes the floating window.
	pip.Close()

	if !pip.closed || pip.closedCount != 1 {
		t.Fatalf("closed = %t close count = %d, want one destroyed window", pip.closed, pip.closedCount)
	}
	if pip.handlerCalls != 1 {
		t.Fatalf("close handler ran %d times, want one user close and no reentry", pip.handlerCalls)
	}
	if pip.offs != 1 {
		t.Fatalf("listener removals = %d, want 1", pip.offs)
	}
	if !pip.offBeforeLastClose() {
		t.Fatalf("close events %v, want the listener removed before the app close", pip.events)
	}
	if surface.closes != 1 {
		t.Fatalf("surface closes = %d, want playback stopped once", surface.closes)
	}
	if quits != 1 {
		t.Fatalf("quit calls = %d, want 1", quits)
	}
	if a.pipWindow != nil || a.pipOffClose != nil {
		t.Fatalf("pipWindow set = %t, close listener set = %t; want both cleared", a.pipWindow != nil, a.pipOffClose != nil)
	}
}

// A second close request does nothing: the window is already released.
func TestClosePiPTwiceIsIdempotent(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true

	quits := 0
	a.quit = func() { quits++ }
	registerPiPClose(a, pip)

	pip.Close()
	a.closePiP()

	if quits != 1 {
		t.Fatalf("quit calls = %d after a duplicate close, want 1", quits)
	}
	if pip.closedCount != 1 {
		t.Fatalf("closed windows = %d, want 1", pip.closedCount)
	}
	if surface.closes != 1 {
		t.Fatalf("surface closes = %d, want 1", surface.closes)
	}
}

// The quit sequence releases the listener before it asks windows to close, so
// Cmd+Q with PiP open can actually finish.
func TestStopReleasesPiPCloseListener(t *testing.T) {
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, &fakeSurface{}, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true

	quits := 0
	a.quit = func() { quits++ }
	registerPiPClose(a, pip)

	a.Stop()

	// prepareQuit closes the floating window after OnBeforeQuit returned.
	pip.Close()

	if pip.onClose != nil {
		t.Fatal("the close listener survived Stop")
	}
	if !pip.closed || pip.closedCount != 1 {
		t.Fatalf("closed = %t closed windows = %d, want the quit close to pass", pip.closed, pip.closedCount)
	}
	if quits != 0 {
		t.Fatalf("quit calls = %d, want none from closing during quit", quits)
	}
}

// Closing PiP with a main window returns to it instead of quitting.
func TestClosePiPWithMainWindowReturns(t *testing.T) {
	surface := &fakeSurface{}
	pip := &fakePip{native: 42, bounds: mygo.Rectangle{Width: 480, Height: 318}}
	a := pipTestApp(t, surface, pip)
	a.presentation = video.PresentationPiP
	a.media.pipAttached = true
	a.win = &fakeMain{native: 11}
	a.mainFrame = video.Rect{X: 0, Y: 60, W: 800, H: 450}

	quits := 0
	a.quit = func() { quits++ }
	registerPiPClose(a, pip)

	a.closePiP()

	if quits != 0 {
		t.Fatalf("quit calls = %d, want a return instead", quits)
	}
	if pip.closedCount != 0 || !pip.hidden {
		t.Fatalf("closed windows = %d hidden = %t, want the window kept and hidden", pip.closedCount, pip.hidden)
	}
	if a.presentation != video.PresentationMain {
		t.Fatalf("presentation %s, want main", a.presentation)
	}
	if !a.media.attached {
		t.Fatal("the main window was not marked attached")
	}
}

// A native close arrives inside AppKit's windowShouldClose:, so the teardown
// must be posted: destroying the window from the handler would close it
// reentrantly, which hung the app before this fix.
func TestPiPCloseRequestDefersTeardown(t *testing.T) {
	a, _ := newTestApp()

	queue := &queuedDispatcher{}
	a.dispatcher = queue

	quits := 0
	a.quit = func() { quits++ }

	surface := &fakeSurface{}
	pip := &fakePip{bounds: mygo.Rectangle{Width: 480, Height: 318}}

	a.mu.Lock()
	a.surface = surface
	a.pipWindow = pip
	a.pipDispatcher = pip
	a.pipOffClose = pip.OnClose(a.pipCloseRequest)
	a.presentation = video.PresentationPiP
	a.mu.Unlock()

	// The viewer clicks the floating window's close control.
	pip.Close()

	if pip.closed {
		t.Fatal("the window was destroyed inside its own close request")
	}
	if surface.closes != 0 || quits != 0 {
		t.Fatalf("surface closes = %d, quits = %d, want the teardown deferred", surface.closes, quits)
	}
	if len(queue.fns) != 1 {
		t.Fatalf("queued updates = %d, want the posted teardown", len(queue.fns))
	}

	// The posted turn runs after the close request returned.
	queue.flush()

	if !pip.closed {
		t.Fatal("the floating window was not closed")
	}
	if surface.closes != 1 {
		t.Fatalf("surface closes = %d, want playback stopped once", surface.closes)
	}
	if quits != 1 {
		t.Fatalf("quits = %d, want exactly one quit", quits)
	}
	if pip.handlerCalls != 1 {
		t.Fatalf("close handler ran %d times, want 1", pip.handlerCalls)
	}
	if !pip.offBeforeLastClose() {
		t.Fatalf("events %v, want the listener removed before the destroying close", pip.events)
	}

	a.mu.Lock()
	pipWindow, pipDispatcher := a.pipWindow, a.pipDispatcher
	a.mu.Unlock()
	if pipWindow != nil || pipDispatcher != nil {
		t.Fatalf("pipWindow = %v pipDispatcher = %v, want both cleared", pipWindow, pipDispatcher)
	}
}
