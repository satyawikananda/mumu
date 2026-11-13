package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"mumu/internal/player"
	"mumu/internal/ui"
	"mumu/internal/video"
)

// pipWindow is the native floating window the presenter needs.
type pipWindow interface {
	Show()
	Hide()
	Close()
	Focus()
	NativeHandle() uintptr
	Bounds() mygo.Rectangle
	ContentBounds() mygo.Rectangle
	SetContentBounds(bounds mygo.Rectangle)
	SetBounds(bounds mygo.Rectangle)
	SetMinimumSize(width, height int)
	// Invalidate asks the floating window for a new frame without running a
	// state callback. The app's own publish path already ran that callback on
	// the main window, whose redraw stops working once it is destroyed.
	Invalidate()
	OnResize(fn func())
	OnMove(fn func())
	// OnClose registers a close handler and returns the function that removes
	// it. A close the app asks for must be able to run without its own handler
	// preventing it, so the registration is removable.
	OnClose(fn func(e *mygo.CloseEvent)) (off func())
}

// nativePip adapts a MyGo window to the presenter's interface.
type nativePip struct{ win *mygo.Window }

func (p nativePip) Show()                 { p.win.Show() }
func (p nativePip) Hide()                 { p.win.Hide() }
func (p nativePip) Close()                { p.win.Close() }
func (p nativePip) Focus()                { p.win.Focus() }
func (p nativePip) Invalidate()           { p.win.Invalidate() }
func (p nativePip) NativeHandle() uintptr { return p.win.NativeHandle() }
func (p nativePip) Bounds() mygo.Rectangle {
	return p.win.Bounds()
}
func (p nativePip) ContentBounds() mygo.Rectangle          { return p.win.ContentBounds() }
func (p nativePip) SetContentBounds(bounds mygo.Rectangle) { p.win.SetContentBounds(bounds) }
func (p nativePip) SetBounds(bounds mygo.Rectangle)        { p.win.SetBounds(bounds) }
func (p nativePip) SetMinimumSize(width, height int)       { p.win.SetMinimumSize(width, height) }
func (p nativePip) OnResize(fn func())                     { p.win.OnResize(fn) }
func (p nativePip) OnMove(fn func())                       { p.win.OnMove(fn) }
func (p nativePip) OnClose(fn func(e *mygo.CloseEvent)) func() {
	return p.win.OnClose(fn)
}

const (
	// pipInitialWidth is the floating player's starting content width.
	pipInitialWidth = 640
	// pipInitialHeight is its 16:9 content height.
	pipInitialHeight = 360
	// pipMinWidth keeps the video usable; below 360 the chrome collapses.
	pipMinWidth  = 280
	pipMinHeight = 158
)

// pipState builds the mini player state from the shared session.
func (a *App) pipState() ui.PiPState {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot := a.media.snapshot

	return ui.PiPState{
		Channel: a.media.channel.Name,
		Status:  snapshot.Detail,
		Error:   snapshot.Error,
		Muted:   snapshot.Muted,
		Paused:  snapshot.State == player.StatePaused,
		Live:    a.media.stream.Live || snapshot.Capabilities.Live,
		Quality: ui.WatchQuality{
			Selected: snapshot.Quality.Selected,
			Current:  snapshot.Quality.Current,
			Known:    snapshot.Quality.Known,
			Fallback: snapshot.Quality.Fallback,
		},
		Recovery: ui.WatchRecovery{
			Attempt:     snapshot.Recovery.Attempt,
			MaxAttempts: snapshot.Recovery.MaxAttempts,
			Pending:     snapshot.Recovery.Pending,
			Terminal:    snapshot.Recovery.Terminal,
			RetryIn:     snapshot.Recovery.RetryIn,
		},
	}
}

