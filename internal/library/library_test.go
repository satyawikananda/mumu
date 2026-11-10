package library

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"mumu/internal/storage"
)

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "mumu.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	return store
}

func newTestService(t *testing.T, store *storage.Store) *Service {
	t.Helper()

	service, err := New(store)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return service
}

func testSource(t *testing.T, store *storage.Store, location string) storage.Source {
	t.Helper()

	source, err := storage.NewSource(storage.SourceRemotePlaylist, location, "Test source")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	stored, err := store.Sources().Upsert(t.Context(), source)
	if err != nil {
		t.Fatalf("Upsert source: %v", err)
	}

	return stored
}

func testBatch(providerID, name, streamURL string) storage.ChannelBatch {
	return storage.ChannelBatch{
		Channel: storage.Channel{
			ProviderID: providerID,
			Name:       name,
			StreamURL:  streamURL,
			Live:       true,
		},
		Streams: []storage.Stream{{
			ProviderID: providerID + "-sd",
			URL:        streamURL,
			Quality:    "SD",
			Live:       true,
		}},
	}
}

func replaceChannels(
	t *testing.T,
	store *storage.Store,
	sourceID string,
	batch ...storage.ChannelBatch,
) {
	t.Helper()

	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		sourceID,
		batch,
		storage.RefreshMeta{},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}
}

func findChannel(t *testing.T, store *storage.Store, sourceID, providerID string) storage.Channel {
	t.Helper()

	channel, found, err := store.Channels().FindByProvider(t.Context(), sourceID, providerID)
	if err != nil || !found {
		t.Fatalf("FindByProvider %q = %v, %v", providerID, found, err)
	}

	return channel
}

func TestNewRejectsNilStore(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("New(nil) error %v, want ErrInvalid", err)
	}
}

func TestKey(t *testing.T) {
	if got := Key("source", "provider"); got != "source:provider" {
		t.Fatalf("Key = %q, want %q", got, "source:provider")
	}
}

func TestFavoritesKeepSourcesSeparate(t *testing.T) {
	store := openTestStore(t)
	service := newTestService(t, store)

	first := testSource(t, store, "https://a.example/list.m3u")
	second := testSource(t, store, "https://b.example/list.m3u")
	replaceChannels(t, store, first.ID, testBatch("same-id", "News", "https://stream.example/a.m3u8"))
	replaceChannels(t, store, second.ID, testBatch("same-id", "News", "https://stream.example/b.m3u8"))

	base := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	for i, channel := range []storage.Channel{
		findChannel(t, store, first.ID, "same-id"),
		findChannel(t, store, second.ID, "same-id"),
	} {
		if err := store.Favorites().Add(t.Context(), channel.ID, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("Add favorite: %v", err)
		}
	}

	entries, err := service.Favorites(t.Context())
	if err != nil {
		t.Fatalf("Favorites: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries %d, want 2", len(entries))
	}

	byKey := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byKey[entry.Key] = entry
	}
	if len(byKey) != 2 {
		t.Fatalf("entries collapsed into one identity: %+v", byKey)
	}

	firstEntry, ok := byKey[Key(first.ID, "same-id")]
	if !ok {
		t.Fatalf("missing key %q in %+v", Key(first.ID, "same-id"), entries)
	}
	if firstEntry.Source.ID != first.ID || firstEntry.Channel.SourceID != first.ID {
		t.Fatalf("first entry source = %+v", firstEntry)
	}
	if firstEntry.Channel.Name != "News" || firstEntry.Channel.ProviderID != "same-id" {
		t.Fatalf("first entry channel = %+v", firstEntry.Channel)
	}
	if len(firstEntry.Channel.Streams) != 1 || firstEntry.Channel.Streams[0].URL != "https://stream.example/a.m3u8" {
		t.Fatalf("first entry streams = %+v", firstEntry.Channel.Streams)
	}
	if !firstEntry.Playable {
		t.Fatal("first entry is not playable")
	}
	if !firstEntry.PlayedAt.IsZero() || firstEntry.PlayCount != 0 {
		t.Fatalf("favorite entry has playback data: %+v", firstEntry)
	}

	secondEntry, ok := byKey[Key(second.ID, "same-id")]
	if !ok {
		t.Fatalf("missing key %q in %+v", Key(second.ID, "same-id"), entries)
	}
	if secondEntry.Source.ID != second.ID || secondEntry.Channel.SourceID != second.ID {
		t.Fatalf("second entry source = %+v", secondEntry)
	}
	if secondEntry.Channel.Streams[0].URL != "https://stream.example/b.m3u8" {
		t.Fatalf("second entry streams = %+v", secondEntry.Channel.Streams)
	}

	// Most recently added first.
	if entries[0].Key != Key(second.ID, "same-id") {
		t.Fatalf("first entry key %q, want the most recent favorite", entries[0].Key)
	}
}

