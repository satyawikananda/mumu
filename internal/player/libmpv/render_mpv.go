//go:build darwin && arm64 && mumu_mpv

package libmpv

import (
	"fmt"
	"sync"
	"unsafe"

	mpv "github.com/gen2brain/go-mpv"

	"mumu/internal/player"
)

var (
	_ player.Renderer      = (*Engine)(nil)
	_ player.FrameRenderer = (*glRender)(nil)
)

// NewRenderContext attaches an OpenGL render context to this engine's client.
//
// The caller owns the render thread: the native GL context must be current and
// locked on the calling goroutine. This never creates a second client, and a
// second call for the same engine fails with player.ErrRenderActive.
func (e *Engine) NewRenderContext(getProcAddress func(name string) unsafe.Pointer) (player.FrameRenderer, error) {
	if getProcAddress == nil {
		return nil, fmt.Errorf("%w: nil GL procedure lookup", player.ErrInvalid)
	}
	if !renderingEnabled(e.cfg) {
		return nil, fmt.Errorf("%w: video output is %q", player.ErrRenderDisabled, e.cfg.VideoOutput)
	}
	e.renderMu.Lock()
	defer e.renderMu.Unlock()

	if e.ctx.Err() != nil {
		return nil, player.ErrClosed
	}
	if e.render != nil {
		return nil, player.ErrRenderActive
	}

	context, err := e.client.NewRenderContextGL(getProcAddress)
	if err != nil {
		return nil, fmt.Errorf("create libmpv OpenGL render context: %w", err)
	}

	render := &glRender{context: context}
	e.render = render

	return render, nil
}

// glRender adapts one libmpv render context to player.FrameRenderer. All
// methods run on the render thread except SetUpdateCallback's callback.
type glRender struct {
	context *mpv.RenderContext
	free    sync.Once
}

// SetUpdateCallback registers fn, which mpv calls from its own thread.
func (r *glRender) SetUpdateCallback(fn func()) {
	r.context.SetUpdateCallback(fn)
}

// Update reports whether a new frame is ready to draw.
func (r *glRender) Update() bool {
	return r.context.Update()&mpv.RenderUpdateFrame != 0
}

// Render draws the current frame into the bound framebuffer at the given
// backing-pixel size. Flipping matches AppKit's bottom-left GL origin.
func (r *glRender) Render(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: render size %dx%d", player.ErrInvalid, width, height)
	}
	if err := r.context.RenderGL(0, width, height, true); err != nil {
		return fmt.Errorf("render frame: %w", err)
	}

	return nil
}

// ReportSwap tells mpv the frame reached the screen.
func (r *glRender) ReportSwap() {
	r.context.ReportSwap()
}

// Free destroys the render context once. It must run before the engine closes.
func (r *glRender) Free() {
	r.free.Do(r.context.Free)
}
