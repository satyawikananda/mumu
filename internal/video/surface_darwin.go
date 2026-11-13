//go:build darwin && arm64

package video

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"mumu/internal/player"
)

// openGLFramework is the system framework the M0 bridge proved. macOS OpenGL
// is deprecated; keeping the path in one constant makes it replaceable.
const openGLFramework = "/System/Library/Frameworks/OpenGL.framework/OpenGL"

// NSOpenGL.h attribute values: accelerated, double buffered, 24-bit colour and
// an OpenGL 3.2 core profile, matching the M0 spike.
const (
	openGLPixelFormatAttributes = 73
	openGLDoubleBuffer          = 5
	openGLColorSize             = 8
	openGLAlphaSize             = 24
	openGLProfile               = 99
	openGLProfileCore32         = 0x3200
	initialFrameWidth           = 640
	initialFrameHeight          = 360
)

type point struct{ X, Y float64 }
type size struct{ Width, Height float64 }
type rect struct {
	Origin point
	Size   size
}

// nativeSurface is the retained NSOpenGLView and its single render thread.
//
// AppKit work happens on the main thread; drawing happens on the driver's
// locked thread. Both take the Go mutex and the native CGL lock, because AppKit
// also touches the drawable and a Go mutex alone did not prevent a native crash
// during M0 capture.
type nativeSurface struct {
	view    objc.ID
	context objc.ID
	cgl     uintptr

	gl            uintptr
	proc          func(handle uintptr, name string) unsafe.Pointer
	lockContext   func(uintptr) int32
	unlockContext func(uintptr) int32

	mu sync.Mutex
	// state is guarded by mu: it is the drawable size and pending redraw the
	// render thread reads under the same lock.
	state drawable

	driver    *driver
	closeOnce sync.Once
	closed    chan struct{}
}

var _ Surface = (*nativeSurface)(nil)

// surfaceSupported reports that this build carries the darwin/arm64 bridge.
const surfaceSupported = true

// newSurface creates the retained view and starts its render thread.
func newSurface(renderer player.Renderer) (Surface, error) {
	if renderer == nil {
		return nil, errors.New("video: renderer is nil")
	}
	if !isMainThread() {
		return nil, ErrNotMainThread
	}

	surface, err := newNativeSurface()
	if err != nil {
		return nil, err
	}

	driver, err := startDriver(surface, renderer)
	if err != nil {
		_ = surface.releaseNative()

		return nil, err
	}
	surface.driver = driver

	return surface, nil
}

func newNativeSurface() (*nativeSurface, error) {
	gl, err := purego.Dlopen(openGLFramework, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("video: load OpenGL framework: %w", err)
	}

	attrs := []uint32{
		openGLPixelFormatAttributes, openGLDoubleBuffer,
		openGLColorSize, openGLAlphaSize,
		openGLProfile, openGLProfileCore32, 0,
	}
	format := objc.ID(objc.GetClass("NSOpenGLPixelFormat")).Send(objc.RegisterName("alloc"))
	format = format.Send(objc.RegisterName("initWithAttributes:"), unsafe.Pointer(&attrs[0]))
	if format == 0 {
		return nil, errors.New("video: no accelerated OpenGL 3.2 pixel format")
	}
	defer format.Send(objc.RegisterName("release"))

	view := objc.ID(objc.GetClass("NSOpenGLView")).Send(objc.RegisterName("alloc"))
	view = view.Send(
		objc.RegisterName("initWithFrame:pixelFormat:"),
		rect{Size: size{Width: initialFrameWidth, Height: initialFrameHeight}},
		format,
	)
	if view == 0 {
		return nil, errors.New("video: create NSOpenGLView")
	}
	// Retina: the drawable is measured in backing pixels, not DIPs.
	view.Send(objc.RegisterName("setWantsBestResolutionOpenGLSurface:"), true)

	context := view.Send(objc.RegisterName("openGLContext"))
	if context == 0 {
		view.Send(objc.RegisterName("release"))
		return nil, errors.New("video: create OpenGL context")
	}
	context.Send(objc.RegisterName("retain"))

	cgl := uintptr(context.Send(objc.RegisterName("CGLContextObj")))
	if cgl == 0 {
		context.Send(objc.RegisterName("release"))
		view.Send(objc.RegisterName("release"))
		return nil, errors.New("video: OpenGL context has no CGL context")
	}

	surface := &nativeSurface{
		view:    view,
		context: context,
		cgl:     cgl,
		gl:      gl,
		closed:  make(chan struct{}),
	}
	purego.RegisterLibFunc(&surface.proc, gl, "dlsym")
	purego.RegisterLibFunc(&surface.lockContext, gl, "CGLLockContext")
	purego.RegisterLibFunc(&surface.unlockContext, gl, "CGLUnlockContext")

	return surface, nil
}

