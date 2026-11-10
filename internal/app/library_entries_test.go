package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/library"
	"mumu/internal/sources"
	"mumu/internal/storage"
	"mumu/internal/ui"
)

// seedPlaylist stores one user playlist with the given channels.
func seedPlaylist(
	t *testing.T,
	a *App,
	location, name string,
	channels []catalog.Channel,
) storage.Source {
	t.Helper()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v, want an open library", store, err)
	}

	source, err := storage.NewSource(storage.SourceRemotePlaylist, location, name)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	if _, err := store.Sources().Upsert(context.Background(), source); err != nil {
		t.Fatalf("Upsert source: %v", err)
	}

	batch, err := sources.ToStorageBatch(channels)
	if err != nil {
		t.Fatalf("ToStorageBatch: %v", err)
	}
	if _, err := store.Channels().ReplaceForSource(
		context.Background(),
		source.ID,
		batch,
		storage.RefreshMeta{RefreshedAt: time.Now(), ItemCount: len(batch)},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}

	return source
}

// testChannel is one live channel with a distinct stream URL.
func testChannel(providerID, name string) catalog.Channel {
	return catalog.Channel{
		ProviderID: providerID,
		Name:       name,
		Group:      "News",
		Streams: []catalog.Stream{{
			URL:  "https://stream.example/" + providerID + ".m3u8",
			Live: true,
		}},
	}
}

// sourceChannel scopes a test channel to one stored source.
func sourceChannel(sourceID, providerID, name string) catalog.Channel {
	channel := testChannel(providerID, name)
	channel.SourceID = sourceID

	return channel
}

