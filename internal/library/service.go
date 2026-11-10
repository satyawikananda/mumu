package library

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/sources"
	"mumu/internal/storage"
)

// Service reads the user's favorites, playback history and resume reference
// from the local store.
//
// It holds no mutable state of its own, and the store serializes its writes.
type Service struct {
	store *storage.Store
}

// New returns a service over the local store.
func New(store *storage.Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: nil store", storage.ErrInvalid)
	}

	return &Service{store: store}, nil
}

// Favorites returns the favorite entries, most recently added first.
//
// Entries whose source row no longer exists are skipped; the foreign key
// cascade normally removes them.
func (s *Service) Favorites(ctx context.Context) ([]Entry, error) {
	channels, err := s.store.Favorites().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list favorite channels: %w", err)
	}

	return s.build(ctx, channels, nil)
}

// Recent returns recently watched entries, most recently played first.
//
// limit is clamped to [1, storage.RecentLimit]; a limit of zero or less, or one
// above the stored bound, returns the whole bounded history.
func (s *Service) Recent(ctx context.Context, limit int) ([]Entry, error) {
	recent, err := s.store.Recent().List(ctx, normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list recent channels: %w", err)
	}

	channels := make([]storage.Channel, 0, len(recent))
	played := make(map[int64]storage.Recent, len(recent))
	for _, item := range recent {
		channels = append(channels, item.Channel)
		played[item.Channel.ID] = item
	}

	return s.build(ctx, channels, played)
}

// Last returns the resume reference: the most recently played entry, if any.
//
// Resume is a proposal, never an autoplay. The caller decides whether to offer
// it and must honour Entry.Playable before starting playback.
func (s *Service) Last(ctx context.Context) (Entry, bool, error) {
	entries, err := s.Recent(ctx, 1)
	if err != nil {
		return Entry{}, false, err
	}
	if len(entries) == 0 {
		return Entry{}, false, nil
	}

	return entries[0], true, nil
}

// ToggleFavorite adds or removes a favorite and returns the resulting state.
//
// A repeated call is idempotent: adding an existing favorite and removing a
// missing one both succeed, so a retry never fails. It is not a compare-and-swap
// though; two toggles racing on the same channel may both observe the same
// state, and the returned value is the state this call stored. A channel that is
// not in the local library reports storage.ErrNotFound.
func (s *Service) ToggleFavorite(
	ctx context.Context,
	sourceID, providerID string,
) (bool, error) {
	if strings.TrimSpace(sourceID) == "" || strings.TrimSpace(providerID) == "" {
		return false, fmt.Errorf("%w: channel identity is incomplete", storage.ErrInvalid)
	}

	channel, found, err := s.store.Channels().FindByProvider(ctx, sourceID, providerID)
	if err != nil {
		return false, fmt.Errorf("find favorite channel: %w", err)
	}
	if !found {
		return false, fmt.Errorf("%w: channel %q in source %q", storage.ErrNotFound, providerID, sourceID)
	}

	favorite, err := s.store.Favorites().Has(ctx, channel.ID)
	if err != nil {
		return false, fmt.Errorf("check favorite channel: %w", err)
	}

	if favorite {
		if err := s.store.Favorites().Remove(ctx, channel.ID); err != nil {
			return false, fmt.Errorf("remove favorite channel: %w", err)
		}

		return false, nil
	}

	if err := s.store.Favorites().Add(ctx, channel.ID, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("add favorite channel: %w", err)
	}

	return true, nil
}

// build maps stored channel rows to entries in the same order.
//
// Sources are loaded once per call, and the stream variants of every row in one
// chunked query, so the row count never multiplies the queries.
func (s *Service) build(
	ctx context.Context,
	channels []storage.Channel,
	played map[int64]storage.Recent,
) ([]Entry, error) {
	entries := make([]Entry, 0, len(channels))
	if len(channels) == 0 {
		return entries, nil
	}

	index, err := s.sourceIndex(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.ID)
	}
	streams, err := s.store.Channels().ListStreamsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list entry streams: %w", err)
	}

	for _, stored := range channels {
		source, ok := index[stored.SourceID]
		if !ok {
			// Defensive: deleting a source cascades to its channels.
			continue
		}

		mapped := sources.FromStorageChannel(stored, streams[stored.ID])
		entry := Entry{
			Key:      Key(stored.SourceID, stored.ProviderID),
			Channel:  mapped,
			Source:   source,
			Playable: playable(source, mapped.Streams),
		}
		if item, ok := played[stored.ID]; ok {
			entry.PlayedAt = item.PlayedAt
			entry.PlayCount = item.PlayCount
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// sourceIndex maps every stored source to its catalog form.
func (s *Service) sourceIndex(ctx context.Context) (map[string]catalog.Source, error) {
	stored, err := s.store.Sources().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list entry sources: %w", err)
	}

	index := make(map[string]catalog.Source, len(stored))
	for _, source := range stored {
		mapped, err := sources.FromStorageSource(source)
		if err != nil {
			return nil, fmt.Errorf("map stored source: %w", err)
		}
		index[source.ID] = mapped
	}

	return index, nil
}

// normalizeLimit applies the stored bound to a caller limit.
func normalizeLimit(limit int) int {
	if limit <= 0 || limit > storage.RecentLimit {
		return storage.RecentLimit
	}

	return limit
}