// Attach reparents the retained view and rebinds the drawable.
func (s *nativeSurface) Attach(window uintptr, frame Rect) error {
	if err := s.usable(); err != nil {
		return err
	}
	if window == 0 {
		return errors.New("video: window handle is zero")
	}

	content := objc.ID(window).Send(objc.RegisterName("contentView"))
	if content == 0 {
		return ErrNoContentView
	}

	if err := s.lock(); err != nil {
		return err
	}
	s.view.Send(objc.RegisterName("removeFromSuperview"))
	insertBelowMyGoSurface(content, s.view)
	s.context.Send(objc.RegisterName("clearDrawable"))
	s.context.Send(objc.RegisterName("setView:"), s.view)
	s.applyFrameLocked(content, frame)
	unlockErr := s.unlock()
	if unlockErr != nil {
		return unlockErr
	}

	width, height := s.Size()
	slog.Default().Debug(
		"video: attached native surface",
		"view", fmt.Sprintf("%x", s.view),
		"context", fmt.Sprintf("%x", s.context),
		"window", fmt.Sprintf("%x", window),
		"pixels", fmt.Sprintf("%dx%d", width, height),
	)

	return nil
}

// SetFrame resizes the attached view and its drawable. While detached there is
// no view to resize, so it reports ErrNotAttached and leaves the state alone;
// the surface's next Attach carries the frame it is given.
func (s *nativeSurface) SetFrame(frame Rect) error {
	if err := s.usable(); err != nil {
		return err
	}

	content := s.view.Send(objc.RegisterName("superview"))
	if content == 0 {
		return ErrNotAttached
	}

	if err := s.lock(); err != nil {
		return err
	}
	s.applyFrameLocked(content, frame)

	return s.unlock()
}

// Detach removes the retained view from its window but keeps the surface, the
// OpenGL context, the render context and the render thread alive.
//
// It takes the same Go mutex and CGL lock as Attach, so it never races a frame
// in flight: the render thread finishes its current draw before the drawable is
// cleared. The view object stays retained for a later Attach, which rebinds it
// with setView: and recomputes the backing pixels, so no stream reload, second
// view, second context or second player is involved.
//
// Detach is idempotent and a safe no-op before the first Attach.
func (s *nativeSurface) Detach() error {
	if err := s.usable(); err != nil {
		return err
	}

	if err := s.lock(); err != nil {
		return err
	}
	s.view.Send(objc.RegisterName("removeFromSuperview"))
	s.context.Send(objc.RegisterName("clearDrawable"))
	s.state.detach()
	unlockErr := s.unlock()
	if unlockErr != nil {
		return unlockErr
	}

	slog.Default().Debug("video: detached native surface", "view", fmt.Sprintf("%x", s.view))

	return nil
}

// Size returns the drawable size in backing pixels, or 0, 0 while detached.
func (s *nativeSurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state.size()
}

// NativeViewHandle returns the retained NSOpenGLView for a UI host that places
// it as a native element, or 0 once the surface is closed.
func (s *nativeSurface) NativeViewHandle() uintptr {
	if s == nil || !isMainThread() {
		return 0
	}
	select {
	case <-s.closed:
		return 0
	default:
	}

	return uintptr(s.view)
}

