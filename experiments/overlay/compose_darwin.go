//go:build darwin && arm64

package main

import (
	"errors"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// nsWindowBelow is NSWindowBelow, the otherView of
// addSubview:positioned:relativeTo:.
const nsWindowBelow = ^uintptr(0)

// demoteBelowSurface moves the retained video view to the bottom of the
// content view's subviews, under MyGo's own surface view.
//
// MyGo's surface hitTest: returns nil where a placed native view shows, which
// hands AppKit's clicks to that view. A view only stays clickable-through when
// MyGo did not place it, so the experiment inserts the view itself instead of
// using ui.NativeView.
func demoteBelowSurface(window, view uintptr) error {
	if window == 0 || view == 0 {
		return errors.New("compose: window or view is zero")
	}

	content := objc.ID(window).Send(objc.RegisterName("contentView"))
	if content == 0 {
		return errors.New("compose: window has no content view")
	}

	// relativeTo nil with NSWindowBelow puts the view at the bottom of the
	// subview list, under the MyGo surface.
	objc.ID(view).Send(
		objc.RegisterName("removeFromSuperview"),
	)
	content.Send(
		objc.RegisterName("addSubview:positioned:relativeTo:"),
		objc.ID(view), nsWindowBelow, objc.ID(0),
	)

	// The view was just reparented, so its window and scale changed.
	objc.ID(view).Send(objc.RegisterName("setNeedsDisplay:"), true)

	return nil
}

// surfaceClass reports whether view is MyGo's own surface view. Kept for the
// probe that records the subview order in evidence.
func surfaceClass(view uintptr) string {
	if view == 0 {
		return ""
	}
	class := objc.ID(view).Send(objc.RegisterName("className"))
	if class == 0 {
		return ""
	}
	// NSString is laid out as a pointer to UTF-8 bytes.
	return cString(objc.Send[unsafe.Pointer](class, objc.RegisterName("UTF8String")))
}

// cString copies a NUL-terminated C string into a Go string.
func cString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	var b []byte
	for i := 0; ; i++ {
		ch := *(*byte)(unsafe.Add(p, i))
		if ch == 0 {
			return string(b)
		}
		b = append(b, ch)
	}
}

// subviewOrder lists the content view's subviews, bottom to top, by class. It
// is the recorded proof of z-order: the retained NSOpenGLView must sit below
// MyGoSurfaceView.
func subviewOrder(window uintptr) []string {
	content := objc.ID(window).Send(objc.RegisterName("contentView"))
	if content == 0 {
		return nil
	}
	subviews := content.Send(objc.RegisterName("subviews"))
	if subviews == 0 {
		return nil
	}
	count := int(objc.Send[uint64](subviews, objc.RegisterName("count")))
	order := make([]string, 0, count)
	for i := range count {
		view := objc.Send[objc.ID](subviews, objc.RegisterName("objectAtIndex:"), uint64(i))
		order = append(order, surfaceClass(uintptr(view)))
	}

	return order
}
