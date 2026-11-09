package storage

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func batchChannel(providerID, name, streamURL string) ChannelBatch {
	return ChannelBatch{
		Channel: Channel{
			ProviderID: providerID,
			Name:       name,
			StreamURL:  streamURL,
			Live:       true,
			EPGID:      providerID,
		},
		Streams: []Stream{{
			ProviderID: providerID + "-sd",
			URL:        streamURL,
			Quality:    "SD",
			Live:       true,
		}},
	}
}

func TestReplaceForSourceInsertsStreams(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	first := batchChannel("one", "One", "https://stream.example/one.m3u8")
	first.Streams = append(first.Streams, Stream{
		ProviderID: "one-hd",
		URL:        "https://stream.example/one-hd.m3u8",
		Quality:    "HD",
		HLS:        true,
		Live:       true,
		Headers:    map[string]string{"User-Agent": "Mozilla/5.0"},
	})
	second := batchChannel("two", "Two", "https://stream.example/two.m3u8")

	refreshedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
	result, err := store.Channels().ReplaceForSource(t.Context(), source.ID, []ChannelBatch{first, second}, RefreshMeta{
		RefreshedAt: refreshedAt,
		ETag:        `"v2"`,
	})
	if err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}
	if result.Channels != 2 || result.Streams != 3 || result.Removed != 0 {
		t.Fatalf("result = %+v", result)
	}

	channels, err := store.Channels().List(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(channels) != 2 || channels[0].EPGID == "" {
		t.Fatalf("channels = %+v", channels)
	}

	streams, err := store.Channels().ListStreams(t.Context(), channels[0].ID)
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if len(streams) != 2 || streams[1].Quality != "HD" || !streams[1].HLS {
		t.Fatalf("streams = %+v", streams)
	}
	if streams[1].Headers["User-Agent"] != "Mozilla/5.0" {
		t.Fatalf("headers = %+v", streams[1].Headers)
	}

	grouped, err := store.Channels().ListStreamsBySource(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("ListStreamsBySource: %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("grouped streams = %+v", grouped)
	}

	stored, _, err := store.Sources().Get(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("Get source: %v", err)
	}
	if stored.ItemCount != 2 || stored.RefreshETag != `"v2"` || stored.RefreshError != "" {
		t.Fatalf("source metadata = %+v", stored)
	}
	if !stored.LastSuccessAt.Equal(refreshedAt) {
		t.Fatalf("LastSuccessAt %v, want %v", stored.LastSuccessAt, refreshedAt)
	}
}

func TestReplaceForSourceKeepsFavoritesAndDropsStale(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	initial := batchChannel("keep", "Keep", "https://stream.example/keep.m3u8")
	stale := batchChannel("stale", "Stale", "https://stream.example/stale.m3u8")
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{initial, stale},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	channel, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "keep")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %v, %v", found, err)
	}
	if err := store.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	updated := batchChannel("keep", "Keep renamed", "https://stream.example/keep-v2.m3u8")
	result, err := store.Channels().ReplaceForSource(t.Context(), source.ID, []ChannelBatch{updated}, RefreshMeta{})
	if err != nil {
		t.Fatalf("second ReplaceForSource: %v", err)
	}
	if result.Removed != 1 {
		t.Fatalf("removed %d, want 1", result.Removed)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != channel.ID || favorites[0].Name != "Keep renamed" {
		t.Fatalf("favorites = %+v", favorites)
	}

	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Channel.ID != channel.ID {
		t.Fatalf("recent = %+v", recent)
	}

	if _, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "stale"); err != nil || found {
		t.Fatalf("stale channel found = %v, %v", found, err)
	}
}

func TestReplaceForSourceRollsBack(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("keep", "Keep", "https://stream.example/keep.m3u8")},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	broken := batchChannel("broken", "Broken", "https://stream.example/broken.m3u8")
	broken.Streams[0].ProviderID = strings.Repeat("x", maxProviderIDLength+1)

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{broken},
		RefreshMeta{},
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}

	channels, err := store.Channels().List(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(channels) != 1 || channels[0].Name != "Keep" {
		t.Fatalf("channels after failed refresh = %+v", channels)
	}
	streams, err := store.Channels().ListStreams(t.Context(), channels[0].ID)
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if len(streams) != 1 {
		t.Fatalf("streams after failed refresh = %+v", streams)
	}
}

func TestReplaceForSourceEmptyBatchClears(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("one", "One", "https://stream.example/one.m3u8")},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}

	result, err := store.Channels().ReplaceForSource(t.Context(), source.ID, nil, RefreshMeta{})
	if err != nil {
		t.Fatalf("clear ReplaceForSource: %v", err)
	}
	if result.Channels != 0 || result.Removed != 1 {
		t.Fatalf("result = %+v", result)
	}

	channels, err := store.Channels().List(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("channels = %+v, want none", channels)
	}
}

func TestReplaceForSourceRequiresSource(t *testing.T) {
	store := openTestStore(t)

	_, err := store.Channels().ReplaceForSource(
		t.Context(),
		"missing",
		[]ChannelBatch{batchChannel("one", "One", "https://stream.example/one.m3u8")},
		RefreshMeta{},
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
}

func TestReplaceForSourceRejectsOversizedHeaders(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	item := batchChannel("one", "One", "https://stream.example/one.m3u8")
	item.Streams[0].Headers = map[string]string{"User-Agent": strings.Repeat("x", maxStreamHeaderBytes)}

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{item},
		RefreshMeta{},
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestReplaceForSourceRejectsDuplicateProviders(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	first := batchChannel("same", "One", "https://stream.example/one.m3u8")
	second := batchChannel("same", "Two", "https://stream.example/two.m3u8")

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{first, second},
		RefreshMeta{},
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestReplaceForSourceRejectsHeaderInjection(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	item := batchChannel("one", "One", "https://stream.example/one.m3u8")
	item.Streams[0].Headers = map[string]string{"User-Agent": "ok\r\nX-Injected: 1"}

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{item},
		RefreshMeta{},
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestReplaceForSourceRejectsUnknownHeader(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	item := batchChannel("one", "One", "https://stream.example/one.m3u8")
	item.Streams[0].Headers = map[string]string{"Authorization": "Bearer token"}

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{item},
		RefreshMeta{},
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}
