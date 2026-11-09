package sources

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"mumu/internal/catalog"
	"mumu/internal/playlist"
	"mumu/internal/storage"
)

// Sentinel errors for source management.
var (
	// ErrBuiltinManaged reports a built-in source managed outside the Library.
	ErrBuiltinManaged = errors.New("built-in catalog is managed by Discover")
	// ErrEmptyPreview reports a preview without a usable channel.
	ErrEmptyPreview = errors.New("preview has no channels")
)

// Preview is a parsed source that has not been stored.
type Preview struct {
	Kind           catalog.SourceKind
	Location       string
	DisplayName    string
	Channels       []catalog.Channel
	Warnings       []playlist.Warning
	Skipped        int
	SkippedReasons map[playlist.Code]int
	SkippedEntries []playlist.SkippedEntry
	Truncated      bool
	ETag           string
}

// RefreshResult reports what one source refresh changed.
type RefreshResult struct {
	Source   catalog.Source
	Channels int
	Streams  int
	Removed  int
}

// PreviewRemote parses a remote M3U playlist without storing it.
func (s *Service) PreviewRemote(ctx context.Context, rawURL, displayName string) (Preview, error) {
	loaded, err := s.loader.Load(ctx, rawURL, playlist.LoadOptions{
		AllowPrivate: true,
	})
	if err != nil {
		return Preview{}, err
	}
	if loaded.Kind != playlist.KindChannelList {
		return Preview{}, &playlist.Error{
			Code:    playlist.CodeStreamNotPlaylist,
			Err:     playlist.ErrPlaylistNotStream,
			Message: "URL is a single stream; add it as a stream instead",
		}
	}

	return s.previewFromLoad(catalog.SourceRemotePlaylist, rawURL, displayName, loaded), nil
}

