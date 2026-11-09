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

	"mumu/internal/ui"
)

// shutdownTimeout bounds how long Stop waits for background work.
const shutdownTimeout = 2 * time.Second

// App owns the application lifecycle.
type App struct {
	shell *ui.Shell

	mu         sync.Mutex
	dispatcher Dispatcher
	stopped    bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates an app with its cancellation context.
func New() *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		shell:  ui.NewShell(),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Run opens the main window and blocks until the app quits.
func (a *App) Run() error {
	// Stop runs on the main thread before exit.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.Stop() })
	mygo.App.WhenReady(a.openWindow)

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
}
