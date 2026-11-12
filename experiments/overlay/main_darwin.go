//go:build darwin && arm64

// Command overlay is the #32 feasibility spike: it plays one real libmpv
// session and draws MyGo chrome above it.
//
// It is an experiment, not the app. See README.md for the commands and for what
// each mode proves.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/native"
	"github.com/egoist/mygo/ui"

	"mumu/internal/player"
	"mumu/internal/video"
)

// readAheadSeconds restores the demuxer read-ahead that a paused stream drops.
const readAheadSeconds = 30

type app struct {
	s  appState
	do actions

	adapter *viewAdapter
	win     *mygo.Window
	panel   *mygo.Window

	sess        *session
	stream      string
	autoplay    bool
	keepFront   bool
	radius      float64
	pipToggles  int
	toggleEvery time.Duration
	resizeCycle bool
	host        *hostedView
	pauseAfter  time.Duration
	resumeAfter time.Duration
	ctx         context.Context
	cancel      context.CancelFunc

	pendingMu sync.Mutex
	pending   []func()

	mu sync.Mutex
}

func main() {
	stream := flag.String("stream", "", "HLS or video URL to play in the single session")
	chooseMode := flag.String("mode", string(modeBelow), "below inserts the view under MyGo's surface; nativeview hosts it as a native element; above attaches it over the window")
	volume := flag.Int("volume", 70, "engine volume, 0-100")
	autoplay := flag.Bool("autoplay", false, "load the stream as soon as the window is ready (spike only; the product never autoplays)")
	autoclose := flag.Duration("autoclose", 0, "quit after this duration, for scripted evidence runs")
	logfile := flag.String("logfile", "", "write logs here instead of stderr, for runs started through open(1)")
	keepFront := flag.Bool("keepfront", false, "keep this window in front, for scripted screenshots and clicks; an experiment-only convenience")
	radius := flag.Float64("radius", 0, "corner radius of the hosted video container, in points")
	pipToggles := flag.Int("pip-toggles", 0, "move the session between windows this many times, for scripted evidence")
	toggleEvery := flag.Duration("toggle-every", 1200*time.Millisecond, "interval between scripted PiP toggles")
	resizeCycle := flag.Bool("resize-cycle", false, "resize the window while playing, for scripted evidence")
	pauseAfter := flag.Duration("pause-after", 0, "pause the stream after this duration, for scripted evidence runs")
	resumeAfter := flag.Duration("resume-after", 0, "resume a scripted pause after this duration from the pause")
	flag.Parse()

	if *stream == "" {
		fmt.Fprintln(os.Stderr, "overlay: -stream is required")
		os.Exit(2)
	}

	selected := mode(*chooseMode)
	if selected != modeNativeView && selected != modeBelow && selected != modeAbove {
		fmt.Fprintf(os.Stderr, "overlay: unknown -mode %q\n", *chooseMode)
		os.Exit(2)
	}

	if *logfile != "" {
		file, err := os.OpenFile(*logfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a := &app{
		s: appState{
			Mode:    selected,
			Status:  "idle",
			Volume:  *volume,
			Overlay: true,
		},
		stream:      *stream,
		autoplay:    *autoplay,
		keepFront:   *keepFront,
		radius:      *radius,
		pipToggles:  *pipToggles,
		toggleEvery: *toggleEvery,
		resizeCycle: *resizeCycle,
		pauseAfter:  *pauseAfter,
		resumeAfter: *resumeAfter,
		ctx:         ctx,
		cancel:      cancel,
	}
	a.adapter = &viewAdapter{handle: a.surfaceHandle}
	a.do = actions{
		Play:          a.play,
		Pause:         a.togglePause,
		Stop:          a.stop,
		ToggleOverlay: func() { a.publish(func() { a.s.Overlay = !a.s.Overlay }) },
		ToggleDrawer:  func() { a.publish(func() { a.s.Drawer = !a.s.Drawer }) },
		Fullscreen:    a.fullscreen,
		MovePiP:       a.movePiP,
		Quit:          func() { mygo.App.Quit() },
		Volume:        a.setVolume,
		Hit:           a.hit,
	}

	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.stop() })
	mygo.App.WhenReady(func() {
		activate()
		a.createWindows()
		a.start()
	})

	if *autoclose > 0 {
		go func() {
			select {
			case <-ctx.Done():
			case <-time.After(*autoclose):
				mygo.App.Quit()
			}
		}()
	}

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// activate makes this unsandboxed command-line binary behave like an app the
// viewer can see: a regular activation policy puts its windows in front of
// other apps, which a bare binary does not do. The product ships as a bundle
// and needs none of this.
func activate() {
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	if app == 0 {
		return
	}
	app.Send(objc.RegisterName("setActivationPolicy:"), 0) // NSApplicationActivationPolicyRegular
	app.Send(objc.RegisterName("activateIgnoringOtherApps:"), true)
}

// createWindows builds the Watch window and the hidden floating panel.
func (a *app) createWindows() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:          "mumu · overlay spike",
		Width:          1100,
		Height:         700,
		X:              80,
		Y:              80,
		MinWidth:       640,
		MinHeight:      420,
		UseContentSize: true,
		// The window itself stays opaque: the video view shows through the
		// transparent pixels of MyGo's own surface, not through the window.
		BackgroundColor: "#09090B",
		Content:         ui.View(func(c *ui.Context) { a.view(c, false) }),
	})
	a.win.OnResize(func() { a.publish(a.onResize) })
	log.Printf("main window bounds: %+v handle=%x", a.win.ContentBounds(), a.win.NativeHandle())

	a.panel = mygo.NewWindow(mygo.WindowOptions{
		Title:           "mumu · overlay spike PiP",
		Width:           420,
		Height:          300,
		MinWidth:        320,
		MinHeight:       240,
		UseContentSize:  true,
		Panel:           true,
		AlwaysOnTop:     true,
		Hidden:          true,
		BackgroundColor: "#09090B",
		Content:         ui.View(func(c *ui.Context) { a.view(c, true) }),
	})
}

