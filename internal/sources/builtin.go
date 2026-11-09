package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/catalog/iptvorg"
	"mumu/internal/playlist"
	"mumu/internal/storage"
)

const builtinRefreshInterval = 6 * time.Hour

// Region returns the selected built-in region, defaulting to Indonesia.
func (s *Service) Region(ctx context.Context) (string, error) {
	value, ok, err := s.store.Settings().Get(ctx, SettingRegion)
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(value) == "" {
		return DefaultRegion, nil
	}

	region := strings.ToUpper(strings.TrimSpace(value))
	if _, err := s.provider.PlaylistURL(region); err == nil {
		return region, nil
	}

	return DefaultRegion, nil
}

// SetRegion stores a validated region code.
func (s *Service) SetRegion(ctx context.Context, region string) error {
	region = strings.ToUpper(strings.TrimSpace(region))
	if _, err := s.provider.PlaylistURL(region); err != nil {
		return err
	}

	return s.store.Settings().Set(ctx, SettingRegion, region)
}

// BuiltinEnabled reports whether the community catalog may be shown.
func (s *Service) BuiltinEnabled(ctx context.Context) (bool, error) {
	value, ok, err := s.store.Settings().Get(ctx, SettingBuiltinEnabled)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}

	if enabled, parseErr := strconv.ParseBool(value); parseErr == nil {
		return enabled, nil
	}

	return true, nil
}

// SetBuiltinEnabled shows or hides the community catalog.
func (s *Service) SetBuiltinEnabled(ctx context.Context, enabled bool) error {
	return s.store.Settings().Set(ctx, SettingBuiltinEnabled, strconv.FormatBool(enabled))
}

// Countries returns the region list and whether it came from a stale cache.
func (s *Service) Countries(ctx context.Context) ([]iptvorg.Country, bool, error) {
	cached, fetchedAt := s.cachedCountries(ctx)
	if len(cached) > 0 && s.now().Sub(fetchedAt) < countriesTTL {
		return cached, false, nil
	}

	countries, err := s.provider.Countries(ctx)
	if err != nil {
		if len(cached) > 0 {
			s.logger.Debug("catalog region list refresh failed; using cache")

			return cached, true, nil
		}

		return nil, false, err
	}

	if err := s.storeCountries(ctx, countries); err != nil {
		return nil, false, err
	}

	return countries, false, nil
}

// BuiltinSource returns the cached community source and its channels without
// touching the network.
func (s *Service) BuiltinSource(ctx context.Context, region string) (catalog.Source, []catalog.Channel, bool, error) {
	stored, id, err := s.builtinRecord(ctx, region)
	if err != nil {
		return catalog.Source{}, nil, false, err
	}
	if stored == nil {
		return catalog.Source{}, nil, false, nil
	}

	source, err := FromStorageSource(*stored)
	if err != nil {
		return catalog.Source{}, nil, false, err
	}

	channels, err := s.channelsForSource(ctx, id)
	if err != nil {
		return catalog.Source{}, nil, false, err
	}

	return source, channels, true, nil
}

