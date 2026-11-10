package storage

import (
	"errors"
	"fmt"
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

func findStoredChannel(t *testing.T, store *Store, sourceID, providerID string) Channel {
	t.Helper()

	channel, found, err := store.Channels().FindByProvider(t.Context(), sourceID, providerID)
	if err != nil || !found {
		t.Fatalf("FindByProvider %q = %v, %v", providerID, found, err)
	}

	return channel
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

func TestReplaceForSourceRemapsEntriesWhenProviderIDChanges(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	const streamURL = "https://stream.example/news.m3u8"
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("old-id", "News", streamURL)},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	stale, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "old-id")
	if err != nil || !found {
		t.Fatalf("FindByProvider old-id = %v, %v", found, err)
	}

	createdAt := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	playedAt := createdAt.Add(time.Hour)
	lastPlayedAt := playedAt.Add(time.Minute)
	if err := store.Favorites().Add(t.Context(), stale.ID, createdAt); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), stale.ID, playedAt); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), stale.ID, lastPlayedAt); err != nil {
		t.Fatalf("MarkPlayed again: %v", err)
	}

	// The provider started publishing a tvg-id: the identity changed but the
	// stream did not. The favorite and history must follow the stream.
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("new-id", "News", streamURL)},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("remapping ReplaceForSource: %v", err)
	}

	replacement, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "new-id")
	if err != nil || !found {
		t.Fatalf("FindByProvider new-id = %v, %v", found, err)
	}
	if replacement.ID == stale.ID {
		t.Fatal("provider change reused the stale channel row")
	}
	if _, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "old-id"); err != nil || found {
		t.Fatalf("stale channel found = %v, %v", found, err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != replacement.ID {
		t.Fatalf("favorites = %+v, want the replacement channel %d", favorites, replacement.ID)
	}
	keptCreatedAt := queryInt(t, store.db, `SELECT created_at FROM favorites WHERE channel_id = ?`, replacement.ID)
	if keptCreatedAt != unixMillis(createdAt) {
		t.Fatalf("favorite created_at %d, want %d", keptCreatedAt, unixMillis(createdAt))
	}

	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Channel.ID != replacement.ID {
		t.Fatalf("recent = %+v, want the replacement channel %d", recent, replacement.ID)
	}
	if !recent[0].PlayedAt.Equal(lastPlayedAt) {
		t.Fatalf("played_at %v, want %v", recent[0].PlayedAt, lastPlayedAt)
	}
	if recent[0].PlayCount != 2 {
		t.Fatalf("play count %d, want 2 (a merged copy must not sum)", recent[0].PlayCount)
	}

	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM favorites WHERE channel_id = ?`,
		stale.ID,
	); rows != 0 {
		t.Fatalf("stale favorite rows %d, want 0", rows)
	}
	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM recent_channels WHERE channel_id = ?`,
		stale.ID,
	); rows != 0 {
		t.Fatalf("stale recent rows %d, want 0", rows)
	}
}

func TestReplaceForSourceDropsEntriesWhenStreamURLLeaves(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("old-id", "News", "https://stream.example/old.m3u8")},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	stale, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "old-id")
	if err != nil || !found {
		t.Fatalf("FindByProvider old-id = %v, %v", found, err)
	}
	if err := store.Favorites().Add(t.Context(), stale.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), stale.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	// A different stream is a different channel: keeping the favorite would
	// point it at content the user never chose.
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("new-id", "News", "https://stream.example/new.m3u8")},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 0 {
		t.Fatalf("favorites = %+v, want none", favorites)
	}
	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("recent = %+v, want none", recent)
	}
	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM favorites WHERE channel_id = ?`,
		stale.ID,
	); rows != 0 {
		t.Fatalf("stale favorite rows %d, want 0", rows)
	}
	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM recent_channels WHERE channel_id = ?`,
		stale.ID,
	); rows != 0 {
		t.Fatalf("stale recent rows %d, want 0", rows)
	}
}

func TestReplaceForSourceRemapStaysInOneSource(t *testing.T) {
	store := openTestStore(t)
	first := mustSource(t, store, SourceRemotePlaylist, "https://a.example/list.m3u")
	second := mustSource(t, store, SourceRemotePlaylist, "https://b.example/list.m3u")

	const sharedURL = "https://stream.example/shared.m3u8"
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		first.ID,
		[]ChannelBatch{batchChannel("a-keep", "News", sharedURL)},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("first ReplaceForSource: %v", err)
	}
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		second.ID,
		[]ChannelBatch{batchChannel("b-old", "News", sharedURL)},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("second ReplaceForSource: %v", err)
	}

	kept, found, err := store.Channels().FindByProvider(t.Context(), first.ID, "a-keep")
	if err != nil || !found {
		t.Fatalf("FindByProvider a-keep = %v, %v", found, err)
	}
	gone, found, err := store.Channels().FindByProvider(t.Context(), second.ID, "b-old")
	if err != nil || !found {
		t.Fatalf("FindByProvider b-old = %v, %v", found, err)
	}
	if err := store.Favorites().Add(t.Context(), kept.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), kept.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	// The other source lost that stream. Its rows must not adopt the first
	// source's favorite just because the URL and the name match.
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		second.ID,
		[]ChannelBatch{batchChannel("b-new", "News", "https://stream.example/other.m3u8")},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("third ReplaceForSource: %v", err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != kept.ID {
		t.Fatalf("favorites = %+v, want the first source channel %d", favorites, kept.ID)
	}
	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Channel.ID != kept.ID {
		t.Fatalf("recent = %+v, want the first source channel %d", recent, kept.ID)
	}
	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM favorites WHERE channel_id = ?`,
		gone.ID,
	); rows != 0 {
		t.Fatalf("stale favorite rows %d, want 0", rows)
	}
	replacement, found, err := store.Channels().FindByProvider(t.Context(), second.ID, "b-new")
	if err != nil || !found {
		t.Fatalf("FindByProvider b-new = %v, %v", found, err)
	}
	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM favorites WHERE channel_id = ?`,
		replacement.ID,
	); rows != 0 {
		t.Fatalf("cross-source favorite rows %d, want 0", rows)
	}
}

