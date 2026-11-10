//go:build darwin && arm64

package video

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/ebitengine/purego/objc"

	"mumu/internal/player"
)

// driver owns the single render thread for one native surface.
//
// The thread creates the render context, draws on wakeups and frees the
// context. It never makes synchronous engine calls: render.h forbids client
// calls or waits from the render thread, so control stays on the engine's own
// goroutine.
type driver struct {
	surface *nativeSurface

	// wake carries "draw something" signals from the update callback and from
	// frame/orientation changes. It is bounded and never blocks a producer.
	wake chan struct{}
	// errs carries the first render failure and closes when the thread stops.
	errs chan error

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// startDriver creates the render context on its own locked thread and returns
// once that context exists, so creation errors reach the caller.
func startDriver(surface *nativeSurface, renderer player.Renderer) (*driver, error) {
	d := &driver{
		surface: surface,
		wake:    make(chan struct{}, 1),
		errs:    make(chan error, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	ready := make(chan error, 1)
	go d.run(renderer, ready)

	if err := <-ready; err != nil {
		<-d.done

		return nil, err
	}

	return d, nil
}

// signal asks the render thread to draw. It is safe from any goroutine and
// from the engine's update callback.
func (d *driver) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// fail reports one render failure without blocking the render thread.
func (d *driver) fail(err error) {
	select {
	case d.errs <- err:
	default:
	}
}

// close stops the render thread and waits until it freed its native resources.
func (d *driver) close() {
	d.stopOnce.Do(func() { close(d.stop) })
	<-d.done
}

func (d *driver) run(renderer player.Renderer, ready chan<- error) {
	defer close(d.done)
	defer close(d.errs)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	defer d.surface.clearCurrentContext()

	render, err := d.newRenderContext(renderer)
	ready <- err
	if err != nil {
		return
	}

	defer d.freeRenderContext(render)
	render.SetUpdateCallback(d.signal)
	d.signal()

	for {
		select {
		case <-d.stop:
			return
		case <-d.wake:
			if err := d.draw(render); err != nil {
				slog.Default().Error("video: render failed", "error", err)
				d.fail(err)

				return
			}
		}
	}
}

// newRenderContext binds the engine's render context to the surface's GL
// context, which must be current on this thread.
func (d *driver) newRenderContext(renderer player.Renderer) (player.FrameRenderer, error) {
	if err := d.surface.lock(); err != nil {
		return nil, err
	}
	d.surface.makeCurrentContext()
	render, createErr := renderer.NewRenderContext(d.surface.procAddress)
	unlockErr := d.surface.unlock()
	if createErr != nil {
		return nil, fmt.Errorf("video: create render context: %w", createErr)
	}
	if unlockErr != nil {
		render.Free()

		return nil, unlockErr
	}

	return render, nil
}

// draw renders one frame when the engine has a new one or the drawable changed.
// While the surface is detached the drawable is size zero, so the frame is
// skipped: nothing touches the cleared drawable and nothing is logged.
func (d *driver) draw(render player.FrameRenderer) error {
	updated := render.Update()

	if err := d.surface.lock(); err != nil {
		return err
	}
	width, height, ok := d.surface.renderFrameLocked(updated)
	if !ok {
		return d.surface.unlock()
	}

	d.surface.makeCurrentContext()
	if err := render.Render(width, height); err != nil {
		return errors.Join(fmt.Errorf("video: draw frame: %w", err), d.surface.unlock())
	}
	d.surface.flushBuffer()
	render.ReportSwap()
	d.surface.markCleanLocked()

	return d.surface.unlock()
}

// freeRenderContext releases the context on the thread that owns it. A lost GL
// context is reported, not fatal: crashing the app would be worse than an
// unfreed context during shutdown.
func (d *driver) freeRenderContext(render player.FrameRenderer) {
	render.SetUpdateCallback(nil)

	if err := d.surface.lock(); err != nil {
		slog.Default().Error("video: render context cleanup skipped; GL context unavailable", "error", err)

		return
	}
	d.surface.makeCurrentContext()
	render.Free()

	if err := d.surface.unlock(); err != nil {
		slog.Default().Error("video: unlock after render context cleanup", "error", err)
	}
}
