//go:build darwin

package app

import (
	"errors"

	"github.com/ebitengine/purego/objc"
)

// activityMonitorMask covers pointer movement, drags, scrolling and key
// presses. Press and release stay out: MyGo delivers those to the view, which
// decides whether a click reveals chrome or activates the hidden chevron.
const activityMonitorMask = 1<<5 | 1<<6 | 1<<7 | 1<<10 | 1<<22 | 1<<27

// startActivityMonitor reports input in one window until stop is called. It
// runs on AppKit's main thread and returns immediately from every event.
func startActivityMonitor(window uintptr, onActivity func(activityKind)) (stop func(), err error) {
	if window == 0 || onActivity == nil {
		return nil, errors.New("app: activity monitor needs a window and a callback")
	}

	nsWindow := objc.ID(window)
	// Tracking areas deliver moves, but the window must accept mouse-moved
	// events for the local monitor to see them too.
	nsWindow.Send(objc.RegisterName("setAcceptsMouseMovedEvents:"), true)

	block := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		if event != 0 && objc.Send[objc.ID](event, objc.RegisterName("window")) == nsWindow {
			kind := activityInput
			if objc.Send[uint64](event, objc.RegisterName("type")) == 5 { // NSEventTypeMouseMoved
				kind = activityMove
			}
			onActivity(kind)
		}

		return event
	})

	monitor := objc.ID(objc.GetClass("NSEvent")).Send(
		objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"),
		uintptr(activityMonitorMask), block,
	)
	if monitor == 0 {
		block.Release()

		return nil, errors.New("app: install activity monitor")
	}
	monitor.Send(objc.RegisterName("retain"))

	return func() {
		objc.ID(objc.GetClass("NSEvent")).Send(objc.RegisterName("removeMonitor:"), monitor)
		monitor.Send(objc.RegisterName("release"))
		block.Release()
	}, nil
}
