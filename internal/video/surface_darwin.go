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

	mu            sync.Mutex
	width, height int
	dirty         bool

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
	content.Send(objc.RegisterName("addSubview:"), s.view)
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

// SetFrame resizes the attached view and its drawable.
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

// Size returns the drawable size in backing pixels.
func (s *nativeSurface) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.width, s.height
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

	s.width, s.height = PixelSize(frame, s.backingScaleLocked(content))
	s.dirty = true
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

// drawableLocked returns the drawable size and whether a redraw is pending.
func (s *nativeSurface) drawableLocked() (int, int, bool) {
	return s.width, s.height, s.dirty
}

func (s *nativeSurface) markCleanLocked() {
	s.dirty = false
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