func TestReplaceForSourceRemapMergesExistingHistory(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	const streamURL = "https://stream.example/news.m3u8"
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{
			batchChannel("keep", "Keep", streamURL),
			batchChannel("old", "Old", streamURL),
		},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	kept := findStoredChannel(t, store, source.ID, "keep")
	stale := findStoredChannel(t, store, source.ID, "old")

	keptFavoriteAt := time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)
	staleFavoriteAt := keptFavoriteAt.Add(2 * time.Hour)
	if err := store.Favorites().Add(t.Context(), kept.ID, keptFavoriteAt); err != nil {
		t.Fatalf("Add kept favorite: %v", err)
	}
	if err := store.Favorites().Add(t.Context(), stale.ID, staleFavoriteAt); err != nil {
		t.Fatalf("Add stale favorite: %v", err)
	}

	base := keptFavoriteAt.Add(-time.Hour)
	for i := range 5 {
		if err := store.Recent().MarkPlayed(
			t.Context(),
			kept.ID,
			base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("MarkPlayed kept: %v", err)
		}
	}
	latest := base.Add(2 * time.Hour)
	if err := store.Recent().MarkPlayed(t.Context(), stale.ID, latest); err != nil {
		t.Fatalf("MarkPlayed stale: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), stale.ID, latest); err != nil {
		t.Fatalf("MarkPlayed stale again: %v", err)
	}

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("keep", "Keep", streamURL)},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("merging ReplaceForSource: %v", err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != kept.ID {
		t.Fatalf("favorites = %+v, want only the surviving channel %d", favorites, kept.ID)
	}
	keptat := queryInt(t, store.db, `SELECT created_at FROM favorites WHERE channel_id = ?`, kept.ID)
	if keptat != unixMillis(keptFavoriteAt) {
		t.Fatalf("favorite created_at %d, want the surviving favorite %d", keptat, unixMillis(keptFavoriteAt))
	}

	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Channel.ID != kept.ID {
		t.Fatalf("recent = %+v, want only the surviving channel %d", recent, kept.ID)
	}
	if !recent[0].PlayedAt.Equal(latest) {
		t.Fatalf("played_at %v, want the newest %v", recent[0].PlayedAt, latest)
	}
	if recent[0].PlayCount != 5 {
		t.Fatalf("play count %d, want 5 (the higher count, never the sum)", recent[0].PlayCount)
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

func TestReplaceForSourceRemapFansOutToSharedURL(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	const streamURL = "https://stream.example/news.m3u8"
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{batchChannel("old", "Old", streamURL)},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	stale := findStoredChannel(t, store, source.ID, "old")
	if err := store.Favorites().Add(t.Context(), stale.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), stale.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	// Two new channels offer the same stream. Both keep a copy; only the stale
	// rows are removed, so no favorite is lost.
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{
			batchChannel("new-a", "New A", streamURL),
			batchChannel("new-b", "New B", streamURL),
		},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("fan-out ReplaceForSource: %v", err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("favorites = %+v, want one copy per surviving channel", favorites)
	}
	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent = %+v, want one copy per surviving channel", recent)
	}
	if rows := queryInt(
		t,
		store.db,
		`SELECT COUNT(*) FROM favorites WHERE channel_id = ?`,
		stale.ID,
	); rows != 0 {
		t.Fatalf("stale favorite rows %d, want 0", rows)
	}
}

func TestReplaceForSourceRemapsAcrossChunks(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	count := deleteChunkSize + 1
	old := make([]ChannelBatch, 0, count)
	fresh := make([]ChannelBatch, 0, count)
	for i := range count {
		url := fmt.Sprintf("https://stream.example/%d.m3u8", i)
		old = append(old, batchChannel(fmt.Sprintf("old-%d", i), fmt.Sprintf("Channel %d", i), url))
		fresh = append(fresh, batchChannel(fmt.Sprintf("new-%d", i), fmt.Sprintf("Channel %d", i), url))
	}

	if _, err := store.Channels().ReplaceForSource(t.Context(), source.ID, old, RefreshMeta{}); err != nil {
		t.Fatalf("initial ReplaceForSource: %v", err)
	}

	// Favorites in the first and in the second chunk must both survive.
	for _, index := range []int{0, count - 1} {
		channel := findStoredChannel(t, store, source.ID, fmt.Sprintf("old-%d", index))
		if err := store.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
			t.Fatalf("Add favorite %d: %v", index, err)
		}
		if err := store.Recent().MarkPlayed(t.Context(), channel.ID, time.Now()); err != nil {
			t.Fatalf("MarkPlayed %d: %v", index, err)
		}
	}

	if _, err := store.Channels().ReplaceForSource(t.Context(), source.ID, fresh, RefreshMeta{}); err != nil {
		t.Fatalf("remapping ReplaceForSource: %v", err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("favorites = %d, want 2 across chunks", len(favorites))
	}
	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent = %d, want 2 across chunks", len(recent))
	}
	for _, channel := range favorites {
		if !strings.HasPrefix(channel.ProviderID, "new-") {
			t.Fatalf("favorite channel %q, want a surviving provider ID", channel.ProviderID)
		}
	}
}
