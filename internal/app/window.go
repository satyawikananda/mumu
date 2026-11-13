package app

import (
	"log/slog"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"mumu/internal/ui"
	"mumu/internal/video"
)

// mainWindow is the native main window surface the app presents into.
type mainWindow interface {
	Show()
	Focus()
	Restore()
	ToggleFullScreen()
	NativeHandle() uintptr
	ContentBounds() mygo.Rectangle
}

// nativeMain adapts a MyGo window to mainWindow.
type nativeMain struct{ win *mygo.Window }

func (m nativeMain) Show()                 { m.win.Show() }
func (m nativeMain) Focus()                { m.win.Focus() }
func (m nativeMain) Restore()              { m.win.Restore() }
func (m nativeMain) ToggleFullScreen()     { m.win.ToggleFullScreen() }
func (m nativeMain) NativeHandle() uintptr { return m.win.NativeHandle() }
func (m nativeMain) ContentBounds() mygo.Rectangle {
	return m.win.ContentBounds()
}

// createMainWindow builds the single application window and its callbacks.
func (a *App) createMainWindow() (mainWindow, *mygo.Window) {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     "mumu",
		Width:     960,
		Height:    640,
		MinWidth:  640,
		MinHeight: 420,
		// Opens where the user left it last time.
		StateKey: "main",
		// Native UI only: no page and no web view.
		Content: myui.View(a.shell.View),
		// Dark canvas behind the first frame.
		BackgroundColor: ui.WindowBackground(),
	})

	win.OnResize(func() { a.publish(func() {}) })
	win.OnClosed(func() { a.mainWindowClosed(win) })

	return nativeMain{win: win}, win
}

// openWindow creates the application window, or recreates it after the viewer
// closed it while the floating player kept the session alive.
func (a *App) openWindow() {
	a.mu.Lock()
	stopped, open := a.stopped, a.win != nil
	a.mu.Unlock()

	if stopped || open {
		return
	}

	// The native window is created outside the lock; creating it touches
	// AppKit and may call back into the app.
	adapter, win := a.newMainWindow()

	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()

		return
	}
	a.win = adapter
	a.mainWin = win
	if win != nil {
		a.dispatcher = win
	}
	storeErr := a.storeErr
	a.mu.Unlock()

	if win != nil {
		if off, err := startActivityMonitor(win.NativeHandle(), a.noteWatchActivity); err != nil {
			slog.Debug("watch activity monitor unavailable", "error", err)
		} else {
			a.mu.Lock()
			a.mainActivityOff = off
			a.mu.Unlock()
		}
	}

	if storeErr != nil {
		a.shell.ShowError(storeMessage(storeErr))
	}
}

// mainWindowClosed forgets a destroyed main window and releases the surface it
// presented on. It runs on the UI thread from the window's close callback.
//
// The PiP window, the session and the audio keep running: MyGo only quits when
// the last window closes, and the floating player is still open.
func (a *App) mainWindowClosed(win *mygo.Window) {
	a.mu.Lock()
	if a.mainWin != win {
		// A newer window already replaced this one; do not clear it.
		a.mu.Unlock()

		return
	}
	a.mainWin = nil
	a.win = nil
	surface := a.surface
	off := a.mainActivityOff
	a.mainActivityOff = nil
	detach := a.media.attached && a.presentation == video.PresentationMain
	// The view went down with the window; the flag must say so.
	a.media.attached = false
	a.mu.Unlock()

	if off != nil {
		off()
	}

	if !detach || surface == nil {
		return
	}

	if err := surface.Detach(); err != nil {
		slog.Debug("detach video surface after the main window closed", "error", err)
	}
}