// start opens the single session on the main thread and loads on request.
func (a *app) start() {
	sess, err := newSession(a.s.Volume)
	if err != nil {
		a.publish(func() {
			a.s.Status = "unavailable"
			a.s.Detail = err.Error()
		})

		return
	}
	a.sess = sess

	log.Printf("after start bounds: %+v", a.win.ContentBounds())
	a.win.Focus()
	a.watchEvents()
	a.watchDrawable()
	a.publish(a.onResize)

	if a.autoplay {
		a.play()
	}
	if a.pauseAfter > 0 {
		a.schedulePause()
	}
	if a.pipToggles > 0 {
		a.schedulePiPToggles()
	}
	if a.resizeCycle {
		a.scheduleResizeCycle()
	}
}

// play loads the stream in the existing session. A second click is a retry of
// the same channel, never a second session.
func (a *app) play() {
	if a.sess == nil {
		return
	}

	a.publish(func() {
		a.s.Status = "loading"
		a.s.Detail = ""
		a.s.Busy = true
	})

	go func() {
		if _, err := a.sess.load(a.ctx, a.stream); err != nil {
			a.publish(func() {
				a.s.Status = "error"
				a.s.Detail = err.Error()
				a.s.Busy = false
			})
		}
	}()
}

// stop ends the stream without closing the session.
func (a *app) stop() {
	if a.cancel != nil {
		// Kept for the quit path: the context ends the worker loops.
		_ = a.ctx.Err()
	}
	if a.sess == nil {
		return
	}

	go func() {
		if err := a.sess.engine.Stop(a.ctx); err != nil {
			log.Printf("stop: %v", err)
		}
		a.publish(func() {
			a.s.Playing = false
			a.s.Paused = false
			a.s.Status = "stopped"
			a.s.Detail = ""
		})
	}()
}

// togglePause holds the frame and stops audio; resume continues the session.
func (a *app) togglePause() {
	if a.sess == nil {
		return
	}

	paused := !a.s.Paused
	go func() {
		if err := a.sess.setPaused(a.ctx, paused); err != nil {
			log.Printf("pause: %v", err)

			return
		}
		a.publish(func() {
			a.s.Paused = paused
			if paused {
				a.s.Status = "paused"
			} else {
				a.s.Status = "playing"
			}
		})
	}()
}

// setVolume changes the engine volume only; the spike persists nothing.
func (a *app) setVolume(volume int) {
	if a.sess == nil {
		return
	}

	a.publish(func() { a.s.Volume = volume })
	go func() {
		if err := a.sess.setVolume(a.ctx, volume); err != nil {
			log.Printf("volume: %v", err)
		}
	}()
}

