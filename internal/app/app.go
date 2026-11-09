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

	"mumu/internal/sources"
	"mumu/internal/storage"
	"mumu/internal/ui"
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
	refreshBuiltin catalogRefresher
	discover       runSession
	discoverQuery  runSession
	discoverIndex  *discoverIndex
	logos          *logoCache
	library        runSession
	libraryData    *libraryCache
	settings       runSession
	settingsData   *settingsCache

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates an app with its cancellation context.
func New() *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		shell:          ui.NewShell(),
		ctx:            ctx,
		cancel:         cancel,
		refreshBuiltin: serviceRefreshBuiltin,
		logos:          newDiscoverLogos(),
		discoverIndex:  newDiscoverIndex(),
		libraryData:    newLibraryCache(),
		settingsData:   newSettingsCache(),
	}
}

// Run opens the main window and blocks until the app quits.
func (a *App) Run() error {
	// Stop runs on the main thread before exit.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.Stop() })
	mygo.App.WhenReady(func() {
		a.openLocalStore()
		a.openCatalog()
		a.openWindow()
		a.startDiscover()
		a.startLibrary()
		a.startSettings()
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

	a.closeStore()
}