// enterPiP moves the one surface into a floating window.
func (a *App) enterPiP() {
	a.revealWatch()

	a.mu.Lock()
	surface, active := a.surface, a.media.channel.ProviderID != ""
	pip := a.pipWindow
	a.mu.Unlock()

	if surface == nil || !active {
		return
	}

	// Snapshot the main audio intent before the floating panel can override it.
	a.mu.Lock()
	a.pipMainMuted = a.media.snapshot.Muted
	a.pipMuted = a.media.snapshot.Muted
	a.mu.Unlock()

	if pip == nil {
		pip = a.newPiPWindow()
		if pip == nil {
			a.failWatch("Picture-in-Picture could not open.")

			return
		}

		a.mu.Lock()
		a.pipWindow = pip
		// The floating window paints independently once the main window is
		// closed, so it is also the second redraw target for state updates.
		a.pipDispatcher = pip
		a.mu.Unlock()

		a.applySavedPiPGeometry(pip)
	}

	a.mu.Lock()
	state := a.presentation
	a.mu.Unlock()

	attached := false
	err := video.Transfer(&state, video.PresentationPiP, func() error {
		pip.Show()
		pip.Focus()

		if err := surface.Attach(pip.NativeHandle(), pipFrame(pip)); err != nil {
			return err
		}
		attached = true

		return nil
	})

	a.mu.Lock()
	a.presentation = state
	if attached {
		// The retained view is in the floating window now; the main window has
		// no drawable until a return attaches it again.
		a.media.attached = false
		a.media.pipAttached = true
	}
	a.mu.Unlock()

	if err != nil {
		slog.Warn("enter picture-in-picture", "error", err)
		a.failWatch("Picture-in-Picture could not start.")

		return
	}

	a.publishWatch()
}

// returnFromPiP moves the surface back to the main window, recreating it when
// the viewer closed it earlier.
//
// The page reports its video box only on a layout pass, so the app shows Watch,
// forgets the stale box and then binds the retained view to the new window. A
// failed attach leaves the presentation in PiP, the floating player untouched
// and the session playing: there is no half state to clean up.
func (a *App) returnFromPiP() {
	a.mu.Lock()
	surface, pip, frame, haveWindow := a.surface, a.pipWindow, a.mainFrame, a.win != nil
	a.mu.Unlock()

	if surface == nil {
		return
	}

	if !haveWindow {
		a.openWindow()
	}

	a.mu.Lock()
	main := a.win
	a.mu.Unlock()

	if main == nil {
		// Stay in PiP rather than dropping the picture or leaving a
		// transition behind; the viewer can try Return again.
		slog.Warn("return from picture-in-picture: the main window is unavailable")

		return
	}

	// Show Watch and forget the box before the transfer, so the recreated page
	// lays out and reports the video area again. Both calls are UI-thread work,
	// as is the transfer that follows.
	a.shell.ShowWatch()
	a.shell.Watch().ResetFrame()

	a.mu.Lock()
	state := a.presentation
	a.mu.Unlock()

	attached := false
	err := video.Transfer(&state, video.PresentationMain, func() error {
		main.Restore()
		main.Show()

		target := frame
		if target.W <= 0 || target.H <= 0 {
			target = contentFrame(main)
		}

		if err := surface.Attach(main.NativeHandle(), target); err != nil {
			return err
		}
		attached = true

		return nil
	})

	a.mu.Lock()
	a.presentation = state
	if attached {
		a.media.attached = true
		a.media.pipAttached = false
	}
	a.mu.Unlock()

	if err != nil {
		// The floating player keeps the session and the view: Attach fails
		// before it reparents, so there is no drawable to repair. The viewer
		// can press Return again.
		slog.Warn("return from picture-in-picture", "error", err)

		return
	}

	if pip != nil {
		pip.Hide()
	}

	a.restoreMainMute()
	a.publishWatch()

	if !a.shell.IsWatch() {
		a.detachVideoSurface()
	}

	main.Focus()
}

