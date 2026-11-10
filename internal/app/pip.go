package app

import (
	"log/slog"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"mumu/internal/ui"
	"mumu/internal/video"
)

// pipWindow is the native floating window the presenter needs.
type pipWindow interface {
	Show()
	Hide()
	Close()
	Focus()
	NativeHandle() uintptr
	ContentBounds() mygo.Rectangle
	SetContentBounds(bounds mygo.Rectangle)
	// Invalidate asks the floating window for a new frame without running a
	// state callback. The app's own publish path already ran that callback on
	// the main window, whose redraw stops working once it is destroyed.
	Invalidate()
	OnResize(fn func())
	// OnClose registers a close handler and returns the function that removes
	// it. A close the app asks for must be able to run without its own handler
	// preventing it, so the registration is removable.
	OnClose(fn func(e *mygo.CloseEvent)) (off func())
}

// nativePip adapts a MyGo window to the presenter's interface.
type nativePip struct{ win *mygo.Window }

func (p nativePip) Show()                                  { p.win.Show() }
func (p nativePip) Hide()                                  { p.win.Hide() }
func (p nativePip) Close()                                 { p.win.Close() }
func (p nativePip) Focus()                                 { p.win.Focus() }
func (p nativePip) Invalidate()                            { p.win.Invalidate() }
func (p nativePip) NativeHandle() uintptr                  { return p.win.NativeHandle() }
func (p nativePip) ContentBounds() mygo.Rectangle          { return p.win.ContentBounds() }
func (p nativePip) SetContentBounds(bounds mygo.Rectangle) { p.win.SetContentBounds(bounds) }
func (p nativePip) OnResize(fn func())                     { p.win.OnResize(fn) }
func (p nativePip) OnClose(fn func(e *mygo.CloseEvent)) func() {
	return p.win.OnClose(fn)
}

const (
	// pipInitialWidth is the floating player's starting content width.
	pipInitialWidth = 480
	// pipStripHeight is the mini player's control strip, in points.
	pipStripHeight = 48
)

// pipState builds the mini player state from the shared session.
func (a *App) pipState() ui.PiPState {
	a.mu.Lock()
	defer a.mu.Unlock()

	return ui.PiPState{
		Channel: a.media.channel.Name,
		Status:  a.media.snapshot.Detail,
		Muted:   a.media.snapshot.Muted,
		Error:   a.media.snapshot.Error,
	}
}

// enterPiP moves the one surface into a floating window.
func (a *App) enterPiP() {
	a.mu.Lock()
	surface, active := a.surface, a.media.channel.ProviderID != ""
	pip := a.pipWindow
	a.mu.Unlock()

	if surface == nil || !active {
		return
	}

	if pip == nil {
		pip = a.newPiPWindow()
		if pip == nil {
			a.failWatch("Picture-in-Picture could not open.")

			return
		}

		a.mu.Lock()
		a.pipWindow = pip
		// The floating window paints independently once the main window is
		// closed, so it is also the second redraw target for state updates.
		a.pipDispatcher = pip
		a.mu.Unlock()
	}

	a.mu.Lock()
	state := a.presentation
	a.mu.Unlock()

	attached := false
	err := video.Transfer(&state, video.PresentationPiP, func() error {
		pip.Show()
		pip.Focus()

		if err := surface.Attach(pip.NativeHandle(), pipFrame(pip)); err != nil {
			return err
		}
		attached = true

		return nil
	})

	a.mu.Lock()
	a.presentation = state
	if attached {
		// The retained view is in the floating window now; the main window has
		// no drawable until a return attaches it again.
		a.media.attached = false
		a.media.pipAttached = true
	}
	a.mu.Unlock()

	if err != nil {
		slog.Warn("enter picture-in-picture", "error", err)
		a.failWatch("Picture-in-Picture could not start.")

		return
	}

	a.publishWatch()
}

// returnFromPiP moves the surface back to the main window, recreating it when
// the viewer closed it earlier.
//
// The page reports its video box only on a layout pass, so the app shows Watch,
// forgets the stale box and then binds the retained view to the new window. A
// failed attach leaves the presentation in PiP, the floating player untouched
// and the session playing: there is no half state to clean up.
func (a *App) returnFromPiP() {
	a.mu.Lock()
	surface, pip, frame, haveWindow := a.surface, a.pipWindow, a.mainFrame, a.win != nil
	a.mu.Unlock()

	if surface == nil {
		return
	}

	if !haveWindow {
		a.openWindow()
	}

	a.mu.Lock()
	main := a.win
	a.mu.Unlock()

	if main == nil {
		// Stay in PiP rather than dropping the picture or leaving a
		// transition behind; the viewer can try Return again.
		slog.Warn("return from picture-in-picture: the main window is unavailable")

		return
	}

	// Show Watch and forget the box before the transfer, so the recreated page
	// lays out and reports the video area again. Both calls are UI-thread work,
	// as is the transfer that follows.
	a.shell.ShowWatch()
	a.shell.Watch().ResetFrame()

	a.mu.Lock()
	state := a.presentation
	a.mu.Unlock()

	attached := false
	err := video.Transfer(&state, video.PresentationMain, func() error {
		main.Restore()
		main.Show()

		target := frame
		if target.W <= 0 || target.H <= 0 {
			target = contentFrame(main)
		}

		if err := surface.Attach(main.NativeHandle(), target); err != nil {
			return err
		}
		attached = true

		return nil
	})

	a.mu.Lock()
	a.presentation = state
	if attached {
		a.media.attached = true
		a.media.pipAttached = false
	}
	a.mu.Unlock()

	if err != nil {
		// The floating player keeps the session and the view: Attach fails
		// before it reparents, so there is no drawable to repair. The viewer
		// can press Return again.
		slog.Warn("return from picture-in-picture", "error", err)

		return
	}

	if pip != nil {
		pip.Hide()
	}

	a.publishWatch()

	if !a.shell.IsWatch() {
		a.detachVideoSurface()
	}

	main.Focus()
}

