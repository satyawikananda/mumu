package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func mustSource(t *testing.T, store *Store, kind SourceKind, location string) Source {
	t.Helper()

	source, err := NewSource(kind, location, "Test source")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	stored, err := store.Sources().Upsert(t.Context(), source)
	if err != nil {
		t.Fatalf("Upsert source: %v", err)
	}

	return stored
}

func mustChannel(t *testing.T, store *Store, source Source, channel Channel) Channel {
	t.Helper()

	stored, err := store.Channels().Upsert(t.Context(), source.ID, channel)
	if err != nil {
		t.Fatalf("Upsert channel: %v", err)
	}

	return stored
}

func TestSourceIdentityStableAcrossTokenRotation(t *testing.T) {
	store := openTestStore(t)

	first, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u?token=one", "List")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	stored, err := store.Sources().Upsert(t.Context(), first)
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u?token=two", "Renamed")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	if second.ID != stored.ID {
		t.Fatalf("source ID changed across token rotation: %s vs %s", second.ID, stored.ID)
	}
	if _, err := store.Sources().Upsert(t.Context(), second); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	sources, err := store.Sources().List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("source count %d, want 1", len(sources))
	}
	if sources[0].Location != "https://example.com/list.m3u?token=two" {
		t.Fatalf("location %q, want rotated URL", sources[0].Location)
	}
	if sources[0].DisplayName != "Renamed" {
		t.Fatalf("display name %q, want Renamed", sources[0].DisplayName)
	}
}

func TestSourceRejectsCredentials(t *testing.T) {
	_, err := NewSource(SourceRemotePlaylist, "https://user:pass@example.com/list.m3u", "")
	if !errors.Is(err, ErrCredentialsUnsupported) {
		t.Fatalf("error %v, want ErrCredentialsUnsupported", err)
	}
}

func TestDuplicateNamesAcrossSources(t *testing.T) {
	store := openTestStore(t)

	first := mustSource(t, store, SourceRemotePlaylist, "https://a.example/list.m3u")
	second := mustSource(t, store, SourceRemotePlaylist, "https://b.example/list.m3u")

	firstChannel := mustChannel(t, store, first, Channel{
		Name:      "News",
		StreamURL: "https://stream.example/live.m3u8",
		Live:      true,
	})
	secondChannel := mustChannel(t, store, second, Channel{
		Name:      "News",
		StreamURL: "https://stream.example/live.m3u8",
		Live:      true,
	})

	if firstChannel.ID == secondChannel.ID {
		t.Fatal("same name and URL in different sources collapsed into one row")
	}

	channels, err := store.Channels().ListAll(t.Context(), 0)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("channel count %d, want 2", len(channels))
	}
}

