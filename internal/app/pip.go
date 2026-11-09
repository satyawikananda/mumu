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
	OnResize(fn func())
	OnClose(fn func(e *mygo.CloseEvent))
}

// nativePip adapts a MyGo window to the presenter's interface.
type nativePip struct{ win *mygo.Window }

func (p nativePip) Show()                                  { p.win.Show() }
func (p nativePip) Hide()                                  { p.win.Hide() }
func (p nativePip) Close()                                 { p.win.Close() }
func (p nativePip) Focus()                                 { p.win.Focus() }
func (p nativePip) NativeHandle() uintptr                  { return p.win.NativeHandle() }
func (p nativePip) ContentBounds() mygo.Rectangle          { return p.win.ContentBounds() }
func (p nativePip) SetContentBounds(bounds mygo.Rectangle) { p.win.SetContentBounds(bounds) }
func (p nativePip) OnResize(fn func())                     { p.win.OnResize(fn) }
func (p nativePip) OnClose(fn func(e *mygo.CloseEvent)) {
	p.win.OnClose(fn)
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
		a.mu.Unlock()
	}

	a.mu.Lock()
	state := a.presentation
	a.mu.Unlock()

	err := video.Transfer(&state, video.PresentationPiP, func() error {
		pip.Show()
		pip.Focus()

		return surface.Attach(pip.NativeHandle(), pipFrame(pip))
	})

	a.mu.Lock()
	a.presentation = state
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
func (a *App) returnFromPiP() {
	a.mu.Lock()
	surface, pip, frame := a.surface, a.pipWindow, a.mainFrame
	a.mu.Unlock()

	if surface == nil {
		return
	}

	if a.win == nil {
		a.openWindow()
	}

	a.mu.Lock()
	main := a.win
	a.mu.Unlock()

	if main == nil {
		a.failWatch("The main window could not be restored.")

		return
	}

	a.mu.Lock()
	state := a.presentation
	a.mu.Unlock()

	err := video.Transfer(&state, video.PresentationMain, func() error {
		main.Restore()
		main.Show()

		target := frame
		if target.W <= 0 || target.H <= 0 {
			target = contentFrame(main)
		}

		return surface.Attach(main.NativeHandle(), target)
	})

	a.mu.Lock()
	a.presentation = state
	a.mu.Unlock()

	if err != nil {
		slog.Warn("return from picture-in-picture", "error", err)
		a.failWatch("The video could not return to the main window.")

		return
	}

	if pip != nil {
		pip.Hide()
	}

	a.publishWatch()
	main.Focus()
}

// closePiP ends the mini player. With a main window it returns there; without
// one it stops playback and quits, as the UX contract requires.
func (a *App) closePiP() {
	if a.win != nil {
		a.returnFromPiP()

		return
	}

	a.mu.Lock()
	pip := a.pipWindow
	a.pipWindow = nil
	a.presentation = video.PresentationStopped
	a.mu.Unlock()

	if pip != nil {
		pip.Close()
	}

	a.shutdownPlayback()
	mygo.RunOnMain(mygo.App.Quit)
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
	adapter.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		a.closePiP()
	})

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
