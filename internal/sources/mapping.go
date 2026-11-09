package sources

import (
	"fmt"

	"mumu/internal/catalog"
	"mumu/internal/storage"
)

// ToStorageKind maps a domain source kind to its stored form.
func ToStorageKind(kind catalog.SourceKind) (storage.SourceKind, error) {
	switch kind {
	case catalog.SourceBuiltin:
		return storage.SourceBuiltin, nil
	case catalog.SourceRemotePlaylist:
		return storage.SourceRemotePlaylist, nil
	case catalog.SourceLocalPlaylist:
		return storage.SourceLocalPlaylist, nil
	case catalog.SourceCustomStream:
		return storage.SourceCustomStream, nil
	default:
		return "", fmt.Errorf("%w: unknown source kind %q", storage.ErrInvalid, kind)
	}
}

// FromStorageKind maps a stored source kind to the domain.
func FromStorageKind(kind storage.SourceKind) (catalog.SourceKind, error) {
	switch kind {
	case storage.SourceBuiltin:
		return catalog.SourceBuiltin, nil
	case storage.SourceRemotePlaylist:
		return catalog.SourceRemotePlaylist, nil
	case storage.SourceLocalPlaylist:
		return catalog.SourceLocalPlaylist, nil
	case storage.SourceCustomStream:
		return catalog.SourceCustomStream, nil
	default:
		return "", fmt.Errorf("%w: unknown stored source kind %q", catalog.ErrInvalid, kind)
	}
}

// ToStorageSource validates a domain source and returns its stored form.
func ToStorageSource(source catalog.Source) (storage.Source, error) {
	if err := source.Validate(); err != nil {
		return storage.Source{}, err
	}

	kind, err := ToStorageKind(source.Kind)
	if err != nil {
		return storage.Source{}, err
	}

	stored, err := storage.NewSource(kind, source.Location, source.DisplayName)
	if err != nil {
		return storage.Source{}, err
	}
	stored.Enabled = source.Enabled

	return stored, nil
}

// FromStorageSource maps a stored source to the domain.
func FromStorageSource(source storage.Source) (catalog.Source, error) {
	kind, err := FromStorageKind(source.Kind)
	if err != nil {
		return catalog.Source{}, err
	}

	return catalog.Source{
		ID:            source.ID,
		Kind:          kind,
		Identity:      source.Identity,
		Location:      source.Location,
		DisplayName:   source.DisplayName,
		Enabled:       source.Enabled,
		LastRefreshAt: source.LastRefreshAt,
		LastSuccessAt: source.LastSuccessAt,
		RefreshETag:   source.RefreshETag,
		RefreshError:  source.RefreshError,
		ItemCount:     source.ItemCount,
	}, nil
}

// ToStorageBatch maps channels and their streams for one atomic refresh.
func ToStorageBatch(channels []catalog.Channel) ([]storage.ChannelBatch, error) {
	batch := make([]storage.ChannelBatch, 0, len(channels))

	for i := range channels {
		channel := channels[i]
		if err := channel.Validate(); err != nil {
			return nil, err
		}

		streams := make([]storage.Stream, 0, len(channel.Streams))
		for position := range channel.Streams {
			stream := channel.Streams[position]
			headers, err := catalog.SanitizeHeaders(stream.Headers)
			if err != nil {
				return nil, err
			}
			providerID, err := catalog.ProviderID(stream.ProviderID, stream.URL)
			if err != nil {
				return nil, err
			}

			streams = append(streams, storage.Stream{
				ProviderID: providerID,
				URL:        stream.URL,
				Quality:    stream.Quality,
				HLS:        stream.HLS,
				Live:       stream.Live,
				Position:   position,
				Headers:    headers.Clone(),
			})
		}

		primary := channel.Streams[0]
		batch = append(batch, storage.ChannelBatch{
			Channel: storage.Channel{
				ProviderID: channel.ProviderID,
				Name:       channel.Name,
				StreamURL:  primary.URL,
				LogoURL:    channel.LogoURL,
				Country:    channel.Country,
				Category:   channel.Group,
				Language:   channel.Language,
				EPGID:      channel.EPGID,
				Live:       primary.Live,
			},
			Streams: streams,
		})
	}

	return batch, nil
}

// FromStorageChannel maps a stored channel and its streams to the domain.
func FromStorageChannel(channel storage.Channel, streams []storage.Stream) catalog.Channel {
	mapped := catalog.Channel{
		SourceID:   channel.SourceID,
		ProviderID: channel.ProviderID,
		Name:       channel.Name,
		Group:      channel.Category,
		Country:    channel.Country,
		Language:   channel.Language,
		LogoURL:    channel.LogoURL,
		EPGID:      channel.EPGID,
		Streams:    make([]catalog.Stream, 0, len(streams)),
	}

	for _, stream := range streams {
		var headers catalog.Headers
		if len(stream.Headers) > 0 {
			headers = catalog.Headers(stream.Headers).Clone()
		}

		mapped.Streams = append(mapped.Streams, catalog.Stream{
			ProviderID: stream.ProviderID,
			URL:        stream.URL,
			Quality:    stream.Quality,
			HLS:        stream.HLS,
			Live:       stream.Live,
			Headers:    headers,
		})
	}

	return mapped
}