// RefreshBuiltin fetches one country playlist, filters it, and replaces the
// cached rows atomically. On failure the previous channels stay available.
func (s *Service) RefreshBuiltin(ctx context.Context, region string) (catalog.Source, error) {
	url, err := s.provider.PlaylistURL(region)
	if err != nil {
		return catalog.Source{}, err
	}
	region = strings.ToUpper(strings.TrimSpace(region))

	existing, existingID, err := s.builtinRecord(ctx, region)
	if err != nil {
		return catalog.Source{}, err
	}

	etag := ""
	if existing != nil {
		etag = existing.RefreshETag
	}

	loaded, err := s.loader.Load(ctx, url, playlist.LoadOptions{
		DefaultCountry: region,
		ETag:           etag,
	})
	if err != nil {
		s.markFailed(ctx, existingID, err)

		return catalog.Source{}, err
	}

	if loaded.NotModified {
		if existing == nil {
			return catalog.Source{}, fmt.Errorf("%w: cached source disappeared", storage.ErrNotFound)
		}
		if err := s.store.Sources().MarkRefreshSucceeded(
			ctx,
			existing.ID,
			s.now(),
			existing.RefreshETag,
			existing.ItemCount,
		); err != nil {
			return catalog.Source{}, err
		}

		return FromStorageSource(*existing)
	}
	if loaded.Kind != playlist.KindChannelList {
		err := fmt.Errorf("%w: country playlist returned %s", playlist.ErrUnsupported, loaded.Kind)
		s.markFailed(ctx, existingID, err)

		return catalog.Source{}, err
	}

	channels, blocked := s.blocklist.WithEntries(s.upstreamBlocklist(ctx)).Filter(loaded.Channels)
	// A built-in source is always strict, whatever a playlist or a cached row
	// claims, so no community entry can reach a private host or an
	// unpublished scheme.
	channels, dropped := applyPolicy(catalog.SourceBuiltin, catalog.StrictPolicy(), channels)
	if len(channels) == 0 {
		err := fmt.Errorf("%w: playlist has no usable channels", playlist.ErrNoChannels)
		s.markFailed(ctx, existingID, err)

		return catalog.Source{}, err
	}

	batch, err := ToStorageBatch(channels)
	if err != nil {
		s.markFailed(ctx, existingID, err)

		return catalog.Source{}, err
	}

	displayName := builtinDisplayName(region)
	source, err := ToStorageSource(catalog.Source{
		Kind:        catalog.SourceBuiltin,
		Location:    url,
		DisplayName: displayName,
		Enabled:     true,
	})
	if err != nil {
		return catalog.Source{}, err
	}

	stored, err := s.store.Sources().Upsert(ctx, source)
	if err != nil {
		return catalog.Source{}, err
	}

	refreshedAt := s.now()
	if _, err := s.store.Channels().ReplaceForSource(ctx, stored.ID, batch, storage.RefreshMeta{
		RefreshedAt: refreshedAt,
		ETag:        loaded.ETag,
		ItemCount:   len(channels),
	}); err != nil {
		s.markFailed(ctx, stored.ID, err)

		return catalog.Source{}, err
	}

	s.logger.Info("community catalog refreshed",
		"region", region,
		"channels", len(channels),
		"blocked", blocked,
		"dropped", dropped,
	)
	saved, found, err := s.store.Sources().Get(ctx, stored.ID)
	if err != nil {
		return catalog.Source{}, err
	}
	if !found {
		return catalog.Source{}, fmt.Errorf("%w: source", storage.ErrNotFound)
	}

	return FromStorageSource(saved)
}

// BuiltinStale reports whether the cached community source should refresh.
func (s *Service) BuiltinStale(source catalog.Source) bool {
	if source.LastSuccessAt.IsZero() {
		return true
	}

	return s.now().Sub(source.LastSuccessAt) >= builtinRefreshInterval
}

func (s *Service) builtinRecord(ctx context.Context, region string) (*storage.Source, string, error) {
	url, err := s.provider.PlaylistURL(region)
	if err != nil {
		return nil, "", err
	}

	stored, err := storage.NewSource(storage.SourceBuiltin, url, builtinDisplayName(region))
	if err != nil {
		return nil, "", err
	}

	source, found, err := s.store.Sources().Get(ctx, stored.ID)
	if err != nil {
		return nil, stored.ID, err
	}
	if !found {
		return nil, stored.ID, nil
	}

	return &source, stored.ID, nil
}

