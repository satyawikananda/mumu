package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/ui"
)

// homeFeed reads the published Home state under the fake window's lock.
func homeFeed(a *App, f *fakeDispatcher) ui.HomeState {
	f.mu.Lock()
	defer f.mu.Unlock()

	return a.shell.Home().State()
}

// savedChannels stores count channels in one source.
func savedChannels(t *testing.T, a *App, count int) {
	t.Helper()

	channels := make([]catalog.Channel, 0, count)
	for i := range count {
		channels = append(channels, testChannel(fmt.Sprintf("ch-%d", i), fmt.Sprintf("Channel %d", i)))
	}
	seedPlaylist(t, a, "https://example.com/list.m3u", "My list", channels)
}

// Home is built from persisted favorites and recent channels, with capped
// rails and full counts.
func TestHomeFeedFromSavedChannels(t *testing.T) {
	a, f := newLibraryTestApp(t)
	savedChannels(t, a, 8)

	store, _ := a.Store()
	ctx := context.Background()
	sources, _ := store.Sources().List(ctx)
	for i := range 7 {
		stored, found, err := store.Channels().FindByProvider(ctx, sources[0].ID, fmt.Sprintf("ch-%d", i))
		if err != nil || !found {
			t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
		}
		playedAt := time.Now().UTC().Add(-time.Duration(7-i) * time.Minute)
		if err := store.Recent().MarkPlayed(ctx, stored.ID, playedAt); err != nil {
			t.Fatalf("MarkPlayed: %v", err)
		}
		if i < 2 {
			if err := store.Favorites().Add(ctx, stored.ID, playedAt); err != nil {
				t.Fatalf("Add favorite: %v", err)
			}
		}
	}

	a.loadLibrary(ctx)

	state := homeFeed(a, f)
	if state.Kind != ui.HomeResume {
		t.Fatalf("kind %v, want HomeResume", state.Kind)
	}
	if state.Hero.Name != "Channel 6" || state.Hero.SourceName != "My list" {
		t.Fatalf("hero %+v, want the last played channel", state.Hero)
	}
	if state.RecentCount != 7 || len(state.Recent) != homeRailLimit {
		t.Fatalf("recent %d of %d, want 6 of 7", len(state.Recent), state.RecentCount)
	}
	if state.FavoriteCount != 2 || len(state.Favorites) != 2 {
		t.Fatalf("favorites %d of %d, want 2", len(state.Favorites), state.FavoriteCount)
	}
	if !state.Hero.Live || !state.Hero.Playable {
		t.Fatalf("hero %+v, want a playable live channel", state.Hero)
	}
	if state.Hero.Key == "" || state.Hero.Key != state.Recent[0].Key {
		t.Fatalf("hero key %q, recent[0] %q, want one identity", state.Hero.Key, state.Recent[0].Key)
	}
}

// A failed library read is a Home error state, not an empty feed.
func TestHomeFeedReportsReadFailure(t *testing.T) {
	a, _ := newLibraryTestApp(t)
	a.libraryData.setLoadError("mumu could not read the saved playlists.")

	state := a.homeState()
	if state.Kind != ui.HomeFailed || state.Message == "" {
		t.Fatalf("state %+v, want the failed feed", state)
	}
}

// Toggling a favorite republishes Home with the new count.
func TestHomeFavoriteSync(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	ctx := context.Background()
	stored, found, err := store.Channels().FindByProvider(ctx, source.ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
	}
	if err := store.Recent().MarkPlayed(ctx, stored.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}
	a.loadLibrary(ctx)

	key := source.ID + ":my-news"
	a.toggleFavoriteKey(key)
	waitFor(t, f, "home favorite", func() bool {
		return a.shell.Home().State().FavoriteCount == 1
	})
	if got := homeFeed(a, f).Favorites; len(got) != 1 || got[0].Name != "My News" {
		t.Fatalf("favorites %+v, want My News on Home", got)
	}
}