func TestFavoriteOrder(t *testing.T) {
	store := openTestStore(t)
	service := newTestService(t, store)
	source := testSource(t, store, "https://example.com/list.m3u")

	replaceChannels(t, store, source.ID,
		testBatch("one", "One", "https://stream.example/one.m3u8"),
		testBatch("two", "Two", "https://stream.example/two.m3u8"),
		testBatch("three", "Three", "https://stream.example/three.m3u8"),
	)

	base := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	for i, providerID := range []string{"one", "two", "three"} {
		channel := findChannel(t, store, source.ID, providerID)
		if err := store.Favorites().Add(t.Context(), channel.ID, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("Add favorite: %v", err)
		}
	}

	entries, err := service.Favorites(t.Context())
	if err != nil {
		t.Fatalf("Favorites: %v", err)
	}

	want := []string{"three", "two", "one"}
	if len(entries) != len(want) {
		t.Fatalf("entries %d, want %d", len(entries), len(want))
	}
	for i, providerID := range want {
		if entries[i].Channel.ProviderID != providerID {
			t.Fatalf("entry %d = %q, want %q", i, entries[i].Channel.ProviderID, providerID)
		}
		if entries[i].Key != Key(source.ID, providerID) {
			t.Fatalf("entry %d key = %q", i, entries[i].Key)
		}
	}
}

func TestRecentLimitAndOrder(t *testing.T) {
	store := openTestStore(t)
	service := newTestService(t, store)
	source := testSource(t, store, "https://example.com/list.m3u")

	replaceChannels(t, store, source.ID,
		testBatch("one", "One", "https://stream.example/one.m3u8"),
		testBatch("two", "Two", "https://stream.example/two.m3u8"),
		testBatch("three", "Three", "https://stream.example/three.m3u8"),
	)

	base := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	for i, providerID := range []string{"one", "two", "three"} {
		channel := findChannel(t, store, source.ID, providerID)
		if err := store.Recent().MarkPlayed(t.Context(), channel.ID, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("MarkPlayed: %v", err)
		}
	}

	entries, err := service.Recent(t.Context(), 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries %d, want 2", len(entries))
	}
	if entries[0].Channel.ProviderID != "three" || entries[1].Channel.ProviderID != "two" {
		t.Fatalf("order = %q, %q, want three, two", entries[0].Channel.ProviderID, entries[1].Channel.ProviderID)
	}
	if entries[0].PlayCount != 1 {
		t.Fatalf("play count %d, want 1", entries[0].PlayCount)
	}
	if entries[0].PlayedAt.IsZero() {
		t.Fatal("recent entry has no played time")
	}

	all, err := service.Recent(t.Context(), 0)
	if err != nil {
		t.Fatalf("Recent(0): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Recent(0) entries %d, want 3", len(all))
	}

	clamped, err := service.Recent(t.Context(), storage.RecentLimit+50)
	if err != nil {
		t.Fatalf("Recent(clamped): %v", err)
	}
	if len(clamped) != 3 {
		t.Fatalf("Recent(clamped) entries %d, want 3", len(clamped))
	}
}

func TestLast(t *testing.T) {
	store := openTestStore(t)
	service := newTestService(t, store)

	if _, ok, err := service.Last(t.Context()); err != nil || ok {
		t.Fatalf("Last on empty library = %v, %v, want no entry", ok, err)
	}

	source := testSource(t, store, "https://example.com/list.m3u")
	replaceChannels(t, store, source.ID,
		testBatch("one", "One", "https://stream.example/one.m3u8"),
		testBatch("two", "Two", "https://stream.example/two.m3u8"),
	)

	base := time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)
	older := findChannel(t, store, source.ID, "one")
	newer := findChannel(t, store, source.ID, "two")
	if err := store.Recent().MarkPlayed(t.Context(), older.ID, base); err != nil {
		t.Fatalf("MarkPlayed older: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), newer.ID, base.Add(time.Hour)); err != nil {
		t.Fatalf("MarkPlayed newer: %v", err)
	}

	entry, ok, err := service.Last(t.Context())
	if err != nil || !ok {
		t.Fatalf("Last = %v, %v, want an entry", ok, err)
	}
	if entry.Key != Key(source.ID, "two") {
		t.Fatalf("Last key %q, want %q", entry.Key, Key(source.ID, "two"))
	}
	if !entry.PlayedAt.Equal(base.Add(time.Hour)) || entry.PlayCount != 1 {
		t.Fatalf("Last entry = %+v", entry)
	}
	if !entry.Playable {
		t.Fatal("Last entry is not playable")
	}
}