// SyncDrawable refreshes the drawable from the view's own bounds. It is the
// path for hosts that lay the view out themselves: Attach applied a frame, but
// a hosted view is sized by the UI, and the view's window can also change when
// the UI moves it to another window.
func (s *nativeSurface) SyncDrawable() error {
	if err := s.usable(); err != nil {
		return err
	}

	if err := s.lock(); err != nil {
		return err
	}

	window := s.view.Send(objc.RegisterName("window"))
	if window == 0 {
		// Out of a window: no drawable, and the render thread must not guess.
		s.state.detach()
		unlockErr := s.unlock()

		return unlockErr
	}

	s.view.Send(objc.RegisterName("setWantsBestResolutionOpenGLSurface:"), true)
	s.context.Send(objc.RegisterName("update"))

	bounds := objc.Send[rect](s.view, objc.RegisterName("bounds"))
	scale := float32(objc.Send[float64](window, objc.RegisterName("backingScaleFactor")))
	s.state.resize(PixelSize(Rect{W: float32(bounds.Size.Width), H: float32(bounds.Size.Height)}, scale))
	s.signal()

	return s.unlock()
}

// Errors forwards renderer failures from the driver.
func (s *nativeSurface) Errors() <-chan error {
	if s.driver == nil {
		closed := make(chan error)
		close(closed)

		return closed
	}

	return s.driver.errs
}

// Close stops the render thread, then releases the native view and context.
func (s *nativeSurface) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		// The render context must be freed on its own thread first: destroying
		// a client with a live renderer can hang.
		if s.driver != nil {
			s.driver.close()
		}
		if !isMainThread() {
			// Releasing AppKit objects off the main thread can crash the app,
			// and this is a caller error rather than a playback failure.
			slog.Default().Error("video: Close called off the main thread; native surface kept")

			return
		}
		if err := s.releaseNative(); err != nil {
			slog.Default().Error("video: release native surface", "error", err)
		}
	})
}

// usable reports whether a main-thread call may proceed.
func (s *nativeSurface) usable() error {
	if !isMainThread() {
		return ErrNotMainThread
	}
	select {
	case <-s.closed:
		return ErrClosed
	default:
		return nil
	}
}

// nsWindowBelow is NSWindowBelow, the otherView of
// addSubview:positioned:relativeTo:.
const nsWindowBelow = ^uintptr(0)

// myGoSurfaceView is MyGo's layer-backed view. The video view must sit below
// it, so MyGo chrome, scrims and pointer input stay above live video.
const myGoSurfaceView = "MyGoSurfaceView"

// insertBelowMyGoSurface places view under MyGo's surface. A host without that
// view, as a test window or a future MyGo release that renames it, keeps the
// previous above-content behaviour instead of failing playback.
func insertBelowMyGoSurface(content, view objc.ID) {
	surface := findSubview(content, myGoSurfaceView)
	if surface == 0 {
		content.Send(objc.RegisterName("addSubview:"), view)
		slog.Default().Debug("video: MyGo surface view missing; view attached above chrome")

		return
	}

	content.Send(
		objc.RegisterName("addSubview:positioned:relativeTo:"),
		view, nsWindowBelow, surface,
	)
}

// findSubview returns the first direct subview with the given class name.
func findSubview(content objc.ID, class string) objc.ID {
	if content == 0 {
		return 0
	}

	subviews := content.Send(objc.RegisterName("subviews"))
	if subviews == 0 {
		return 0
	}

	count := int(objc.Send[uint64](subviews, objc.RegisterName("count")))
	for i := range count {
		view := objc.Send[objc.ID](subviews, objc.RegisterName("objectAtIndex:"), uint64(i))
		if view != 0 && className(view) == class {
			return view
		}
	}

	return 0
}

// className returns a view's Objective-C class name.
func className(view objc.ID) string {
	if view == 0 {
		return ""
	}

	name := view.Send(objc.RegisterName("className"))
	if name == 0 {
		return ""
	}
	bytes := objc.Send[unsafe.Pointer](name, objc.RegisterName("UTF8String"))
	if bytes == nil {
		return ""
	}

	return cString(bytes)
}