// Real playback puts the channel on Home without a manual reload.
func TestRecordRecentUpdatesHome(t *testing.T) {
	a, f := newLibraryTestApp(t)
	source := seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})
	a.loadLibrary(context.Background())

	a.recordRecent(context.Background(), catalog.Channel{
		SourceID: source.ID, ProviderID: "my-news", Name: "My News",
	})

	waitFor(t, f, "home recent", func() bool {
		return a.shell.Home().State().RecentCount == 1
	})
	state := homeFeed(a, f)
	if state.Hero.Name != "My News" || len(state.Recent) != 1 {
		t.Fatalf("state %+v, want the played channel as hero and rail", state)
	}
}

// A resume target whose source was disabled cannot start playback.
func TestResumeRejectsDisabledSource(t *testing.T) {
	a, _ := newLibraryTestApp(t)
	seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	ctx := context.Background()
	sources, _ := store.Sources().List(ctx)
	stored, found, err := store.Channels().FindByProvider(ctx, sources[0].ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
	}
	if err := store.Recent().MarkPlayed(ctx, stored.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	a.loadLibrary(ctx)
	a.loadWatchResume(ctx)
	if err := store.Sources().SetEnabled(ctx, sources[0].ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	a.loadLibrary(ctx)

	a.resumeWatching()

	a.mu.Lock()
	started := a.controller != nil || a.engine != nil
	a.mu.Unlock()
	if started {
		t.Fatal("a disabled source started playback from the resume target")
	}
	if a.shell.ErrorMessage() == "" {
		t.Fatal("resume refusal was silent")
	}
}

// Two sources with equal channel names keep separate Home identities.
func TestHomeKeepsSourceProvenance(t *testing.T) {
	a, f := newLibraryTestApp(t)
	first := seedPlaylist(t, a, "https://example.com/first.m3u", "First", []catalog.Channel{
		testChannel("news", "News"),
	})
	second := seedPlaylist(t, a, "https://example.com/second.m3u", "Second", []catalog.Channel{
		testChannel("news", "News"),
	})

	store, _ := a.Store()
	ctx := context.Background()
	for _, source := range []struct{ id, name string }{{first.ID, "First"}, {second.ID, "Second"}} {
		stored, found, err := store.Channels().FindByProvider(ctx, source.id, "news")
		if err != nil || !found {
			t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
		}
		if err := store.Favorites().Add(ctx, stored.ID, time.Now().UTC()); err != nil {
			t.Fatalf("Add favorite: %v", err)
		}
	}
	a.loadLibrary(ctx)

	state := homeFeed(a, f)
	if len(state.Favorites) != 2 {
		t.Fatalf("favorites %+v, want both sources", state.Favorites)
	}
	if state.Favorites[0].Key == state.Favorites[1].Key {
		t.Fatalf("keys %q and %q are not source-scoped", state.Favorites[0].Key, state.Favorites[1].Key)
	}
	if state.Favorites[0].SourceName == state.Favorites[1].SourceName {
		t.Fatalf("source names %+v, want distinct provenance", state.Favorites)
	}
}

// A recent channel whose source was removed cannot be resumed.
func TestResumeAfterRemovedSource(t *testing.T) {
	a, _ := newLibraryTestApp(t)
	seedPlaylist(t, a, "https://example.com/list.m3u", "My list", []catalog.Channel{
		testChannel("my-news", "My News"),
	})

	store, _ := a.Store()
	ctx := context.Background()
	sources, _ := store.Sources().List(ctx)
	stored, found, err := store.Channels().FindByProvider(ctx, sources[0].ID, "my-news")
	if err != nil || !found {
		t.Fatalf("FindByProvider = %+v, %v, %v", stored, found, err)
	}
	if err := store.Recent().MarkPlayed(ctx, stored.ID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}
	a.loadLibrary(ctx)
	a.loadWatchResume(ctx)

	if err := store.Sources().Delete(ctx, sources[0].ID); err != nil {
		t.Fatalf("Delete source: %v", err)
	}
	a.loadLibrary(ctx)

	a.resumeWatching()

	a.mu.Lock()
	started := a.controller != nil || a.engine != nil
	a.mu.Unlock()
	if started {
		t.Fatal("a removed source started playback from the resume target")
	}
	if a.shell.ErrorMessage() == "" {
		t.Fatal("resume refusal was silent")
	}
	if got := a.homeState(); got.Hero.Key != "" {
		t.Fatalf("hero %+v, want no stale resume target", got.Hero)
	}
}
