package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/player"
	"mumu/internal/sources"
	"mumu/internal/storage"
	"mumu/internal/ui"
)

// drawerTestSource stores one built-in source with channels and returns its ID.
func drawerTestSource(
	t *testing.T,
	a *App,
	region, name string,
	channels []catalog.Channel,
) string {
	t.Helper()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v, want an open library", store, err)
	}

	source, err := storage.NewSource(
		storage.SourceBuiltin,
		"https://iptv-org.github.io/iptv/countries/"+strings.ToLower(region)+".m3u",
		name,
	)
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

	return source.ID
}

// loadIndonesiaCatalog publishes the cached Indonesian catalog through the
// normal Discover path without touching the network.
func loadIndonesiaCatalog(t *testing.T, a *App) {
	t.Helper()

	seedBuiltinCatalog(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)
}

// watchDrawerRows copies the published drawer under the dispatcher lock.
func watchDrawerRows(a *App, f *fakeDispatcher) []ui.WatchChannel {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]ui.WatchChannel(nil), a.shell.Watch().State().Channels...)
}

// The drawer follows the active channel's source, not the last Discover
// country that was browsed.
func TestWatchDrawerFollowsActiveChannelSource(t *testing.T) {
	a, f := discoverTestApp(t)
	loadIndonesiaCatalog(t, a)

	// With no channel active the drawer still offers the Discover catalog.
	idle := watchDrawerRows(a, f)
	if len(idle) != 1 || idle[0].Name != "TVRI Nasional" {
		t.Fatalf("idle drawer %+v, want the Discover catalog", idle)
	}

	sgID := drawerTestSource(t, a, "SG", "Community list (SG)", []catalog.Channel{
		testChannel("channel-5", "Channel 5"),
		testChannel("channel-8", "Channel 8"),
	})

	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }

	a.playChannel(sourceChannel(sgID, "channel-5", "Channel 5"), "Community list (SG)")

	waitFor(t, f, "the Singapore drawer", func() bool {
		return len(a.shell.Watch().State().Channels) == 2
	})

	channels := watchDrawerRows(a, f)
	names := map[string]bool{}
	for _, row := range channels {
		names[row.Name] = true
		if row.Name == "TVRI Nasional" {
			t.Fatalf("drawer kept the Indonesian channel: %+v", channels)
		}
	}
	if !names["Channel 5"] || !names["Channel 8"] {
		t.Fatalf("drawer %+v, want the Singapore source's channels", channels)
	}
}

// A drawer row loaded from storage still resolves to the channel it shows.
func TestWatchDrawerStoredRowSelection(t *testing.T) {
	a, f := discoverTestApp(t)
	loadIndonesiaCatalog(t, a)

	sgID := drawerTestSource(t, a, "SG", "Community list (SG)", []catalog.Channel{
		testChannel("channel-5", "Channel 5"),
		testChannel("channel-8", "Channel 8"),
	})

	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }

	a.playChannel(sourceChannel(sgID, "channel-5", "Channel 5"), "Community list (SG)")
	waitFor(t, f, "the Singapore drawer", func() bool {
		return len(a.shell.Watch().State().Channels) == 2
	})
	waitForLoads(t, engine, 1)

	key := ""
	for _, row := range watchDrawerRows(a, f) {
		if row.Name == "Channel 8" {
			key = row.Key
		}
	}
	if key == "" {
		t.Fatal("drawer has no row for Channel 8")
	}

	// The row is not in the Discover index, so selection must use the drawer's
	// own lookup.
	a.selectWatchChannel(key)

	a.mu.Lock()
	channel := a.media.channel
	a.mu.Unlock()
	if channel.ProviderID != "channel-8" || channel.SourceID != sgID {
		t.Fatalf("selected channel %+v, want Channel 8 from the stored source", channel)
	}

	waitForLoads(t, engine, 2)
	if got := engine.loadSource().URL; !strings.HasSuffix(got, "/channel-8.m3u8") {
		t.Fatalf("loaded %q, want the selected row's stream", got)
	}
}

// A Discover publish must not replace the active channel's drawer.
func TestDiscoverPublishKeepsActiveChannelDrawer(t *testing.T) {
	a, f := discoverTestApp(t)
	loadIndonesiaCatalog(t, a)

	sgID := drawerTestSource(t, a, "SG", "Community list (SG)", []catalog.Channel{
		testChannel("channel-5", "Channel 5"),
	})

	engine := newFakeEngine(t)
	a.newEngine = func(string, int) (player.Engine, error) { return engine, nil }

	a.playChannel(sourceChannel(sgID, "channel-5", "Channel 5"), "Community list (SG)")
	waitFor(t, f, "the Singapore drawer", func() bool {
		return len(a.shell.Watch().State().Channels) == 1
	})

	// A country reload or filter republishes the Indonesian catalog while the
	// Singapore channel keeps playing.
	a.publishView(a.discoverIndex.base(), nil)

	channels := watchDrawerRows(a, f)
	if len(channels) != 1 || channels[0].Name != "Channel 5" {
		t.Fatalf("drawer %+v, want the active Singapore channel", channels)
	}
}

// A slow load for an older source cannot replace a newer source's rows.
func TestWatchDrawerStaleLoadIsDiscarded(t *testing.T) {
	a, _ := discoverTestApp(t)

	firstID := drawerTestSource(t, a, "SG", "Community list (SG)", []catalog.Channel{
		testChannel("channel-5", "Channel 5"),
	})
	secondID := drawerTestSource(t, a, "MY", "Community list (MY)", []catalog.Channel{
		testChannel("channel-1", "Channel 1"),
	})

	a.prepareChannelDrawer(sourceChannel(firstID, "channel-5", "Channel 5"), "Community list (SG)")
	waitSnapshot(t, a, "the first source drawer", func() bool {
		return len(a.media.drawer) == 1 && a.media.drawer[0].Name == "Channel 5"
	})

	a.mu.Lock()
	staleGen := a.media.drawerGen
	a.mu.Unlock()

	a.prepareChannelDrawer(sourceChannel(secondID, "channel-1", "Channel 1"), "Community list (MY)")
	waitSnapshot(t, a, "the second source drawer", func() bool {
		return len(a.media.drawer) == 1 && a.media.drawer[0].Name == "Channel 1"
	})

	staleRows, staleEntries := drawerRows(
		[]catalog.Channel{sourceChannel(firstID, "channel-5", "Channel 5")},
		"Community list (SG)",
		discoverResultLimit,
	)
	if a.applyChannelDrawer(staleGen, firstID, staleRows, staleEntries) {
		t.Fatal("a stale drawer load replaced the newer source")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.media.drawer) != 1 || a.media.drawer[0].Name != "Channel 1" {
		t.Fatalf("drawer %+v, want the newer source's row", a.media.drawer)
	}
}
