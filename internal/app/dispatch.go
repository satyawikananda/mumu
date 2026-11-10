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
//
// The main window's Update still runs fn after that window was destroyed (its
// redraw is the part that becomes a no-op), so the floating player is told to
// repaint separately. Invalidating it from here covers both cases: the PiP
// window is the only one that paints while the main window is closed, and it
// does not repaint from the main window's updates otherwise.
func (a *App) publish(fn func()) {
	a.mu.Lock()
	stopped, d, pip := a.stopped, a.dispatcher, a.pipDispatcher
	a.mu.Unlock()

	if stopped {
		return
	}
	if d == nil {
		slog.Warn("state update dropped: the window is not open yet")
		return
	}

	d.Update(fn)
	if pip != nil {
		pip.Invalidate()
	}
}
