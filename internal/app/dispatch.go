package app

import (
	"log/slog"

	"github.com/egoist/mygo"
)

// Dispatcher publishes state changes to the native UI.
type Dispatcher interface {
	// Update runs fn on the UI thread and redraws.
	Update(fn func())
	// Invalidate redraws without changing state.
	Invalidate()
}

var _ Dispatcher = (*mygo.Window)(nil)

// publish runs fn on the UI thread; drops updates with no usable window.
func (a *App) publish(fn func()) {
	a.mu.Lock()
	stopped, d := a.stopped, a.dispatcher
	a.mu.Unlock()

	if stopped {
		return
	}
	if d == nil {
		slog.Warn("state update dropped: the window is not open yet")
		return
	}

	d.Update(fn)
}
