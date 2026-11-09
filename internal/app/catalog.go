package app

import (
	"errors"
	"log/slog"
	"time"

	"mumu/internal/catalog/iptvorg"
	"mumu/internal/netguard"
	"mumu/internal/playlist"
	"mumu/internal/sources"
)

const catalogRefreshTimeout = 30 * time.Second

// ErrCatalogNotReady reports a catalog read before the service was built.
var ErrCatalogNotReady = errors.New("catalog service is not ready")

// Catalog returns the source service, or ErrCatalogNotReady.
func (a *App) Catalog() (*sources.Service, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.catalog == nil {
		return nil, ErrCatalogNotReady
	}

	return a.catalog, nil
}

// openCatalog builds the source service over the local store. It performs no
// network access.
func (a *App) openCatalog() {
	store, err := a.Store()
	if err != nil || store == nil {
		slog.Debug("catalog service skipped without a local library")

		return
	}

	// Catalog-provided destinations stay public. User-chosen sources pass
	// AllowPrivate explicitly when they load.
	policy := netguard.Policy{}
	service, err := sources.New(
		store,
		playlist.NewLoader(policy),
		iptvorg.NewProvider(netguard.New(policy)),
		sources.WithLogger(slog.Default()),
	)
	if err != nil {
		slog.Error("build catalog service", "error", err)

		return
	}

	a.mu.Lock()
	a.catalog = service
	a.mu.Unlock()
}
