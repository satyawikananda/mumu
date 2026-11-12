package sources

import (
	"fmt"
	"log/slog"
	"time"

	"mumu/internal/catalog/iptvorg"
	"mumu/internal/playlist"
	"mumu/internal/storage"
)

// Settings keys owned by the catalog service.
const (
	SettingRegion         = "catalog.region"
	SettingBuiltinEnabled = "catalog.builtin.enabled"
)

const (
	settingCountries          = "catalog.countries"
	settingCountriesFetchedAt = "catalog.countries_fetched_at"
	settingBlocklist          = "catalog.blocklist"
	settingBlocklistFetchedAt = "catalog.blocklist_fetched_at"
	countriesTTL              = 24 * time.Hour
	blocklistTTL              = 24 * time.Hour
)

// Service coordinates sources, the built-in catalog and local storage.
//
// It owns no UI and no widgets. Network work is cancellable and every stored
// update is atomic through storage.ReplaceForSource.
type Service struct {
	store     *storage.Store
	loader    *playlist.Loader
	provider  *iptvorg.Provider
	blocklist iptvorg.Blocklist
	logger    *slog.Logger
	now       func() time.Time
}

// Option customizes a Service.
type Option func(*Service)

// WithLogger sets the diagnostic logger.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithClock supplies the time source used for cache freshness and metadata.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithBlocklist replaces the reviewed local blocklist.
func WithBlocklist(blocklist iptvorg.Blocklist) Option {
	return func(s *Service) {
		s.blocklist = blocklist
	}
}

// New returns a service over the local store and bounded loader.
func New(
	store *storage.Store,
	loader *playlist.Loader,
	provider *iptvorg.Provider,
	opts ...Option,
) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: nil store", storage.ErrInvalid)
	}
	if loader == nil {
		return nil, fmt.Errorf("%w: nil playlist loader", storage.ErrInvalid)
	}
	if provider == nil {
		return nil, fmt.Errorf("%w: nil catalog provider", storage.ErrInvalid)
	}

	blocklist, err := iptvorg.DefaultBlocklist()
	if err != nil {
		return nil, fmt.Errorf("load catalog blocklist: %w", err)
	}

	service := &Service{
		store:     store,
		loader:    loader,
		provider:  provider,
		blocklist: blocklist,
		logger:    slog.Default(),
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(service)
	}

	return service, nil
}