// closePiP ends the mini player and its viewing session. Close/Esc always
// stops playback, even when the main window still exists; the next Play builds
// a fresh controller and surface.
//
// It is idempotent: a released PiP window means there is nothing left to close,
// so a second request never stops playback or asks to quit twice.
func (a *App) closePiP() {
	a.mu.Lock()
	pip, haveWindow := a.pipWindow, a.win != nil
	quit := a.quit
	controller := a.controller
	a.mu.Unlock()

	if pip == nil {
		return
	}

	if controller != nil {
		_ = controller.Stop()
	}

	a.mu.Lock()
	a.media.snapshot.State = player.StateStopped
	a.media.snapshot.Detail = "Stopped"
	a.media.snapshot.Error = ""
	a.media.snapshot.Recovery = player.Recovery{}
	a.media.playbackErr = ""
	a.pipMuted = false
	a.mu.Unlock()

	// Stop playback while the floating drawable is still alive: releasing the
	// native view from a destroyed window is a hang risk the teardown does not
	// need to take.
	a.shutdownPlayback()
	a.releasePiPWindow()

	a.shell.ShowWatch()
	a.publishWatch()

	if !haveWindow && quit != nil {
		quit()
	}
}

// releasePiPWindow closes the floating window and forgets it as a redraw
// target, so no later publish invalidates a destroyed window.
//
// The close listener goes first: a close the app asks for must not be prevented
// by the handler that turns a user close into closePiP, which would call this
// path again and leave the window open.
func (a *App) releasePiPWindow() {
	a.detachPiPClose()

	a.mu.Lock()
	pip := a.pipWindow
	offDisplays := a.pipOffDisplays
	offActivity := a.pipActivityOff
	a.pipWindow = nil
	a.pipDispatcher = nil
	a.pipOffDisplays = nil
	a.pipActivityOff = nil
	a.stopPiPSaveTimer()
	if a.win != nil {
		a.presentation = video.PresentationMain
	} else {
		a.presentation = video.PresentationStopped
	}
	a.media.attached = false
	a.media.pipAttached = false
	a.mu.Unlock()

	if offDisplays != nil {
		offDisplays()
	}
	if offActivity != nil {
		offActivity()
	}
	if pip != nil {
		pip.Close()
	}
}

// detachPiPClose removes the floating window's close listener once. Stop calls
// it too, so the quit sequence can close the window normally instead of having
// its own request prevented.
func (a *App) detachPiPClose() {
	a.mu.Lock()
	off := a.pipOffClose
	a.pipOffClose = nil
	a.mu.Unlock()

	if off != nil {
		off()
	}
}

// pipCloseRequest runs when the floating window asks to close, either from the
// native title bar or from the panel's own action.
//
// AppKit calls the listener from windowShouldClose:, so tearing the window down
// on the spot would destroy it reentrantly inside its own close request. The
// request is prevented and the teardown is posted instead, running on the next
// main-thread turn once the close handler has returned.
func (a *App) pipCloseRequest(e *mygo.CloseEvent) {
	e.PreventDefault()
	a.publish(a.closePiP)
}

// newPiPWindow creates the native floating player once. It is frameless, so the
// video fills the whole window and the view draws its own chrome.
func (a *App) newPiPWindow() pipWindow {
	pip := mygo.NewWindow(mygo.WindowOptions{
		Title:           "mumu · Picture-in-Picture",
		Width:           pipInitialWidth,
		Height:          pipInitialHeight,
		UseContentSize:  true,
		MinWidth:        pipMinWidth,
		MinHeight:       pipMinHeight,
		Frameless:       true,
		CornerRadius:    14,
		Panel:           true,
		AlwaysOnTop:     true,
		BackgroundColor: "#09090B",
		Content:         myui.View(a.shell.PiPView),
	})

	adapter := nativePip{win: pip}
	adapter.OnResize(func() {
		a.resizePiP(adapter)
		a.savePiPGeometrySoon(adapter)
	})
	adapter.OnMove(func() { a.savePiPGeometrySoon(adapter) })

	off := adapter.OnClose(a.pipCloseRequest)
	activityOff, activityErr := startActivityMonitor(adapter.NativeHandle(), a.notePiPActivity)
	if activityErr != nil {
		slog.Debug("picture-in-picture activity monitor unavailable", "error", activityErr)
	}

	a.mu.Lock()
	a.pipOffClose = off
	a.pipActivityOff = activityOff
	a.pipOffDisplays = mygo.Screen.OnDisplaysChanged(func() { a.displaysChangedPiP(adapter) })
	a.mu.Unlock()

	return adapter
}