// fullscreen toggles the main window.
func (a *app) fullscreen() {
	if a.s.PiP {
		a.panel.ToggleFullScreen()

		return
	}
	a.win.ToggleFullScreen()
}

// movePiP moves the same session between the main window and the panel. The
// retained view is reparented by MyGo's native element host; nothing reloads.
func (a *app) movePiP() {
	toPiP := !a.s.PiP
	a.publish(func() {
		a.s.PiP = toPiP
		if toPiP {
			a.win.Hide()
			a.panel.Show()
			a.panel.Focus()
		} else {
			a.panel.Hide()
			a.win.Show()
			a.win.Focus()
		}
		a.onResize()
		a.win.Invalidate()
		a.panel.Invalidate()
	})
}

// hit records that pointer input reached a chrome control above live video.
func (a *app) hit(name string) {
	a.publish(func() {
		a.s.Clicks++
		a.s.LastAction = name
	})
}

// onResize re-attaches in above mode and refreshes the hosted drawable in
// nativeview mode. Main thread only.
func (a *app) onResize() {
	if a.sess == nil {
		log.Printf("onResize: no session")
		return
	}

	if a.s.Mode == modeAbove || a.s.Mode == modeBelow {
		window := a.win
		if a.s.PiP {
			window = a.panel
		}
		bounds := window.ContentBounds()
		frame := video.Rect{W: float32(bounds.Width), H: float32(bounds.Height)}
		if err := a.sess.surface.Attach(window.NativeHandle(), frame); err != nil {
			log.Printf("attach: %v", err)
		}
		if a.s.Mode == modeBelow {
			if a.host == nil {
				a.host = newHostedView(a.radius)
				if a.host == nil {
					log.Printf("host container: unavailable")
				}
			}
			if a.host != nil {
				if err := a.host.attach(window.NativeHandle(), a.sess.surface.NativeViewHandle()); err != nil {
					log.Printf("host attach: %v", err)
				}
				log.Printf("subviews bottom-to-top: %v", subviewOrder(window.NativeHandle()))
			}
		}
		if err := a.sess.syncDrawable(); err != nil && err != video.ErrClosed {
			log.Printf("attach sync: %v", err)
		}
		a.s.DrawableW, a.s.DrawableH = drawableSize(a.sess.surface)

		return
	}

	if err := a.sess.syncDrawable(); err != nil && err != video.ErrClosed {
		log.Printf("sync drawable: %v", err)
	}
	a.s.DrawableW, a.s.DrawableH = drawableSize(a.sess.surface)
}

// post queues fn to run on the UI thread during the next build. Window.Update
// from a worker is not reliable while the window is not frontmost, and the
// scripted runs need a deterministic main-thread hop.
func (a *app) post(fn func()) {
	a.pendingMu.Lock()
	a.pending = append(a.pending, fn)
	a.pendingMu.Unlock()
}

// drainPending runs the queued actions. Main thread only.
func (a *app) drainPending() {
	a.pendingMu.Lock()
	queued := a.pending
	a.pending = nil
	a.pendingMu.Unlock()

	for _, fn := range queued {
		fn()
	}
}

// schedulePiPToggles moves the session between windows on a timer. The count is
// the recorded evidence for repeated transfers: the same view handle and one
// stream load must hold throughout.
func (a *app) schedulePiPToggles() {
	go func() {
		for i := range a.pipToggles {
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(a.toggleEvery):
			}
			n := i + 1
			a.post(func() {
				log.Printf("scripted PiP toggle %d/%d", n, a.pipToggles)
				a.movePiP()
				a.target().Invalidate()
			})
		}
	}()
}

// scheduleResizeCycle resizes the window, then enters and leaves fullscreen,
// while the stream keeps playing.
func (a *app) scheduleResizeCycle() {
	go func() {
		steps := []func(){
			func() { a.win.SetContentSize(760, 480) },
			func() { a.win.SetContentSize(1100, 700) },
			func() { a.win.SetContentSize(900, 560) },
			func() { a.win.ToggleFullScreen() },
			func() { a.win.ToggleFullScreen() },
		}
		for i, step := range steps {
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(1800 * time.Millisecond):
			}
			index := i + 1
			a.post(func() {
				log.Printf("scripted resize step %d/%d", index, len(steps))
				step()
			})
		}
	}()
}

