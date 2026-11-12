//go:build darwin && arm64

package main

import (
	"errors"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// nsRect is NSRect: four CGFloat fields, in the same order AppKit lays them
// out.
type nsRect struct{ X, Y, W, H float64 }

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

// hostedView is a clipping container for the retained view.
//
// A radius needs a layer that masks its subview. NSOpenGLView is not a
// layer-backed view, so the experiment wraps it in a plain NSView whose layer
// masks to the corner radius and carries the video as a subview.
type hostedView struct {
	container objc.ID
}

// newHostedView creates the container and keeps it until close.
func newHostedView(radius float64) *hostedView {
	container := objc.ID(objc.GetClass("NSView")).Send(objc.RegisterName("alloc"))
	container = container.Send(objc.RegisterName("initWithFrame:"), nsRect{})
	if container == 0 {
		return nil
	}
	container.Send(objc.RegisterName("setWantsLayer:"), true)
	if layer := container.Send(objc.RegisterName("layer")); layer != 0 {
		layer.Send(objc.RegisterName("setMasksToBounds:"), true)
		layer.Send(objc.RegisterName("setCornerRadius:"), radius)
	}

	return &hostedView{container: container}
}

// attach puts the video view into the container, sized to the content view,
// and inserts the container under MyGo's surface.
func (h *hostedView) attach(window, view uintptr) error {
	if h == nil || h.container == 0 {
		return errors.New("compose: no host container")
	}
	content := objc.ID(window).Send(objc.RegisterName("contentView"))
	if content == 0 {
		return errors.New("compose: window has no content view")
	}

	bounds := objc.Send[nsRect](content, objc.RegisterName("bounds"))
	h.container.Send(objc.RegisterName("setFrame:"), nsRect{W: bounds.W, H: bounds.H})

	video := objc.ID(view)
	video.Send(objc.RegisterName("removeFromSuperview"))
	h.container.Send(objc.RegisterName("addSubview:"), video)
	video.Send(objc.RegisterName("setFrame:"), nsRect{W: bounds.W, H: bounds.H})
	video.Send(objc.RegisterName("setAutoresizingMask:"), uintptr(2|16)) // width|height sizable

	h.container.Send(objc.RegisterName("removeFromSuperview"))
	content.Send(
		objc.RegisterName("addSubview:positioned:relativeTo:"),
		h.container, nsWindowBelow, objc.ID(0),
	)

	return nil
}

// detach removes the container from its window and keeps the video view in it.
func (h *hostedView) detach() {
	if h == nil || h.container == 0 {
		return
	}
	h.container.Send(objc.RegisterName("removeFromSuperview"))
}