// resizePiP keeps the frameless window at 16:9 and moves the surface with it.
func (a *App) resizePiP(pip pipWindow) {
	bounds := pip.ContentBounds()
	if bounds.Width <= 0 {
		return
	}

	want := int(math.Round(float64(bounds.Width) * 9 / 16))
	if bounds.Height != want {
		bounds.Height = want
		pip.SetContentBounds(bounds)
	}

	a.mu.Lock()
	surface := a.surface
	state := a.presentation
	a.mu.Unlock()

	if surface == nil || state != video.PresentationPiP {
		return
	}
	if err := surface.SetFrame(pipFrame(pip)); err != nil {
		slog.Debug("resize picture-in-picture", "error", err)
	}
	if err := surface.SyncDrawable(); err != nil && !errors.Is(err, video.ErrClosed) {
		slog.Debug("sync picture-in-picture drawable", "error", err)
	}
}

// pipFrame returns the video area. The frameless window is already 16:9, so
// this is the whole content area with a small letterbox only if a drag slipped
// through before the resize handler corrected it.
func pipFrame(pip pipWindow) video.Rect {
	return video.Fit(video.Rect{
		W: float32(pip.ContentBounds().Width),
		H: float32(pip.ContentBounds().Height),
	}, videoAspect)
}

// contentFrame is the fallback main-window frame before the page reports one.
func contentFrame(win mainWindow) video.Rect {
	bounds := win.ContentBounds()

	return video.Fit(video.Rect{
		W: float32(bounds.Width),
		H: float32(bounds.Height),
	}, videoAspect)
}

// togglePiPMute flips the floating window's temporary override. The main audio
// intent stays untouched, so Return can restore it.
func (a *App) togglePiPMute() {
	a.mu.Lock()
	a.pipMuted = !a.pipMuted
	muted := a.pipMuted
	a.media.snapshot.Muted = muted
	controller := a.controller
	a.mu.Unlock()

	if controller != nil {
		_ = controller.SetMuted(muted)
	}
	a.publishWatch()
}

// restoreMainMute hands the captured main audio intent back to the shared
// controller and clears the temporary override.
func (a *App) restoreMainMute() {
	a.mu.Lock()
	main := a.pipMainMuted
	a.pipMuted = false
	a.media.snapshot.Muted = main
	controller := a.controller
	a.mu.Unlock()

	if controller != nil {
		_ = controller.SetMuted(main)
	}
}

// pipGeometrySetting stores the floating window's last bounds per display.
const pipGeometrySetting = "pip.geometry"

type pipGeometry struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// pipGeometryStore keeps one frame per display.
type pipGeometryStore struct {
	Displays map[int64]pipGeometry `json:"displays"`
}

// loadPiPGeometries reads the remembered frames once, before a window opens.
func (a *App) loadPiPGeometries(ctx context.Context) {
	store, err := a.Store()
	if err != nil || store == nil {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
	defer cancel()

	settings, err := store.Settings().All(readCtx)
	if err != nil {
		return
	}
	raw := settings[pipGeometrySetting]
	if raw == "" {
		return
	}

	var saved pipGeometryStore
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return
	}

	a.mu.Lock()
	for display, geometry := range saved.Displays {
		a.pipGeometries[display] = geometry
	}
	a.mu.Unlock()
}

// applySavedPiPGeometry restores the remembered size and corner for the display
// under the pointer. A missing or unusable value leaves the default.
func (a *App) applySavedPiPGeometry(pip pipWindow) {
	display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())

	a.mu.Lock()
	geometry, ok := a.pipGeometries[display.ID]
	a.mu.Unlock()
	if !ok {
		return
	}

	a.publish(func() {
		a.mu.Lock()
		current := a.pipWindow == pip
		a.mu.Unlock()
		if !current {
			return
		}
		a.clampAndSetPiPBounds(pip, geometry)
	})
}