func TestChannelUpsertKeepsIdentity(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	first := mustChannel(t, store, source, Channel{
		ProviderID: "tvg-1",
		Name:       "Old name",
		StreamURL:  "https://stream.example/one.m3u8",
	})
	second := mustChannel(t, store, source, Channel{
		ProviderID: "tvg-1",
		Name:       "New name",
		StreamURL:  "https://stream.example/two.m3u8",
	})

	if first.ID != second.ID {
		t.Fatalf("channel ID changed: %d vs %d", first.ID, second.ID)
	}
	channels, err := store.Channels().List(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(channels) != 1 || channels[0].Name != "New name" {
		t.Fatalf("channels = %+v, want one updated channel", channels)
	}
}

func TestChannelBatchRollsBack(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	err := store.Channels().UpsertMany(t.Context(), source.ID, []Channel{
		{Name: "Valid", StreamURL: "https://stream.example/one.m3u8"},
		{Name: "", StreamURL: "https://stream.example/two.m3u8"},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}

	channels, err := store.Channels().List(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(channels) != 0 {
		t.Fatalf("channels %d after failed batch, want 0", len(channels))
	}
}

func TestChannelBatchRequiresExistingSource(t *testing.T) {
	store := openTestStore(t)

	err := store.Channels().UpsertMany(t.Context(), "missing", []Channel{
		{Name: "Channel", StreamURL: "https://stream.example/live.m3u8"},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
}

func TestFavorites(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")
	channel := mustChannel(t, store, source, Channel{
		Name:      "News",
		StreamURL: "https://stream.example/live.m3u8",
	})

	if err := store.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("Add: %v", err)
	}
	has, err := store.Favorites().Has(t.Context(), channel.ID)
	if err != nil || !has {
		t.Fatalf("Has = %v, %v, want true", has, err)
	}

	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != channel.ID {
		t.Fatalf("favorites = %+v, want one channel", favorites)
	}

	if err := store.Favorites().Remove(t.Context(), channel.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	has, err = store.Favorites().Has(t.Context(), channel.ID)
	if err != nil || has {
		t.Fatalf("Has = %v, %v, want false", has, err)
	}

	if err := store.Favorites().Add(t.Context(), 9999, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Add missing error %v, want ErrNotFound", err)
	}
}

func TestRecentPlaybackAndPrune(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	base := time.Now().Add(-time.Hour)
	for i := range RecentLimit + 5 {
		channel := mustChannel(t, store, source, Channel{
			Name:      fmt.Sprintf("Channel %03d", i),
			StreamURL: fmt.Sprintf("https://stream.example/%03d.m3u8", i),
		})
		if err := store.Recent().MarkPlayed(t.Context(), channel.ID, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("MarkPlayed: %v", err)
		}
	}

	recent, err := store.Recent().List(t.Context(), 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(recent) != RecentLimit {
		t.Fatalf("recent count %d, want %d", len(recent), RecentLimit)
	}
	if recent[0].Channel.Name != fmt.Sprintf("Channel %03d", RecentLimit+4) {
		t.Fatalf("first recent %q, want newest", recent[0].Channel.Name)
	}

	last, ok, err := store.Recent().Last(t.Context())
	if err != nil || !ok {
		t.Fatalf("Last = %+v, %v, %v", last, ok, err)
	}
	if last.Channel.ID != recent[0].Channel.ID {
		t.Fatalf("Last ID %d, want %d", last.Channel.ID, recent[0].Channel.ID)
	}

	// Playing the same channel again increments its count.
	if err := store.Recent().MarkPlayed(t.Context(), last.Channel.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed again: %v", err)
	}
	latest, ok, err := store.Recent().Last(t.Context())
	if err != nil || !ok {
		t.Fatalf("Last after replay = %+v, %v, %v", latest, ok, err)
	}
	if latest.PlayCount < 2 {
		t.Fatalf("play count %d, want at least 2", latest.PlayCount)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := t.Context()

	if err := store.Settings().Set(ctx, "region", "ID"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Settings().Set(ctx, "theme", "dark"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	value, ok, err := store.Settings().Get(ctx, "region")
	if err != nil || !ok || value != "ID" {
		t.Fatalf("Get = %q, %v, %v, want ID", value, ok, err)
	}

	all, err := store.Settings().All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 2 || all["theme"] != "dark" {
		t.Fatalf("All = %+v, want two settings", all)
	}

	if err := store.Settings().Delete(ctx, "region"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := store.Settings().Get(ctx, "region"); err != nil || ok {
		t.Fatalf("Get deleted = %v, %v, want false", ok, err)
	}

	if err := store.Settings().Set(ctx, " ", "value"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Set empty key error %v, want ErrInvalid", err)
	}
}

func TestSourceUpsertPreservesEnabled(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	if err := store.Sources().SetEnabled(t.Context(), source.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	updated, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u", "Renamed")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	stored, err := store.Sources().Upsert(t.Context(), updated)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if stored.Enabled {
		t.Fatal("re-import re-enabled a disabled source")
	}
}

func TestSourceRefreshMetadata(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	refreshedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
	if err := store.Sources().MarkRefreshed(t.Context(), source.ID, refreshedAt, `"etag"`, "offline"); err != nil {
		t.Fatalf("MarkRefreshed: %v", err)
	}

	stored, ok, err := store.Sources().Get(t.Context(), source.ID)
	if err != nil || !ok {
		t.Fatalf("Get = %v, %v", ok, err)
	}
	if !stored.LastRefreshAt.Equal(refreshedAt) {
		t.Fatalf("LastRefreshAt %v, want %v", stored.LastRefreshAt, refreshedAt)
	}
	if stored.RefreshETag != `"etag"` || stored.RefreshError != "offline" {
		t.Fatalf("refresh metadata = %q %q", stored.RefreshETag, stored.RefreshError)
	}

	if err := store.Sources().SetEnabled(t.Context(), source.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	stored, _, err = store.Sources().Get(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Enabled {
		t.Fatal("source still enabled")
	}
}

func TestSourceDeleteCascades(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")
	channel := mustChannel(t, store, source, Channel{
		Name:      "News",
		StreamURL: "https://stream.example/live.m3u8",
	})
	if err := store.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := store.Recent().MarkPlayed(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}

	if err := store.Sources().Delete(t.Context(), source.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	channels, err := store.Channels().List(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("List channels: %v", err)
	}
	favorites, err := store.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	recent, err := store.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	if len(channels) != 0 || len(favorites) != 0 || len(recent) != 0 {
		t.Fatalf("cascade left data: channels=%d favorites=%d recent=%d", len(channels), len(favorites), len(recent))
	}
}

func TestRestartPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mumu.db")

	first, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	source := mustSource(t, first, SourceRemotePlaylist, "https://example.com/list.m3u")
	channel := mustChannel(t, first, source, Channel{
		Name:      "News",
		StreamURL: "https://stream.example/live.m3u8",
	})
	if err := first.Favorites().Add(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("Add favorite: %v", err)
	}
	if err := first.Recent().MarkPlayed(t.Context(), channel.ID, time.Now()); err != nil {
		t.Fatalf("MarkPlayed: %v", err)
	}
	if err := first.Settings().Set(t.Context(), "region", "ID"); err != nil {
		t.Fatalf("Set setting: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = second.Close() }()

	sources, err := second.Sources().List(t.Context())
	if err != nil {
		t.Fatalf("List sources: %v", err)
	}
	favorites, err := second.Favorites().List(t.Context())
	if err != nil {
		t.Fatalf("List favorites: %v", err)
	}
	recent, err := second.Recent().List(t.Context(), 10)
	if err != nil {
		t.Fatalf("List recent: %v", err)
	}
	region, ok, err := second.Settings().Get(t.Context(), "region")
	if err != nil {
		t.Fatalf("Get setting: %v", err)
	}
	if len(sources) != 1 || len(favorites) != 1 || len(recent) != 1 || !ok || region != "ID" {
		t.Fatalf("restart lost data: sources=%d favorites=%d recent=%d region=%q", len(sources), len(favorites), len(recent), region)
	}
}

func TestConcurrentSettings(t *testing.T) {
	store := openTestStore(t)

	var group sync.WaitGroup
	for i := range 20 {
		group.Go(func() {
			key := fmt.Sprintf("key-%02d", i)
			if err := store.Settings().Set(t.Context(), key, "value"); err != nil {
				t.Errorf("Set %s: %v", key, err)

				return
			}
			if _, ok, err := store.Settings().Get(t.Context(), key); err != nil || !ok {
				t.Errorf("Get %s = %v, %v", key, ok, err)
			}
		})
	}
	group.Wait()

	all, err := store.Settings().All(t.Context())
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 20 {
		t.Fatalf("settings %d, want 20", len(all))
	}
}