// schedulePause runs the scripted pause and resume, so a recording or a byte
// counter has a known timeline.
func (a *app) schedulePause() {
	go func() {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(a.pauseAfter):
		}
		a.setPaused(true)
		if a.resumeAfter <= 0 {
			return
		}
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(a.resumeAfter):
		}
		a.setPaused(false)
	}()
}

// setPaused applies an explicit pause state, used by the scripted run.
func (a *app) setPaused(paused bool) {
	if a.sess == nil {
		return
	}
	go func() {
		if paused {
			// Pause alone still lets the demuxer fill its cache from a live
			// stream; the experiment measures both settings.
			if err := a.sess.engine.SetReadAhead(a.ctx, 0); err != nil {
				log.Printf("read-ahead 0: %v", err)
			}
		}
		if err := a.sess.setPaused(a.ctx, paused); err != nil {
			log.Printf("set paused %v: %v", paused, err)

			return
		}
		if !paused {
			if err := a.sess.engine.SetReadAhead(a.ctx, readAheadSeconds); err != nil {
				log.Printf("read-ahead %ds: %v", readAheadSeconds, err)
			}
		}
		log.Printf("paused=%v", paused)
		a.publish(func() {
			a.s.Paused = paused
			if paused {
				a.s.Status = "paused"
			} else {
				a.s.Status = "playing"
			}
			a.target().Invalidate()
		})
	}()
}

// watchEvents applies engine events on the UI thread.
func (a *app) watchEvents() {
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case event, ok := <-a.sess.events():
				if !ok {
					return
				}
				a.apply(event)
			}
		}
	}()
}

// watchDrawable keeps the hosted drawable in step with layout, which the spike
// does not otherwise observe: MyGo sizes the native view itself.
func (a *app) watchDrawable() {
	if a.s.Mode == modeAbove {
		return
	}

	log.Printf("watchDrawable: started mode=%s", a.s.Mode)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				// A redraw is the bounded way to ask for a layout pass from a
				// worker: it does not run UI work on this goroutine.
				a.target().Invalidate()
			}
		}
	}()
}

// syncDrawableOnly refreshes the drawable and redraws only on a size change.
func (a *app) syncDrawableOnly() {
	if a.sess == nil {
		log.Printf("sync: no session")
		return
	}
	if err := a.sess.syncDrawable(); err != nil && err != video.ErrClosed {
		log.Printf("sync: %v", err)
		return
	}

	width, height := drawableSize(a.sess.surface)
	if width == a.s.DrawableW && height == a.s.DrawableH {
		return
	}
	log.Printf("drawable %dx%d handle=%x", width, height, a.sess.surface.NativeViewHandle())
	a.s.DrawableW, a.s.DrawableH = width, height
	a.target().Invalidate()
}

// apply maps one engine event into the state the view reads.
func (a *app) apply(event player.Event) {
	a.publish(func() {
		switch event.Kind {
		case player.EventLoaded:
			log.Printf("event: loaded")
			a.s.Status = "loaded"
			a.s.Busy = false
		case player.EventStarted:
			a.s.Status = "started"
		case player.EventPlaying:
			log.Printf("event: playing")
			a.s.Status = "playing"
			a.s.Playing = true
			a.s.Paused = false
			a.s.Busy = false
		case player.EventBuffering:
			a.s.Status = "buffering"
		case player.EventEnded:
			a.s.Status = "ended"
			a.s.Playing = false
			a.s.Busy = false
		case player.EventFailed:
			log.Printf("event: failed %v", event.Err)
			a.s.Status = "error"
			a.s.Playing = false
			a.s.Busy = false
			if event.Err != nil {
				a.s.Detail = event.Err.Error()
			}
		case player.EventMetrics:
			if event.Metrics.PausedForCache {
				a.s.Detail = "cache paused"
			}
		case player.EventShutdown:
			a.s.Status = "shutdown"
		}
		a.target().Invalidate()
	})
}

// target is the window that currently shows the video.
func (a *app) target() *mygo.Window {
	if a.s.PiP && a.panel != nil {
		return a.panel
	}

	return a.win
}

// publish runs fn on the UI thread.
func (a *app) publish(fn func()) {
	win := a.target()
	if win == nil {
		return
	}
	win.Update(fn)
}