func TestToggleFavorite(t *testing.T) {
	store := openTestStore(t)
	service := newTestService(t, store)
	source := testSource(t, store, "https://example.com/list.m3u")
	replaceChannels(t, store, source.ID, testBatch("news", "News", "https://stream.example/news.m3u8"))
	channel := findChannel(t, store, source.ID, "news")

	favorite, err := service.ToggleFavorite(t.Context(), source.ID, "news")
	if err != nil || !favorite {
		t.Fatalf("first toggle = %v, %v, want true", favorite, err)
	}
	has, err := store.Favorites().Has(t.Context(), channel.ID)
	if err != nil || !has {
		t.Fatalf("Has after add = %v, %v, want true", has, err)
	}

	favorite, err = service.ToggleFavorite(t.Context(), source.ID, "news")
	if err != nil || favorite {
		t.Fatalf("second toggle = %v, %v, want false", favorite, err)
	}
	has, err = store.Favorites().Has(t.Context(), channel.ID)
	if err != nil || has {
		t.Fatalf("Has after remove = %v, %v, want false", has, err)
	}

	favorite, err = service.ToggleFavorite(t.Context(), source.ID, "news")
	if err != nil || !favorite {
		t.Fatalf("third toggle = %v, %v, want true", favorite, err)
	}

	// A repeated add keeps one row, so one toggle still clears the favorite.
	if err := store.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("Add again: %v", err)
	}
	favorite, err = service.ToggleFavorite(t.Context(), source.ID, "news")
	if err != nil || favorite {
		t.Fatalf("toggle after repeated add = %v, %v, want false", favorite, err)
	}
	entries, err := service.Favorites(t.Context())
	if err != nil {
		t.Fatalf("Favorites: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("favorites = %+v, want none", entries)
	}
}

func TestToggleFavoriteRejectsUnknownChannel(t *testing.T) {
	store := openTestStore(t)
	service := newTestService(t, store)
	source := testSource(t, store, "https://example.com/list.m3u")
	replaceChannels(t, store, source.ID, testBatch("news", "News", "https://stream.example/news.m3u8"))

	if _, err := service.ToggleFavorite(t.Context(), source.ID, "missing"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown provider error %v, want ErrNotFound", err)
	}
	if _, err := service.ToggleFavorite(t.Context(), "missing", "news"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown source error %v, want ErrNotFound", err)
	}
	if _, err := service.ToggleFavorite(t.Context(), " ", "news"); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("empty source error %v, want ErrInvalid", err)
	}
	if _, err := service.ToggleFavorite(t.Context(), source.ID, ""); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("empty provider error %v, want ErrInvalid", err)
	}

	entries, err := service.Favorites(t.Context())
	if err != nil {
		t.Fatalf("Favorites: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("favorites = %+v, want none", entries)
	}
}