// The Library lists stored favorites and recent channels with their source.
func TestLibraryListsFavoritesAndRecent(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	channel, found, err := store.Channels().FindByProvider(context.Background(), source.ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", channel, found, err)
	}
	if err := store.Favorites().Add(context.Background(), channel.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(context.Background(), channel.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	a.loadLibrary(context.Background())

	state := libraryState(a, f)
	if state.Kind != ui.LibraryReady {
		t.Fatalf("library kind %v, want ready", state.Kind)
	}
	if len(state.Favorites) != 1 || len(state.Recent) != 1 {
		t.Fatalf("state %+v, want one favorite and one recent", state)
	}

	favorite := state.Favorites[0]
	if favorite.Name != "My News" || favorite.SourceName != "My list" {
		t.Fatalf("favorite %+v, want the channel and its source", favorite)
	}
	if !favorite.Favorite || !favorite.Playable || !favorite.SourceEnabled {
		t.Fatalf("favorite %+v, want a playable favorite", favorite)
	}
	if state.Recent[0].PlayedAt.IsZero() {
		t.Fatalf("recent %+v, want a playback time", state.Recent[0])
	}
}

// Toggling from Discover converges Watch, Discover and the stored favorite.
func TestToggleFavoriteUpdatesPagesAndStore(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	a.loadLibrary(context.Background())
	a.discoverIndex.setCatalog(
		[]catalog.Channel{sourceChannel(source.ID, "my-news", "My News")},
		discoverFacets{names: map[string]string{source.ID: "My list"}},
		discoverBase{},
	)

	key := library.Key(source.ID, "my-news")
	a.mu.Lock()
	a.media.channel = catalog.Channel{SourceID: source.ID, ProviderID: "my-news", Name: "My News"}
	a.mu.Unlock()

	a.toggleFavoriteKey(key)
	waitFor(t, f, "favorite stored", func() bool {
		return a.shell.Discover().Favorite(key)
	})
	if !a.shell.Watch().State().Favorite {
		t.Fatal("Watch did not follow the favorite")
	}
	waitFor(t, f, "library favorite", func() bool {
		favorites := a.shell.Library().State().Favorites

		return len(favorites) == 1 && favorites[0].Name == "My News"
	})

	store, _ := a.Store()
	channel, _, _ := store.Channels().FindByProvider(context.Background(), source.ID, "my-news")
	waitFor(t, f, "favorite stored", func() bool {
		has, err := store.Favorites().Has(context.Background(), channel.ID)

		return err == nil && has
	})

	waitFor(t, f, "favorite write finished", func() bool {
		return !a.libraryData.favoriteRunning(key)
	})

	a.toggleFavoriteKey(key)
	waitFor(t, f, "favorite removed", func() bool {
		has, err := store.Favorites().Has(context.Background(), channel.ID)

		return err == nil && !has
	})
	if a.shell.Watch().State().Favorite {
		t.Fatal("Watch kept the removed favorite")
	}
}

// Recording real playback puts the channel in the Library's recent section.
func TestRecordRecentUpdatesLibrary(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	stored, found, err := store.Channels().FindByProvider(context.Background(), source.ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
	}

	a.loadLibrary(context.Background())
	a.recordRecent(context.Background(), catalog.Channel{
		SourceID: source.ID, ProviderID: "my-news", Name: "My News",
	})

	state := libraryState(a, f)
	if len(state.Recent) != 1 || state.Recent[0].Name != "My News" {
		t.Fatalf("recent %+v, want the played channel", state.Recent)
	}
}

// Two sources with the same provider ID keep independent favorite state.
func TestFavoriteIdentityIsSourceScoped(t *testing.T) {
	a, f := newLibraryTestApp(t)
	first := seedPlaylist(t, a, "https://example.com/first.m3u", "First", []catalog.Channel{
		testChannel("news", "News"),
	})
	second := seedPlaylist(t, a, "https://example.com/second.m3u", "Second", []catalog.Channel{
		testChannel("news", "News"),
	})

	a.loadLibrary(context.Background())
	a.discoverIndex.setCatalog(
		[]catalog.Channel{sourceChannel(first.ID, "news", "News")},
		discoverFacets{names: map[string]string{first.ID: "First"}},
		discoverBase{},
	)

	firstKey := library.Key(first.ID, "news")
	secondKey := library.Key(second.ID, "news")

	a.toggleFavoriteKey(firstKey)
	waitFor(t, f, "first favorite", func() bool { return a.shell.Discover().Favorite(firstKey) })

	if a.shell.Discover().Favorite(secondKey) {
		t.Fatal("the second source was marked as a favorite")
	}
	if a.libraryData.favorite(secondKey) {
		t.Fatal("the favorite cache mixed up two sources")
	}
}

// A disabled source explains itself instead of starting playback.
func TestLibraryPlayRejectsDisabledSource(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	channel, found, err := store.Channels().FindByProvider(context.Background(), source.ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", channel, found, err)
	}
	if err := store.Favorites().Add(context.Background(), channel.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Sources().SetEnabled(context.Background(), source.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	a.loadLibrary(context.Background())
	a.playLibraryChannel(library.Key(source.ID, "my-news"))

	state := libraryState(a, f)
	if !strings.Contains(state.Message, "disabled") {
		t.Fatalf("message %q, want the disabled-source guidance", state.Message)
	}
	if len(state.Recent) > 0 {
		t.Fatalf("state %+v, want no playback", state)
	}
}

// Removing a source leaves no favorite or recent row behind.
func TestRemovedSourceLeavesNoLibraryRows(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	channel, found, err := store.Channels().FindByProvider(context.Background(), source.ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", channel, found, err)
	}
	if err := store.Favorites().Add(context.Background(), channel.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(context.Background(), channel.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	a.loadLibrary(context.Background())
	a.removeLibrarySource(source.ID)

	waitFor(t, f, "source removed", func() bool {
		state := a.shell.Library().State()

		return len(state.Sources) == 0 && len(state.Favorites) == 0 && len(state.Recent) == 0
	})
}

// Removing a source clears the active channel's favorite on Watch too.
func TestRemovedSourceClearsActiveFavorite(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	channel, found, err := store.Channels().FindByProvider(context.Background(), source.ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", channel, found, err)
	}
	if err := store.Favorites().Add(context.Background(), channel.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}

	a.loadLibrary(context.Background())
	a.mu.Lock()
	a.media.channel = catalog.Channel{SourceID: source.ID, ProviderID: "my-news", Name: "My News"}
	a.mu.Unlock()
	a.publishFavorites()

	if !a.shell.Watch().State().Favorite {
		t.Fatal("Watch did not show the stored favorite")
	}

	a.removeLibrarySource(source.ID)
	waitFor(t, f, "watch favorite cleared", func() bool {
		return !a.shell.Watch().State().Favorite
	})
}

// A reload may not clobber the optimistic value of an in-flight toggle.
func TestSetEntriesKeepsInFlightFavorite(t *testing.T) {
	cache := newLibraryCache()
	if !cache.beginFavorite("s:p") {
		t.Fatal("beginFavorite rejected the first toggle")
	}
	cache.setFavorite("s:p", true)

	cache.setEntries(nil, nil)
	if !cache.favorite("s:p") {
		t.Fatal("a stale reload cleared the in-flight favorite")
	}

	cache.endFavorite("s:p")
	cache.setEntries(nil, nil)
	if cache.favorite("s:p") {
		t.Fatal("a completed reload did not follow the store")
	}
}

// A policy-empty import explains the policy instead of the read failure.
func TestLibraryErrorTextEmptyPreview(t *testing.T) {
	got := libraryErrorText(sources.ErrEmptyPreview)
	if !strings.Contains(got, "policy") {
		t.Fatalf("message %q, want the source policy guidance", got)
	}
}

// queuedDispatcher keeps updates until the test runs them in a chosen order.
type queuedDispatcher struct {
	fns []func()
}

func (q *queuedDispatcher) Update(fn func()) { q.fns = append(q.fns, fn) }

func (q *queuedDispatcher) Invalidate() {}

// flush runs every queued update in order, as the UI thread delivers them.
func (q *queuedDispatcher) flush() {
	for len(q.fns) > 0 {
		fn := q.fns[0]
		q.fns = q.fns[1:]
		fn()
	}
}

// A queued update must read the cache when it runs, not when it was enqueued.
func TestPublishLibraryUsesNewestCache(t *testing.T) {
	a := New()
	q := &queuedDispatcher{}
	a.dispatcher = q

	old := catalog.Source{ID: "old", Kind: catalog.SourceRemotePlaylist, DisplayName: "Old", Enabled: true}
	fresh := catalog.Source{ID: "new", Kind: catalog.SourceRemotePlaylist, DisplayName: "New", Enabled: true}

	a.libraryData.setSources([]catalog.Source{old})
	a.publishLibrary()
	a.libraryData.setSources([]catalog.Source{fresh})
	a.publishLibrary()

	// Run the older update last: it must still show the newest cache.
	for i := len(q.fns) - 1; i >= 0; i-- {
		q.fns[i]()
	}

	state := a.shell.Library().State()
	if len(state.Sources) != 1 || state.Sources[0].ID != "new" {
		t.Fatalf("state %+v, want the newest source", state.Sources)
	}
}
