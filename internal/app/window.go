package app

import (
	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"mumu/internal/ui"
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

// openWindow creates the single application window.
func (a *App) openWindow() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return
	}

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

	a.dispatcher = win
	a.win = nativeMain{win: win}
	win.OnResize(func() { a.publish(func() {}) })
	win.OnClosed(func() {
		a.mu.Lock()
		if a.win == win {
			a.win = nil
		}
		a.mu.Unlock()
	})
	if a.storeErr != nil {
		a.shell.ShowError(storeMessage(a.storeErr))
	}
}
