// Package app is mumu's composition root: window, lifecycle and background jobs.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"mumu/internal/library"
	"mumu/internal/player"
	"mumu/internal/sources"
	"mumu/internal/storage"
	"mumu/internal/ui"
	"mumu/internal/video"
)

// shutdownTimeout bounds how long Stop waits for background work.
const shutdownTimeout = 2 * time.Second

// App owns the application lifecycle.
type App struct {
	shell *ui.Shell

	mu             sync.Mutex
	dispatcher     Dispatcher
	stopped        bool
	store          *storage.Store
	storeErr       error
	catalog        *sources.Service
	userLibrary    *library.Service
	refreshBuiltin catalogRefresher
	discover       runSession
	discoverQuery  runSession
	discoverIndex  *discoverIndex
	countryNames   map[string]string
	logos          *logoCache
	library        runSession
	libraryData    *libraryCache
	settings       runSession
	settingsData   *settingsCache

	// mainWin is the native window the current a.win adapts. Compare this
	// pointer, never the mainWindow interface: a closed window's adapter has a
	// different dynamic type than the *mygo.Window it came from.
	mainWin   *mygo.Window
	win       mainWindow
	pipWindow pipWindow
	// pipDispatcher is the floating window as a redraw target. It is set when
	// the PiP window is created and cleared when it is closed: the main
	// window's updates stop painting once that window is destroyed.
	pipDispatcher pipWindow
	// pipOffClose removes the floating window's close listener. A close the app
	// asks for must not be prevented by its own handler.
	pipOffClose  func()
	engine       player.Engine
	controller   *player.Controller
	surface      video.Surface
	presentation video.Presentation
	mainFrame    video.Rect
	media        mediaState

	// openMu serializes engine, controller and surface creation, which happen
	// on the first Play rather than at launch.
	openMu sync.Mutex
	// newEngine builds the playback engine. Tests replace it to run without a
	// native runtime; the default is the platform adapter. The volume is the
	// effective launch volume, so the engine never starts at its own zero.
	newEngine func(videoOutput string, volume int) (player.Engine, error)
	// newMainWindow creates the application window, recreating it after the
	// viewer closed it during PiP. Tests replace it with a fake.
	newMainWindow func() (mainWindow, *mygo.Window)
	// quit requests application shutdown. Tests replace it to count calls
	// without an event loop.
	quit func()

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates an app with its cancellation context.
func New() *App {
	ctx, cancel := context.WithCancel(context.Background())

	if err := ui.RegisterFonts(); err != nil {
		slog.Warn("bundled fonts unavailable; using the system font", "error", err)
	}

	a := &App{
		shell:          ui.NewShell(),
		presentation:   video.PresentationMain,
		newEngine:      newPlaybackEngine,
		quit:           mygo.App.Quit,
		ctx:            ctx,
		cancel:         cancel,
		refreshBuiltin: serviceRefreshBuiltin,
		logos:          newDiscoverLogos(),
		discoverIndex:  newDiscoverIndex(),
		libraryData:    newLibraryCache(),
		settingsData:   newSettingsCache(),
		media:          mediaState{snapshot: player.Snapshot{Volume: player.DefaultVolume}},
	}
	a.newMainWindow = a.createMainWindow

	return a
}

// Run opens the main window and blocks until the app quits.
func (a *App) Run() error {
	// Stop runs on the main thread before exit.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.Stop() })
	mygo.App.WhenReady(func() {
		a.openLocalStore()
		a.openCatalog()
		a.openUserLibrary()
		a.openWindow()
		a.startDiscover()
		a.startLibrary()
		a.startSettings()
		a.startWatch()
	})

	return mygo.App.Run()
}

// Go runs fn with the app context; errors and panics reach the UI. Call after Run.
func (a *App) Go(fn func(context.Context) error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return
	}

	a.wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				a.Fail(fmt.Errorf("background job panic: %v", r))
			}
		}()

		if err := fn(a.ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.Fail(err)
		}
	})
}

// GoQuiet runs fn with the app context without reporting its outcome as a
// global error. Use it when the caller publishes failures inline.
func (a *App) GoQuiet(fn func(context.Context)) {
	if fn == nil {
		return
	}

	a.Go(func(ctx context.Context) error {
		fn(ctx)

		return nil
	})
}

// Fail reports err to the user; safe from any goroutine.
func (a *App) Fail(err error) {
	if err == nil {
		return
	}

	slog.Error("application error", "error", err)

	a.publish(func() { a.shell.ShowError(err.Error()) })
}

// Stop cancels background work, waits up to shutdownTimeout, and is idempotent.
func (a *App) Stop() {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	a.mu.Unlock()

	// The quit sequence closes every window after this hook. Releasing the
	// floating player's close listener lets that close run normally instead of
	// being prevented by the handler that would ask to close PiP again.
	a.detachPiPClose()

	a.cancel()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		slog.Warn("shutdown timed out", "timeout", shutdownTimeout)
	}

	a.shutdownPlayback()
	a.closeStore()
}