func TestPlayable(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		url      string
		streams  []storage.Stream
		want     bool
		wantList bool
	}{
		{
			name:    "https stream",
			enabled: true,
			url:     "https://stream.example/news.m3u8",
			streams: []storage.Stream{{
				ProviderID: "news-sd",
				URL:        "https://stream.example/news.m3u8",
				Live:       true,
			}},
			want:     true,
			wantList: true,
		},
		{
			name:    "http stream",
			enabled: true,
			url:     "http://stream.example/news.m3u8",
			streams: []storage.Stream{{
				ProviderID: "news-sd",
				URL:        "http://stream.example/news.m3u8",
				Live:       true,
			}},
			want:     true,
			wantList: true,
		},
		{
			name:    "one playable variant",
			enabled: true,
			url:     "rtsp://stream.example/camera",
			streams: []storage.Stream{
				{ProviderID: "camera-rtsp", URL: "rtsp://stream.example/camera"},
				{ProviderID: "camera-hls", URL: "https://stream.example/camera.m3u8"},
			},
			want:     true,
			wantList: true,
		},
		{
			name:    "disabled source",
			enabled: false,
			url:     "https://stream.example/news.m3u8",
			streams: []storage.Stream{{
				ProviderID: "news-sd",
				URL:        "https://stream.example/news.m3u8",
			}},
			want:     false,
			wantList: true,
		},
		{
			name:     "no variant rows",
			enabled:  true,
			url:      "https://stream.example/news.m3u8",
			streams:  nil,
			want:     false,
			wantList: true,
		},
		{
			name:    "unsupported scheme",
			enabled: true,
			url:     "rtsp://stream.example/camera",
			streams: []storage.Stream{
				{ProviderID: "camera-rtsp", URL: "rtsp://stream.example/camera"},
			},
			want:     false,
			wantList: true,
		},
		{
			name:    "stream URL without host",
			enabled: true,
			url:     "https://stream.example/news.m3u8",
			streams: []storage.Stream{{
				ProviderID: "news-sd",
				URL:        "http:///news.m3u8",
			}},
			want:     false,
			wantList: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t)
			service := newTestService(t, store)
			source := testSource(t, store, "https://example.com/list.m3u")

			var channel storage.Channel
			if test.streams == nil {
				stored, err := store.Channels().Upsert(t.Context(), source.ID, storage.Channel{
					ProviderID: "news",
					Name:       "News",
					StreamURL:  test.url,
					Live:       true,
				})
				if err != nil {
					t.Fatalf("Upsert channel: %v", err)
				}
				channel = stored
			} else {
				replaceChannels(t, store, source.ID, storage.ChannelBatch{
					Channel: storage.Channel{
						ProviderID: "news",
						Name:       "News",
						StreamURL:  test.url,
						Live:       true,
					},
					Streams: test.streams,
				})
				channel = findChannel(t, store, source.ID, "news")
			}

			if err := store.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
				t.Fatalf("Add favorite: %v", err)
			}
			if !test.enabled {
				if err := store.Sources().SetEnabled(t.Context(), source.ID, false); err != nil {
					t.Fatalf("SetEnabled: %v", err)
				}
			}

			entries, err := service.Favorites(t.Context())
			if err != nil {
				t.Fatalf("Favorites: %v", err)
			}
			if test.wantList && len(entries) != 1 {
				t.Fatalf("entries %d, want 1", len(entries))
			}
			if entries[0].Playable != test.want {
				t.Fatalf("Playable = %v, want %v", entries[0].Playable, test.want)
			}
			if entries[0].Source.Enabled != test.enabled {
				t.Fatalf("Source.Enabled = %v, want %v", entries[0].Source.Enabled, test.enabled)
			}
		})
	}
}

func TestEntriesPersistAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mumu.db")
	ctx := t.Context()

	store, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	source := testSource(t, store, "https://example.com/list.m3u")
	replaceChannels(t, store, source.ID, testBatch("news", "News", "https://stream.example/news.m3u8"))
	channel := findChannel(t, store, source.ID, "news")

	favoritedAt := time.Date(2026, 11, 2, 7, 0, 0, 0, time.UTC)
	playedAt := time.Date(2026, 11, 2, 7, 30, 0, 0, time.UTC)
	if err := store.Favorites().Add(ctx, channel.ID, favoritedAt); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(ctx, channel.ID, playedAt); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}
	if err := store.Recent().MarkPlayed(ctx, channel.ID, playedAt); err != nil {
		t.Fatalf("MarkPlayed again: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	service := newTestService(t, reopened)

	favorites, err := service.Favorites(ctx)
	if err != nil {
		t.Fatalf("Favorites: %v", err)
	}
	if len(favorites) != 1 {
		t.Fatalf("favorites %d, want 1", len(favorites))
	}
	if favorites[0].Key != Key(source.ID, "news") || !favorites[0].Playable {
		t.Fatalf("favorite entry = %+v", favorites[0])
	}

	last, ok, err := service.Last(ctx)
	if err != nil || !ok {
		t.Fatalf("Last = %v, %v, want an entry", ok, err)
	}
	if last.Key != Key(source.ID, "news") {
		t.Fatalf("Last key %q, want %q", last.Key, Key(source.ID, "news"))
	}
	if !last.PlayedAt.Equal(playedAt) {
		t.Fatalf("Last played at %v, want %v", last.PlayedAt, playedAt)
	}
	if last.PlayCount != 2 {
		t.Fatalf("Last play count %d, want 2", last.PlayCount)
	}
}
