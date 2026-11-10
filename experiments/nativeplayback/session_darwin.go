//go:build darwin && arm64 && mumu_spike

package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"
	mpv "github.com/gen2brain/go-mpv"
)

type intent struct {
	kind   string
	volume float64
}
type snapshot struct {
	state, detail string
	frames, loads int
}

func runSession(ctx context.Context, surface *videoSurface, source string, commands <-chan intent, publish func(snapshot)) {
	// The patched binding opens libmpv explicitly; see third_party/go-mpv.
	if err := mpv.Load(nil); err != nil {
		publish(snapshot{state: "error", detail: fmt.Sprintf("libmpv load: %v", err)})
		return
	}
	m := mpv.New()
	if m == nil {
		publish(snapshot{state: "error", detail: "mpv_create failed"})
		return
	}
	defer m.TerminateDestroy()
	options := map[string]string{
		"vo": "libmpv", "hwdec": "auto-safe", "config": "no", "load-scripts": "no",
		"ytdl": "no", "input-default-bindings": "no", "input-terminal": "no",
		"terminal": "no", "idle": "yes", "keep-open": "no", "volume": "30",
		"network-timeout": "10", "demuxer-max-bytes": "32MiB",
	}
	for name, value := range options {
		if err := m.SetOptionString(name, value); err != nil {
			publish(snapshot{state: "error", detail: fmt.Sprintf("option %s: %v", name, err)})
			return
		}
	}
	if err := m.Initialize(); err != nil {
		publish(snapshot{state: "error", detail: fmt.Sprint(err)})
		return
	}
	renderCtx, stopRender := context.WithCancel(ctx)
	ready := make(chan error, 1)
	renderDone := make(chan struct{})
	renderFailure := make(chan error, 1)
	var frames atomic.Int64
	go func() {
		defer close(renderDone)
		runRenderer(renderCtx, m, surface, &frames, ready, renderFailure)
	}()
	defer func() { stopRender(); <-renderDone }()
	if err := <-ready; err != nil {
		publish(snapshot{state: "error", detail: fmt.Sprintf("OpenGL render init: %v", err)})
		return
	}
	log.Printf("session-created id=%d api=%x mpv=%s ffmpeg=%s", m.ID(), m.APIVersion(), m.GetPropertyString("mpv-version"), m.GetPropertyString("ffmpeg-version"))
	state := snapshot{state: "idle", detail: "Ready: press Play"}
	publish(state)
	tick := time.NewTicker(time.Second / 60)
	defer tick.Stop()
	stats := time.NewTicker(time.Second)
	defer stats.Stop()
	for {
		select {
		case err := <-renderFailure:
			publish(snapshot{state: "error", detail: fmt.Sprintf("Video renderer failed: %v", err)})
			return
		case <-ctx.Done():
			log.Printf("stopping session=%d loads=%d frames=%d", m.ID(), state.loads, frames.Load())
			publish(snapshot{state: "stopping"})
			return
		case cmd := <-commands:
			switch cmd.kind {
			case "play":
				if state.state != "idle" && state.state != "error" && state.state != "stopped" {
					continue
				}
				if err := m.CommandAsync(1, []string{"loadfile", source}); err != nil {
					state.state, state.detail = "error", fmt.Sprint(err)
				} else {
					state.loads++
					state.state, state.detail = "loading", "Loading authorized test source"
				}
			case "volume":
				if err := m.SetProperty("volume", mpv.FormatDouble, cmd.volume); err != nil {
					log.Printf("volume error: %v", err)
				} else {
					log.Printf("volume=%s", m.GetPropertyString("volume"))
				}
			}
			publish(state)
		case <-tick.C:
			for range 64 {
				e := m.WaitEvent(0)
				if e.EventID == mpv.EventNone {
					break
				}
				switch e.EventID {
				case mpv.EventStart, mpv.EventFileLoaded, mpv.EventPlaybackRestart:
					log.Printf("event=%s session=%d loads=%d", e.EventID, m.ID(), state.loads)
					if e.EventID == mpv.EventPlaybackRestart {
						state.state, state.detail = "playing", "Playing"
						publish(state)
					}
				case mpv.EventEnd:
					end := e.EndFile()
					state.state, state.detail = "stopped", "Playback ended"
					if end.Error != nil {
						state.state, state.detail = "error", fmt.Sprintf("Playback failed: %v", end.Error)
					}
					log.Printf("event=end-file reason=%s error=%v", end.Reason, end.Error)
					publish(state)
				}
			}
		case <-stats.C:
			if state.state == "playing" || state.state == "buffering" {
				buffering := m.GetPropertyString("paused-for-cache") == "yes"
				state.state, state.detail = "playing", "Playing"
				if buffering {
					state.state, state.detail = "buffering", "Buffering"
				}
				log.Printf("stats session=%d loads=%d rendered=%d time=%s avsync=%s hwdec=%s vo=%s ao=%s audio=%s video=%sx%s drops=%s buffering=%t",
					m.ID(), state.loads, frames.Load(), m.GetPropertyString("time-pos"), m.GetPropertyString("avsync"),
					m.GetPropertyString("hwdec-current"), m.GetPropertyString("current-vo"), m.GetPropertyString("current-ao"),
					m.GetPropertyString("audio-codec-name"), m.GetPropertyString("width"), m.GetPropertyString("height"),
					m.GetPropertyString("decoder-frame-drop-count"), buffering)
				publish(state)
			}
		}
	}
}

// render.h forbids synchronous client calls or waits on the client from this thread.
func runRenderer(
	ctx context.Context,
	m *mpv.Mpv,
	surface *videoSurface,
	frames *atomic.Int64,
	ready chan<- error,
	failure chan<- error,
) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	if err := surface.lock(); err != nil {
		ready <- err
		return
	}
	surface.context.Send(objc.RegisterName("makeCurrentContext"))
	defer objc.ID(objc.GetClass("NSOpenGLContext")).Send(objc.RegisterName("clearCurrentContext"))
	render, err := m.NewRenderContextGL(surface.procAddress)
	surface.unlock()
	ready <- err
	if err != nil {
		return
	}
	defer func() {
		if err := surface.lock(); err != nil {
			// Destroying mpv with a live renderer can hang after native context loss.
			log.Fatalf("fatal render cleanup; CGL context unavailable: %v", err)
		}
		defer surface.unlock()
		surface.context.Send(objc.RegisterName("makeCurrentContext"))
		render.Free()
	}()
	render.SetUpdateCallback(surface.wake)
	defer render.SetUpdateCallback(nil)
	surface.wake()
	var lastFrame time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-surface.redraw:
			updated := render.Update()&mpv.RenderUpdateFrame != 0
			if err := surface.lock(); err != nil {
				failure <- err
				return
			}
			if (updated || surface.dirty) && surface.width > 0 && surface.height > 0 {
				surface.context.Send(objc.RegisterName("makeCurrentContext"))
				if err := render.RenderGL(0, surface.width, surface.height, true); err != nil {
					log.Printf("render error: %v", err)
					surface.unlock()
					failure <- err
					return
				} else {
					surface.context.Send(objc.RegisterName("flushBuffer"))
					render.ReportSwap()
					frames.Add(1)
					if !lastFrame.IsZero() && time.Since(lastFrame) > 100*time.Millisecond {
						log.Printf("render-gap-ms=%.3f", float64(time.Since(lastFrame).Microseconds())/1000)
					}
					lastFrame = time.Now()
				}
				surface.dirty = false
			}
			surface.unlock()
		}
	}
}
