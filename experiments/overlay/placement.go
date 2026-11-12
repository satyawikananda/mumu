//go:build darwin && arm64

package main

import (
	"log"

	"github.com/egoist/mygo/ui"

	"mumu/internal/video"
)

// viewAdapter exposes the session's retained view to MyGo's native element.
//
// ui.NativeView asks for the handle on every build. The session is created
// after the window exists, so a build before it returns 0 and the element
// paints as a plain box; the following redraw shows the view.
type viewAdapter struct {
	// handle is the session's view lookup. It reports 0 until the session
	// exists and after the surface closes.
	handle func() uintptr
}

// NativeView returns the NSView pointer, as ui.PlatformView requires. A nil
// adapter reports no view, so the element stays a plain box.
func (v *viewAdapter) NativeView() uintptr {
	if v == nil || v.handle == nil {
		return 0
	}

	return v.handle()
}

// videoElement builds the video box.
//
// In nativeview mode the retained view is a native element: MyGo places it
// under its surface and clips it to this element, so chrome built after it
// paints above live video. In below mode the app inserts the view under the
// surface itself and the element paints nothing. In above mode the box is an
// opaque placeholder and the app attaches the view over the window content,
// which is the comparison case.
func (a *app) videoElement(c *ui.Context) {
	switch a.s.Mode {
	case modeAbove:
		ui.Column(c).Fill().Background(ui.RGB(9, 9, 11))

		return
	case modeBelow:
		// The view is inserted under the surface by the app; building an
		// element here would paint over it.
		return
	}

	ui.NativeView(c, a.adapter).Fill()
	a.syncHostedView()
}

// syncHostedView refreshes the drawable of a view that MyGo lays out itself.
//
// It runs on the UI thread during a build, which is the only place the hosted
// view's bounds are current. It is bounded work: read the bounds, update the
// drawable size and ask the render thread for at most one redraw, so a rebuild
// cannot turn into an unbounded loop or a goroutine.
func (a *app) syncHostedView() {
	if a.sess == nil {
		return
	}
	if err := a.sess.syncDrawable(); err != nil && err != video.ErrClosed {
		log.Printf("build: sync drawable: %v", err)

		return
	}

	width, height := drawableSize(a.sess.surface)
	if width == a.s.DrawableW && height == a.s.DrawableH {
		return
	}
	log.Printf("build: drawable %dx%d", width, height)
	a.s.DrawableW, a.s.DrawableH = width, height
}

// surfaceHandle reports the retained view pointer for the adapter.
func (a *app) surfaceHandle() uintptr {
	if a.sess == nil || a.sess.surface == nil {
		return 0
	}

	return a.sess.surface.NativeViewHandle()
}

// drawableSize reports the current backing pixels, for the status line.
func drawableSize(s video.Surface) (int, int) {
	if s == nil {
		return 0, 0
	}

	return s.Size()
}
