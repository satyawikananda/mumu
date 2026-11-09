//go:build darwin && arm64 && mumu_spike

package main

import (
	"fmt"
	"log"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type point struct{ X, Y float64 }
type size struct{ Width, Height float64 }
type rect struct {
	Origin point
	Size   size
}

// AppKit attachment runs on main; only the locked worker renders this context.
type videoSurface struct {
	mu            sync.Mutex
	view, context objc.ID
	gl            uintptr
	lookup        func(uintptr, string) unsafe.Pointer
	cgl           uintptr
	lockContext   func(uintptr) int32
	unlockContext func(uintptr) int32
	width, height int
	dirty         bool
	redraw        chan struct{}
}

func newVideoSurface() (*videoSurface, error) {
	gl, err := purego.Dlopen("/System/Library/Frameworks/OpenGL.framework/OpenGL", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("load OpenGL: %w", err)
	}
	// NSOpenGL.h: accelerated, double buffered, 24-bit color, OpenGL 3.2 Core.
	attrs := []uint32{73, 5, 8, 24, 99, 0x3200, 0}
	format := objc.ID(objc.GetClass("NSOpenGLPixelFormat")).Send(objc.RegisterName("alloc"))
	format = format.Send(objc.RegisterName("initWithAttributes:"), unsafe.Pointer(&attrs[0]))
	if format == 0 {
		return nil, fmt.Errorf("no accelerated OpenGL 3.2 pixel format")
	}
	defer format.Send(objc.RegisterName("release"))
	view := objc.ID(objc.GetClass("NSOpenGLView")).Send(objc.RegisterName("alloc"))
	view = view.Send(objc.RegisterName("initWithFrame:pixelFormat:"), rect{Size: size{Width: 640, Height: 360}}, format)
	if view == 0 {
		return nil, fmt.Errorf("create NSOpenGLView")
	}
	view.Send(objc.RegisterName("setWantsBestResolutionOpenGLSurface:"), true)
	context := view.Send(objc.RegisterName("openGLContext"))
	if context == 0 {
		view.Send(objc.RegisterName("release"))
		return nil, fmt.Errorf("create OpenGL context")
	}
	context.Send(objc.RegisterName("retain"))
	s := &videoSurface{view: view, context: context, gl: gl, dirty: true, redraw: make(chan struct{}, 1)}
	purego.RegisterLibFunc(&s.lookup, gl, "dlsym")
	purego.RegisterLibFunc(&s.lockContext, gl, "CGLLockContext")
	purego.RegisterLibFunc(&s.unlockContext, gl, "CGLUnlockContext")
	s.cgl = uintptr(context.Send(objc.RegisterName("CGLContextObj")))
	if s.cgl == 0 {
		context.Send(objc.RegisterName("release"))
		view.Send(objc.RegisterName("release"))
		return nil, fmt.Errorf("OpenGL context has no CGL context")
	}
	return s, nil
}

func (s *videoSurface) lock() error {
	s.mu.Lock()
	// AppKit also accesses the drawable; a Go mutex alone cannot synchronize it.
	if code := s.lockContext(s.cgl); code != 0 {
		s.mu.Unlock()
		return fmt.Errorf("lock CGL context: error %d", code)
	}
	return nil
}

func (s *videoSurface) unlock() {
	if code := s.unlockContext(s.cgl); code != 0 {
		log.Fatalf("fatal CGL unlock: error %d", code)
	}
	s.mu.Unlock()
}

func (s *videoSurface) attach(window uintptr, controlsHeight float64) error {
	parent := objc.ID(window).Send(objc.RegisterName("contentView"))
	if parent == 0 {
		return fmt.Errorf("window has no content view")
	}
	if err := s.lock(); err != nil {
		return err
	}
	defer s.unlock()
	s.view.Send(objc.RegisterName("removeFromSuperview"))
	parent.Send(objc.RegisterName("addSubview:"), s.view)
	s.context.Send(objc.RegisterName("clearDrawable"))
	s.context.Send(objc.RegisterName("setView:"), s.view)
	s.resizeLocked(parent, controlsHeight)
	log.Printf("native-attachment view=%x context=%x window=%x actual_window=%x pixels=%dx%d",
		uintptr(s.view), uintptr(s.context), window,
		uintptr(s.view.Send(objc.RegisterName("window"))), s.width, s.height)
	return nil
}

func (s *videoSurface) resize(window uintptr, controlsHeight float64) {
	if err := s.lock(); err != nil {
		log.Printf("resize surface: %v", err)
		return
	}
	defer s.unlock()
	s.resizeLocked(objc.ID(window).Send(objc.RegisterName("contentView")), controlsHeight)
}

func (s *videoSurface) resizeLocked(parent objc.ID, controlsHeight float64) {
	bounds := objc.Send[rect](parent, objc.RegisterName("bounds"))
	height := max(1, bounds.Size.Height-controlsHeight)
	y := controlsHeight
	if objc.Send[bool](parent, objc.RegisterName("isFlipped")) {
		y = 0
	}
	frame := rect{Origin: point{Y: y}, Size: size{Width: bounds.Size.Width, Height: height}}
	s.view.Send(objc.RegisterName("setFrame:"), frame)
	s.context.Send(objc.RegisterName("update"))
	scale := objc.Send[float64](parent.Send(objc.RegisterName("window")), objc.RegisterName("backingScaleFactor"))
	s.width, s.height = int(frame.Size.Width*scale), int(frame.Size.Height*scale)
	s.dirty = true
	s.wake()
}

func (s *videoSurface) wake() {
	select {
	case s.redraw <- struct{}{}:
	default:
	}
}

func (s *videoSurface) procAddress(name string) unsafe.Pointer { return s.lookup(s.gl, name) }

func (s *videoSurface) close() {
	s.view.Send(objc.RegisterName("removeFromSuperview"))
	s.context.Send(objc.RegisterName("clearDrawable"))
	s.context.Send(objc.RegisterName("release"))
	s.view.Send(objc.RegisterName("release"))
}