// closePiP ends the mini player. With a main window it returns there; without
// one it stops playback and quits, as the UX contract requires.
//
// It is idempotent: a released PiP window means there is nothing left to close,
// so a second request never stops playback or asks to quit twice.
func (a *App) closePiP() {
	a.mu.Lock()
	pip, haveWindow := a.pipWindow, a.win != nil
	quit := a.quit
	a.mu.Unlock()

	if pip == nil {
		return
	}

	if haveWindow {
		a.returnFromPiP()

		return
	}

	// Stop playback while the floating drawable is still alive: releasing the
	// native view from a destroyed window is a hang risk the teardown does not
	// need to take.
	a.shutdownPlayback()
	a.releasePiPWindow()

	if quit != nil {
		quit()
	}
}

// releasePiPWindow closes the floating window and forgets it as a redraw
// target, so no later publish invalidates a destroyed window.
//
// The close listener goes first: a close the app asks for must not be prevented
// by the handler that turns a user close into closePiP, which would call this
// path again and leave the window open.
func (a *App) releasePiPWindow() {
	a.detachPiPClose()

	a.mu.Lock()
	pip := a.pipWindow
	a.pipWindow = nil
	a.pipDispatcher = nil
	a.presentation = video.PresentationStopped
	a.media.attached = false
	a.media.pipAttached = false
	a.mu.Unlock()

	if pip != nil {
		pip.Close()
	}
}

// detachPiPClose removes the floating window's close listener once. Stop calls
// it too, so the quit sequence can close the window normally instead of having
// its own request prevented.
func (a *App) detachPiPClose() {
	a.mu.Lock()
	off := a.pipOffClose
	a.pipOffClose = nil
	a.mu.Unlock()

	if off != nil {
		off()
	}
}

// pipCloseRequest runs when the floating window asks to close, either from the
// native title bar or from the panel's own action.
//
// AppKit calls the listener from windowShouldClose:, so tearing the window down
// on the spot would destroy it reentrantly inside its own close request. The
// request is prevented and the teardown is posted instead, running on the next
// main-thread turn once the close handler has returned.
func (a *App) pipCloseRequest(e *mygo.CloseEvent) {
	e.PreventDefault()
	a.publish(a.closePiP)
}

// newPiPWindow creates the native floating player once.
func (a *App) newPiPWindow() pipWindow {
	height := pipInitialWidth*9/16 + pipStripHeight

	pip := mygo.NewWindow(mygo.WindowOptions{
		Title:           "mumu · Picture-in-Picture",
		Width:           pipInitialWidth,
		Height:          height,
		UseContentSize:  true,
		MinWidth:        320,
		MinHeight:       320*9/16 + pipStripHeight,
		Panel:           true,
		AlwaysOnTop:     true,
		BackgroundColor: "#09090B",
		Content:         myui.View(a.shell.PiPView),
	})

	adapter := nativePip{win: pip}
	adapter.OnResize(func() { a.resizePiP(adapter) })

	off := adapter.OnClose(a.pipCloseRequest)

	a.mu.Lock()
	a.pipOffClose = off
	a.mu.Unlock()

	return adapter
}

// resizePiP keeps the video area at 16:9 and moves the surface with it.
func (a *App) resizePiP(pip pipWindow) {
	bounds := pip.ContentBounds()
	if bounds.Width <= 0 {
		return
	}

	want := bounds.Width*9/16 + pipStripHeight
	if bounds.Height != want {
		bounds.Height = want
		pip.SetContentBounds(bounds)
	}

	a.mu.Lock()
	surface := a.surface
	state := a.presentation
	a.mu.Unlock()

	if surface == nil || state != video.PresentationPiP {
		return
	}
	if err := surface.SetFrame(pipFrame(pip)); err != nil {
		slog.Debug("resize picture-in-picture", "error", err)
	}
}

// pipFrame returns the 16:9 video area above the control strip.
func pipFrame(pip pipWindow) video.Rect {
	return video.Fit(video.Rect{
		W: float32(pip.ContentBounds().Width),
		H: float32(pip.ContentBounds().Height - pipStripHeight),
	}, videoAspect)
}

// contentFrame is the fallback main-window frame before the page reports one.
func contentFrame(win mainWindow) video.Rect {
	bounds := win.ContentBounds()

	return video.Fit(video.Rect{
		W: float32(bounds.Width),
		H: float32(bounds.Height),
	}, videoAspect)
}

// togglePiPMute forwards the mini player's mute action to the shared player.
func (a *App) togglePiPMute() {
	a.togglePlaybackMute()
}
