package app

import (
	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"mumu/internal/ui"
)

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
	if a.storeErr != nil {
		a.shell.ShowError(storeMessage(a.storeErr))
	}
}