// PreviewLocal parses a local M3U file without storing it.
func (s *Service) PreviewLocal(ctx context.Context, path, displayName string) (Preview, error) {
	loaded, err := s.loader.Load(ctx, path, playlist.LoadOptions{})
	if err != nil {
		return Preview{}, err
	}
	if loaded.Kind != playlist.KindChannelList {
		return Preview{}, &playlist.Error{
			Code:    playlist.CodeStreamNotPlaylist,
			Err:     playlist.ErrPlaylistNotStream,
			Message: "file is a single stream; local stream files are not supported yet",
		}
	}
	if displayName == "" {
		displayName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	return s.previewFromLoad(catalog.SourceLocalPlaylist, path, displayName, loaded), nil
}

// PreviewStream validates a direct stream URL and builds a one-channel source.
func (s *Service) PreviewStream(ctx context.Context, rawURL, displayName string) (Preview, error) {
	info, err := s.loader.Inspect(ctx, rawURL, true)
	if err != nil {
		return Preview{}, err
	}
	if info.Kind != playlist.KindHLSManifest && info.Kind != playlist.KindDirectStream {
		return Preview{}, &playlist.Error{
			Code:    playlist.CodeUnsupported,
			Err:     playlist.ErrUnsupported,
			Message: "URL is not a supported single stream",
		}
	}

	providerID, err := catalog.ProviderID("", rawURL)
	if err != nil {
		return Preview{}, err
	}
	if displayName == "" {
		displayName = streamDisplayName(rawURL)
	}

	return Preview{
		Kind:        catalog.SourceCustomStream,
		Location:    rawURL,
		DisplayName: displayName,
		Channels: []catalog.Channel{{
			ProviderID: providerID,
			Name:       displayName,
			Streams: []catalog.Stream{{
				URL:  rawURL,
				HLS:  info.Kind == playlist.KindHLSManifest,
				Live: false,
			}},
		}},
	}, nil
}

// Save stores a preview and replaces that source's channels atomically.
func (s *Service) Save(ctx context.Context, preview Preview) (catalog.Source, error) {
	if err := validatePreview(preview); err != nil {
		return catalog.Source{}, err
	}

	source, err := ToStorageSource(catalog.Source{
		Kind:        preview.Kind,
		Location:    preview.Location,
		DisplayName: preview.DisplayName,
		Enabled:     true,
	})
	if err != nil {
		return catalog.Source{}, err
	}

	stored, err := s.store.Sources().Upsert(ctx, source)
	if err != nil {
		return catalog.Source{}, err
	}

	batch, err := ToStorageBatch(preview.Channels)
	if err != nil {
		return catalog.Source{}, err
	}

	if _, err := s.store.Channels().ReplaceForSource(ctx, stored.ID, batch, storage.RefreshMeta{
		RefreshedAt: s.now(),
		ETag:        preview.ETag,
		ItemCount:   len(preview.Channels),
	}); err != nil {
		return catalog.Source{}, err
	}

	saved, found, err := s.store.Sources().Get(ctx, stored.ID)
	if err != nil {
		return catalog.Source{}, err
	}
	if !found {
		return catalog.Source{}, fmt.Errorf("%w: source", storage.ErrNotFound)
	}

	return FromStorageSource(saved)
}

// List returns every source, including the built-in catalog.
func (s *Service) List(ctx context.Context) ([]catalog.Source, error) {
	stored, err := s.store.Sources().List(ctx)
	if err != nil {
		return nil, err
	}

	sources := make([]catalog.Source, 0, len(stored))
	for _, source := range stored {
		mapped, err := FromStorageSource(source)
		if err != nil {
			return nil, err
		}
		sources = append(sources, mapped)
	}

	return sources, nil
}

// ListUserSources returns sources the Library may edit, excluding built-ins.
func (s *Service) ListUserSources(ctx context.Context) ([]catalog.Source, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	user := make([]catalog.Source, 0, len(sources))
	for _, source := range sources {
		if source.Kind == catalog.SourceBuiltin {
			continue
		}
		user = append(user, source)
	}

	return user, nil
}

// Channels returns the cached channels for one source.
func (s *Service) Channels(ctx context.Context, sourceID string) ([]catalog.Channel, error) {
	return s.channelsForSource(ctx, sourceID)
}

// UserChannels returns channels from every enabled, non-builtin source.
func (s *Service) UserChannels(ctx context.Context) ([]catalog.Channel, error) {
	stored, err := s.store.Sources().List(ctx)
	if err != nil {
		return nil, err
	}

	channels := []catalog.Channel{}
	for _, source := range stored {
		if source.Kind == storage.SourceBuiltin || !source.Enabled {
			continue
		}

		group, err := s.channelsForSource(ctx, source.ID)
		if err != nil {
			return nil, err
		}
		channels = append(channels, group...)
	}

	return channels, nil
}

// Refresh re-reads one user source and replaces its rows atomically.
func (s *Service) Refresh(ctx context.Context, sourceID string) (RefreshResult, error) {
	stored, found, err := s.store.Sources().Get(ctx, sourceID)
	if err != nil {
		return RefreshResult{}, err
	}
	if !found {
		return RefreshResult{}, fmt.Errorf("%w: source", storage.ErrNotFound)
	}
	if stored.Kind == storage.SourceBuiltin {
		return RefreshResult{}, ErrBuiltinManaged
	}

	source, err := FromStorageSource(stored)
	if err != nil {
		return RefreshResult{}, err
	}

	var (
		channels []catalog.Channel
		etag     string
	)
	switch source.Kind {
	case catalog.SourceRemotePlaylist:
		channels, etag, err = s.loadRemote(ctx, source)
	case catalog.SourceLocalPlaylist:
		channels, etag, err = s.loadLocal(ctx, source)
	case catalog.SourceCustomStream:
		channels, etag, err = s.inspectStream(ctx, source)
	default:
		err = fmt.Errorf("%w: source kind %q", storage.ErrInvalid, source.Kind)
	}
	if err != nil {
		s.markFailed(ctx, stored.ID, err)

		return RefreshResult{}, err
	}
	if etag == "" {
		etag = stored.RefreshETag
	}

	batch, err := ToStorageBatch(channels)
	if err != nil {
		s.markFailed(ctx, stored.ID, err)

		return RefreshResult{}, err
	}

	replaced, err := s.store.Channels().ReplaceForSource(ctx, stored.ID, batch, storage.RefreshMeta{
		RefreshedAt: s.now(),
		ETag:        etag,
		ItemCount:   len(channels),
	})
	if err != nil {
		s.markFailed(ctx, stored.ID, err)

		return RefreshResult{}, err
	}

	saved, found, err := s.store.Sources().Get(ctx, stored.ID)
	if err != nil {
		return RefreshResult{}, err
	}
	if !found {
		return RefreshResult{}, fmt.Errorf("%w: source", storage.ErrNotFound)
	}
	mapped, err := FromStorageSource(saved)
	if err != nil {
		return RefreshResult{}, err
	}

	return RefreshResult{
		Source:   mapped,
		Channels: replaced.Channels,
		Streams:  replaced.Streams,
		Removed:  replaced.Removed,
	}, nil
}

// SetEnabled turns a user source on or off.
func (s *Service) SetEnabled(ctx context.Context, sourceID string, enabled bool) error {
	stored, found, err := s.store.Sources().Get(ctx, sourceID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: source", storage.ErrNotFound)
	}
	if stored.Kind == storage.SourceBuiltin {
		return ErrBuiltinManaged
	}

	return s.store.Sources().SetEnabled(ctx, sourceID, enabled)
}

// Remove deletes a user source and its channels, favorites and recent rows.
func (s *Service) Remove(ctx context.Context, sourceID string) error {
	stored, found, err := s.store.Sources().Get(ctx, sourceID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: source", storage.ErrNotFound)
	}
	if stored.Kind == storage.SourceBuiltin {
		return ErrBuiltinManaged
	}

	return s.store.Sources().Delete(ctx, sourceID)
}

func (s *Service) loadRemote(
	ctx context.Context,
	source catalog.Source,
) ([]catalog.Channel, string, error) {
	loaded, err := s.loader.Load(ctx, source.Location, playlist.LoadOptions{
		AllowPrivate: true,
		ETag:         source.RefreshETag,
	})
	if err != nil {
		return nil, "", err
	}
	if loaded.NotModified {
		channels, err := s.channelsForSource(ctx, source.ID)

		return channels, source.RefreshETag, err
	}
	if loaded.Kind != playlist.KindChannelList {
		return nil, "", &playlist.Error{
			Code:    playlist.CodeStreamNotPlaylist,
			Err:     playlist.ErrPlaylistNotStream,
			Message: "URL is a single stream; add it as a stream instead",
		}
	}

	return loaded.Channels, loaded.ETag, nil
}

func (s *Service) loadLocal(
	ctx context.Context,
	source catalog.Source,
) ([]catalog.Channel, string, error) {
	loaded, err := s.loader.Load(ctx, source.Location, playlist.LoadOptions{})
	if err != nil {
		return nil, "", err
	}
	if loaded.Kind != playlist.KindChannelList {
		return nil, "", &playlist.Error{
			Code:    playlist.CodeStreamNotPlaylist,
			Err:     playlist.ErrPlaylistNotStream,
			Message: "file is a single stream; local stream files are not supported yet",
		}
	}

	return loaded.Channels, "", nil
}

func (s *Service) inspectStream(
	ctx context.Context,
	source catalog.Source,
) ([]catalog.Channel, string, error) {
	info, err := s.loader.Inspect(ctx, source.Location, true)
	if err != nil {
		return nil, "", err
	}
	if info.Kind != playlist.KindHLSManifest && info.Kind != playlist.KindDirectStream {
		return nil, "", &playlist.Error{
			Code:    playlist.CodeUnsupported,
			Err:     playlist.ErrUnsupported,
			Message: "URL is not a supported single stream",
		}
	}

	providerID, err := catalog.ProviderID("", source.Location)
	if err != nil {
		return nil, "", err
	}
	name := source.DisplayName
	if name == "" {
		name = streamDisplayName(source.Location)
	}

	return []catalog.Channel{{
		ProviderID: providerID,
		Name:       name,
		Streams: []catalog.Stream{{
			URL:  source.Location,
			HLS:  info.Kind == playlist.KindHLSManifest,
			Live: false,
		}},
	}}, "", nil
}

func (s *Service) previewFromLoad(
	kind catalog.SourceKind,
	location, displayName string,
	loaded playlist.LoadResult,
) Preview {
	if displayName == "" {
		displayName = loaded.Name
	}
	if displayName == "" {
		displayName = locationDisplayName(location)
	}

	return Preview{
		Kind:           kind,
		Location:       location,
		DisplayName:    displayName,
		Channels:       loaded.Channels,
		Warnings:       loaded.Warnings,
		Skipped:        loaded.Skipped,
		SkippedReasons: loaded.SkippedReasons,
		SkippedEntries: loaded.SkippedEntries,
		Truncated:      loaded.Truncated,
		ETag:           loaded.ETag,
	}
}

func validatePreview(preview Preview) error {
	if !preview.Kind.Valid() || preview.Kind == catalog.SourceBuiltin {
		return fmt.Errorf("%w: source kind %q", storage.ErrInvalid, preview.Kind)
	}
	if strings.TrimSpace(preview.Location) == "" {
		return fmt.Errorf("%w: preview has no location", storage.ErrInvalid)
	}
	if strings.TrimSpace(preview.DisplayName) == "" {
		return fmt.Errorf("%w: preview has no display name", storage.ErrInvalid)
	}
	if len(preview.Channels) == 0 {
		return ErrEmptyPreview
	}

	return nil
}

func locationDisplayName(location string) string {
	parsed, err := url.Parse(location)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}

	base := filepath.Base(location)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "Custom source"
	}

	return base
}

func streamDisplayName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}

	return "Custom stream"
}