// cString copies a NUL-terminated C string.
func cString(p unsafe.Pointer) string {
	var out []byte
	for i := 0; ; i++ {
		ch := *(*byte)(unsafe.Add(p, i))
		if ch == 0 {
			return string(out)
		}
		out = append(out, ch)
	}
}

// applyFrameLocked applies a DIP frame, refreshes the drawable size and asks
// the render thread for a redraw. The caller holds the mutex and CGL lock.
func (s *nativeSurface) applyFrameLocked(content objc.ID, frame Rect) {
	bounds := objc.Send[rect](content, objc.RegisterName("bounds"))
	flipped := objc.Send[bool](content, objc.RegisterName("isFlipped"))
	frame = Rect{X: frame.X, Y: frame.Y, W: max(frame.W, 0), H: max(frame.H, 0)}

	s.view.Send(objc.RegisterName("setFrame:"), rect{
		Origin: point{X: float64(frame.X), Y: float64(originY(frame, float32(bounds.Size.Height), flipped))},
		Size:   size{Width: float64(frame.W), Height: float64(frame.H)},
	})
	s.context.Send(objc.RegisterName("update"))

	s.state.resize(PixelSize(frame, s.backingScaleLocked(content)))
	s.signal()
}

func (s *nativeSurface) backingScaleLocked(content objc.ID) float32 {
	window := content.Send(objc.RegisterName("window"))
	if window == 0 {
		return 1
	}

	return float32(objc.Send[float64](window, objc.RegisterName("backingScaleFactor")))
}

// procAddress resolves GL entry points for the surface's own context.
func (s *nativeSurface) procAddress(name string) unsafe.Pointer {
	// The OpenGL framework handle stays open for the process lifetime, as in
	// the M0 bridge.
	return s.proc(s.gl, name)
}

// lock takes the Go mutex and the native CGL lock.
func (s *nativeSurface) lock() error {
	s.mu.Lock()
	if code := s.lockContext(s.cgl); code != 0 {
		s.mu.Unlock()

		return fmt.Errorf("video: lock CGL context: error %d", code)
	}

	return nil
}

func (s *nativeSurface) unlock() error {
	code := s.unlockContext(s.cgl)
	s.mu.Unlock()
	if code != 0 {
		return fmt.Errorf("video: unlock CGL context: error %d", code)
	}

	return nil
}

func (s *nativeSurface) makeCurrentContext() {
	s.context.Send(objc.RegisterName("makeCurrentContext"))
}

func (s *nativeSurface) clearCurrentContext() {
	objc.ID(objc.GetClass("NSOpenGLContext")).Send(objc.RegisterName("clearCurrentContext"))
}

func (s *nativeSurface) flushBuffer() {
	s.context.Send(objc.RegisterName("flushBuffer"))
}

// renderFrameLocked reports whether this wakeup draws and, if so, the backing
// pixels to draw into. It is the single place that decides to skip a frame, so a
// detached surface never reaches GL or AppKit.
func (s *nativeSurface) renderFrameLocked(updated bool) (int, int, bool) {
	return s.state.renderFrame(updated)
}

func (s *nativeSurface) markCleanLocked() {
	s.state.clean()
}

func (s *nativeSurface) signal() {
	if s.driver != nil {
		s.driver.signal()
	}
}

// releaseNative detaches and releases the view and context once.
func (s *nativeSurface) releaseNative() error {
	if err := s.lock(); err != nil {
		return err
	}
	s.view.Send(objc.RegisterName("removeFromSuperview"))
	s.context.Send(objc.RegisterName("clearDrawable"))
	unlockErr := s.unlock()

	s.context.Send(objc.RegisterName("release"))
	s.view.Send(objc.RegisterName("release"))

	return unlockErr
}

// isMainThread reports whether the caller runs on AppKit's main thread.
func isMainThread() bool {
	return objc.Send[bool](objc.ID(objc.GetClass("NSThread")), objc.RegisterName("isMainThread"))
}