func (s *Service) channelsForSource(ctx context.Context, sourceID string) ([]catalog.Channel, error) {
	channels, err := s.store.Channels().List(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	streams, err := s.store.Channels().ListStreamsBySource(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	mapped := make([]catalog.Channel, 0, len(channels))
	for _, channel := range channels {
		mapped = append(mapped, FromStorageChannel(channel, streams[channel.ID]))
	}

	return mapped, nil
}

func (s *Service) markFailed(ctx context.Context, sourceID string, cause error) {
	if sourceID == "" {
		return
	}
	if err := s.store.Sources().MarkRefreshFailed(
		ctx,
		sourceID,
		s.now(),
		safeMessage(cause),
	); err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.logger.Warn("record catalog refresh failure", "error", err)
	}
}

func (s *Service) cachedCountries(ctx context.Context) ([]iptvorg.Country, time.Time) {
	value, ok, err := s.store.Settings().Get(ctx, settingCountries)
	if err != nil || !ok {
		return nil, time.Time{}
	}

	var countries []iptvorg.Country
	if err := json.Unmarshal([]byte(value), &countries); err != nil {
		return nil, time.Time{}
	}

	raw, ok, err := s.store.Settings().Get(ctx, settingCountriesFetchedAt)
	if err != nil || !ok {
		return nil, time.Time{}
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, time.Time{}
	}

	return countries, time.UnixMilli(millis).UTC()
}

func (s *Service) storeCountries(ctx context.Context, countries []iptvorg.Country) error {
	data, err := json.Marshal(countries)
	if err != nil {
		return fmt.Errorf("encode country list: %w", err)
	}

	if err := s.store.Settings().Set(ctx, settingCountries, string(data)); err != nil {
		return err
	}

	return s.store.Settings().Set(
		ctx,
		settingCountriesFetchedAt,
		strconv.FormatInt(s.now().UTC().UnixMilli(), 10),
	)
}

// upstreamBlocklist returns the cached iptv-org exclusions, refreshing them
// when stale. A failure keeps the last cached list; it never fails the
// catalog refresh.
func (s *Service) upstreamBlocklist(ctx context.Context) []iptvorg.BlockEntry {
	cached, fetchedAt := s.cachedBlocklist(ctx)
	if !fetchedAt.IsZero() && s.now().Sub(fetchedAt) < blocklistTTL {
		return cached
	}

	entries, err := s.provider.BlockedChannels(ctx)
	if err != nil {
		if len(cached) > 0 {
			s.logger.Debug("catalog blocklist refresh failed; using cache")
		} else {
			s.logger.Debug("catalog blocklist unavailable")
		}

		return cached
	}

	if err := s.storeBlocklist(ctx, entries); err != nil {
		s.logger.Warn("cache catalog blocklist", "error", err)
	}

	return entries
}

func (s *Service) cachedBlocklist(ctx context.Context) ([]iptvorg.BlockEntry, time.Time) {
	value, ok, err := s.store.Settings().Get(ctx, settingBlocklist)
	if err != nil || !ok {
		return nil, time.Time{}
	}

	var entries []iptvorg.BlockEntry
	if err := json.Unmarshal([]byte(value), &entries); err != nil {
		return nil, time.Time{}
	}

	raw, ok, err := s.store.Settings().Get(ctx, settingBlocklistFetchedAt)
	if err != nil || !ok {
		return nil, time.Time{}
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, time.Time{}
	}

	return entries, time.UnixMilli(millis).UTC()
}

func (s *Service) storeBlocklist(ctx context.Context, entries []iptvorg.BlockEntry) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode blocklist: %w", err)
	}

	if err := s.store.Settings().Set(ctx, settingBlocklist, string(data)); err != nil {
		return err
	}

	return s.store.Settings().Set(
		ctx,
		settingBlocklistFetchedAt,
		strconv.FormatInt(s.now().UTC().UnixMilli(), 10),
	)
}

func safeMessage(err error) string {
	var playlistErr *playlist.Error
	if errors.As(err, &playlistErr) && playlistErr.Message != "" {
		return playlistErr.Message
	}

	return "catalog refresh failed"
}

func builtinDisplayName(region string) string {
	return "Community list (" + strings.ToUpper(strings.TrimSpace(region)) + ")"
}
