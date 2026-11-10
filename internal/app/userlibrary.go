package app

import (
	"errors"
	"log/slog"

	"mumu/internal/library"
)

// ErrUserLibraryNotReady reports a favorites read before the service was built.
var ErrUserLibraryNotReady = errors.New("favorites service is not ready")

// UserLibrary returns the favorites and recent-viewing service.
func (a *App) UserLibrary() (*library.Service, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.userLibrary == nil {
		return nil, ErrUserLibraryNotReady
	}

	return a.userLibrary, nil
}

// openUserLibrary builds the favorites service over the local store. It
// performs no network access and runs after the store opens.
func (a *App) openUserLibrary() {
	store, err := a.Store()
	if err != nil || store == nil {
		slog.Debug("favorites service skipped without a local library")

		return
	}

	service, err := library.New(store)
	if err != nil {
		slog.Error("build favorites service", "error", err)

		return
	}

	a.mu.Lock()
	a.userLibrary = service
	a.mu.Unlock()
}