// clampAndSetPiPBounds keeps a saved frame inside a connected display, even
// after the display it was saved on is gone.
func (a *App) clampAndSetPiPBounds(pip pipWindow, geometry pipGeometry) {
	bounds := mygo.Rectangle{
		X:      geometry.X,
		Y:      geometry.Y,
		Width:  geometry.Width,
		Height: geometry.Height,
	}
	bounds = clampPiPBounds(bounds)
	pip.SetBounds(bounds)
	a.resizePiP(pip)
}

func clampPiPBounds(bounds mygo.Rectangle) mygo.Rectangle {
	display := mygo.Screen.DisplayMatching(bounds)
	area := display.WorkArea
	if area.Width <= 0 || area.Height <= 0 {
		area = display.Bounds
	}
	if area.Width <= 0 || area.Height <= 0 {
		display = mygo.Screen.DisplayNearestPoint(mygo.Point{X: bounds.X + bounds.Width/2, Y: bounds.Y + bounds.Height/2})
		area = display.WorkArea
		if area.Width <= 0 || area.Height <= 0 {
			area = display.Bounds
		}
	}
	if area.Width <= 0 || area.Height <= 0 {
		return bounds
	}

	return clampPiPToArea(bounds, area)
}

// clampPiPToArea keeps a floating frame inside one display work area and at
// 16:9. It is pure so the geometry policy is testable without an AppKit app.
func clampPiPToArea(bounds, area mygo.Rectangle) mygo.Rectangle {
	width := min(max(bounds.Width, pipMinWidth), area.Width)
	height := int(math.Round(float64(width) * 9 / 16))
	height = min(max(height, pipMinHeight), area.Height)

	x := min(max(bounds.X, area.X), area.X+area.Width-width)
	y := min(max(bounds.Y, area.Y), area.Y+area.Height-height)

	return mygo.Rectangle{X: x, Y: y, Width: width, Height: height}
}

// savePiPGeometrySoon debounces move/resize notifications, which arrive many
// times during a drag.
func (a *App) savePiPGeometrySoon(pip pipWindow) {
	a.mu.Lock()
	a.stopPiPSaveTimer()
	a.pipSaveTimer = time.AfterFunc(400*time.Millisecond, func() {
		a.savePiPGeometry(pip)
	})
	a.mu.Unlock()
}

// savePiPGeometry writes the current bounds with the display they belong to.
func (a *App) savePiPGeometry(pip pipWindow) {
	bounds := pip.Bounds()
	display := mygo.Screen.DisplayMatching(bounds)
	geometry := pipGeometry{
		X:      bounds.X,
		Y:      bounds.Y,
		Width:  bounds.Width,
		Height: bounds.Height,
	}

	a.mu.Lock()
	if a.pipGeometries == nil {
		a.pipGeometries = map[int64]pipGeometry{}
	}
	a.pipGeometries[display.ID] = geometry
	payload, err := json.Marshal(pipGeometryStore{Displays: a.pipGeometries})
	a.mu.Unlock()
	if err != nil {
		return
	}

	a.GoQuiet(func(ctx context.Context) {
		store, err := a.Store()
		if err != nil || store == nil {
			return
		}

		writeCtx, cancel := context.WithTimeout(ctx, playbackTimeout)
		defer cancel()

		if err := store.Settings().Set(writeCtx, pipGeometrySetting, string(payload)); err != nil {
			slog.Debug("save picture-in-picture geometry", "error", err)
		}
	})
}

// displaysChangedPiP pulls the floating window back onto a connected display.
func (a *App) displaysChangedPiP(pip pipWindow) {
	a.publish(func() {
		a.mu.Lock()
		current := a.pipWindow == pip
		a.mu.Unlock()
		if !current {
			return
		}

		bounds := pip.Bounds()
		bounds = clampPiPBounds(bounds)
		pip.SetBounds(bounds)
		a.resizePiP(pip)
	})
}

func (a *App) stopPiPSaveTimer() {
	if a.pipSaveTimer != nil {
		a.pipSaveTimer.Stop()
		a.pipSaveTimer = nil
	}
}
