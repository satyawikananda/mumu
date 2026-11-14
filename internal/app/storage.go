package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"mumu/internal/storage"
	"mumu/internal/ui"
)

const (
	storeOpenTimeout = 3 * time.Second
	storeFileName    = "mumu.db"
)

// ErrStorageNotReady reports a read before the local library opened.
var ErrStorageNotReady = errors.New("local library is not open")

// Store returns the local library, or the error that prevented opening it.
func (a *App) Store() (*storage.Store, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil && a.storeErr == nil {
		return nil, ErrStorageNotReady
	}

	return a.store, a.storeErr
}

func (a *App) openLocalStore() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.setStoreError(fmt.Errorf("resolve local data directory: %w", err))

		return
	}

	a.openStoreAt(filepath.Join(dir, storeFileName))
}

func (a *App) openStoreAt(path string) {
	ctx, cancel := context.WithTimeout(a.ctx, storeOpenTimeout)
	defer cancel()

	store, err := storage.Open(ctx, path)

	a.mu.Lock()
	a.store = store
	a.storeErr = err
	a.mu.Unlock()

	if err != nil {
		slog.Error("open local library", "error", err)
	}
}

func (a *App) setStoreError(err error) {
	a.mu.Lock()
	a.storeErr = err
	a.mu.Unlock()

	slog.Error("open local library", "error", err)
}

func (a *App) closeStore() {
	a.mu.Lock()
	store := a.store
	a.store = nil
	a.mu.Unlock()

	if store == nil {
		return
	}
	if err := store.Close(); err != nil {
		slog.Error("close local library", "error", err)
	}
}

func storeMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, storage.ErrCorrupt):
		return "The local library file is damaged. " + ui.ProductName + " left it untouched. Move it aside to start a new library."
	case errors.Is(err, storage.ErrSchemaNewer):
		return "The local library was created by a newer version of " + ui.ProductName + ". Update " + ui.ProductName + " or move the library aside."
	case errors.Is(err, storage.ErrUnknownSchema):
		return ui.ProductName + " does not recognize the local library format. The file was left untouched."
	case errors.Is(err, storage.ErrPermission):
		return ui.ProductName + " cannot read or write its local library. Check permissions for the Mumu application support folder."
	default:
		return ui.ProductName + " could not open its local library. Existing data was left untouched."
	}
}
