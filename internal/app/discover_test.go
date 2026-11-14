package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/logos"
	"mumu/internal/sources"
	"mumu/internal/storage"
	"mumu/internal/ui"
)

// discoverTestApp opens a temporary library with a catalog service and a
// dispatcher that runs updates synchronously.
func discoverTestApp(t *testing.T) (*App, *fakeDispatcher) {
	t.Helper()

	a := New()
	a.openStoreAt(filepath.Join(t.TempDir(), "mumu.db"))
	a.openCatalog()
	a.openUserLibrary()
	t.Cleanup(a.closeStore)

	f := &fakeDispatcher{}
	a.dispatcher = f

	return a, f
}

// seedBuiltinCatalog caches one community channel for region ID.
func seedBuiltinCatalog(t *testing.T, a *App) {
	t.Helper()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v, want an open library", store, err)
	}

	// The saved country drives the cached catalog; nothing defaults to ID.
	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if err := service.SetRegion(context.Background(), "ID"); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	source, err := storage.NewSource(
		storage.SourceBuiltin,
		"https://iptv-org.github.io/iptv/countries/id.m3u",
		"Community list (ID)",
	)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	if _, err := store.Sources().Upsert(context.Background(), source); err != nil {
		t.Fatalf("Upsert source: %v", err)
	}

	batch, err := sources.ToStorageBatch([]catalog.Channel{{
		ProviderID: "tvri",
		Name:       "TVRI Nasional",
		Group:      "General",
		Country:    "ID",
		Streams: []catalog.Stream{{
			URL:  "https://stream.example/tvri.m3u8",
			HLS:  true,
			Live: true,
		}},
	}})
	if err != nil {
		t.Fatalf("ToStorageBatch: %v", err)
	}
	if _, err := store.Channels().ReplaceForSource(
		context.Background(),
		source.ID,
		batch,
		storage.RefreshMeta{RefreshedAt: time.Now(), ItemCount: 1},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}
}

// saveRegion stores a catalog country for a test that expects one.
func saveRegion(t *testing.T, a *App, region string) {
	t.Helper()

	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if err := service.SetRegion(context.Background(), region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
}

// failRefresh keeps the bridge offline; the message must never reach the UI.
func failRefresh(context.Context, *sources.Service, string) (catalog.Source, error) {
	return catalog.Source{}, errors.New("dial https://iptv-org.github.io: connection refused")
}

// Cached channels stay visible when the refresh fails, marked offline.
func TestDiscoverCachedCatalogFallsBackToOffline(t *testing.T) {
	a, _ := discoverTestApp(t)
	seedBuiltinCatalog(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	got := a.shell.Discover().State()
	if got.Kind != ui.DiscoverOffline {
		t.Fatalf("state kind %v, want DiscoverOffline", got.Kind)
	}
	if got.Region != "ID" {
		t.Fatalf("region %q, want ID", got.Region)
	}
	if len(got.Channels) != 1 || got.Channels[0].Name != "TVRI Nasional" {
		t.Fatalf("channels %+v, want the cached channel", got.Channels)
	}
	if !got.Channels[0].Live {
		t.Fatal("cached live channel lost its LIVE format")
	}
	if strings.Contains(got.Message, "http") || strings.Contains(got.Message, "dial") {
		t.Fatalf("message %q leaks network detail", got.Message)
	}
}

// With nothing cached, a failed refresh reports an error and keeps actions.
func TestDiscoverWithoutCacheReportsError(t *testing.T) {
	a, _ := discoverTestApp(t)
	saveRegion(t, a, "ID")
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	got := a.shell.Discover().State()
	if got.Kind != ui.DiscoverError {
		t.Fatalf("state kind %v, want DiscoverError", got.Kind)
	}
	if len(got.Channels) != 0 {
		t.Fatalf("channels %+v, want none", got.Channels)
	}
	if strings.Contains(got.Message, "http") || strings.Contains(got.Message, "dial") {
		t.Fatalf("message %q leaks network detail", got.Message)
	}
}

// discoverChannels keeps identity, metadata and the live format.
func TestDiscoverChannelsMapping(t *testing.T) {
	rows := discoverChannels([]catalog.Channel{
		{
			ProviderID: "news",
			Name:       "News One",
			Country:    "ID",
			Group:      "News",
			LogoURL:    "https://logos.example/news.png",
			Streams:    []catalog.Stream{{Live: true}},
		},
		{
			ProviderID: "clips",
			Name:       "Clips",
			Streams:    []catalog.Stream{{Live: false}},
		},
		{Name: "No identity"},
	})

	if len(rows) != 3 {
		t.Fatalf("rows %+v, want three", rows)
	}
	if rows[0].Key != "news" || rows[0].Category != "News" || !rows[0].Live {
		t.Fatalf("row %+v, want identity, category and live", rows[0])
	}
	if rows[1].Live {
		t.Fatalf("row %+v, want no LIVE for a non-live stream", rows[1])
	}
	if rows[2].Key != "row-2" {
		t.Fatalf("row key %q, want an index fallback", rows[2].Key)
	}
}

// Logo URLs are distinct and bounded to the first channels.
func TestDiscoverLogoURLsBudget(t *testing.T) {
	channels := make([]catalog.Channel, discoverLogoBudget+20)
	for i := range channels {
		channels[i].LogoURL = fmt.Sprintf("https://logos.example/%d.png", i)
		// A repeated URL must not use part of the budget twice.
		if i%10 == 1 {
			channels[i].LogoURL = "https://logos.example/0.png"
		}
	}

	urls := discoverLogoURLs(channels)
	if len(urls) != discoverLogoBudget {
		t.Fatalf("urls %d, want %d", len(urls), discoverLogoBudget)
	}

	seen := make(map[string]struct{}, len(urls))
	for _, url := range urls {
		if _, ok := seen[url]; ok {
			t.Fatalf("duplicate logo URL %q", url)
		}
		seen[url] = struct{}{}
	}

	if len(discoverLogoURLs(nil)) != 0 {
		t.Fatal("no channels should yield no logo URLs")
	}
}

// The bitmap cache is bounded and keeps the newest logos.
func TestLogoCacheLimit(t *testing.T) {
	cache := newDiscoverLogos()
	cache.limit = 2

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tint := logos.DominantColor(img)
	cache.add("a", img, tint)
	cache.add("b", img, tint)
	cache.add("c", img, tint)

	if len(cache.bitmaps) != 2 {
		t.Fatalf("cache holds %d bitmaps, want 2", len(cache.bitmaps))
	}
	if _, ok := cache.bitmaps["a"]; ok {
		t.Fatal("cache kept the oldest bitmap")
	}
	if _, ok := cache.bitmaps["c"]; !ok {
		t.Fatal("cache dropped the newest bitmap")
	}
}

// Provider IDs are scoped to a source; rows must not share a list key.
func TestDiscoverChannelsKeysAreSourceScoped(t *testing.T) {
	rows := discoverChannels([]catalog.Channel{
		{SourceID: "source-a", ProviderID: "news", Name: "News"},
		{SourceID: "source-b", ProviderID: "news", Name: "News"},
	})
	if rows[0].Key == rows[1].Key {
		t.Fatalf("rows share key %q", rows[0].Key)
	}
}

// seedUserSource caches one enabled user playlist channel.
func seedUserSource(t *testing.T, a *App) {
	t.Helper()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v, want an open library", store, err)
	}

	source, err := storage.NewSource(
		storage.SourceRemotePlaylist,
		"https://example.com/my-list.m3u",
		"My list",
	)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	if _, err := store.Sources().Upsert(context.Background(), source); err != nil {
		t.Fatalf("Upsert source: %v", err)
	}

	batch, err := sources.ToStorageBatch([]catalog.Channel{{
		ProviderID: "my-news",
		Name:       "My News",
		Streams: []catalog.Stream{{
			URL:  "https://stream.example/my-news.m3u8",
			Live: true,
		}},
	}})
	if err != nil {
		t.Fatalf("ToStorageBatch: %v", err)
	}
	if _, err := store.Channels().ReplaceForSource(
		context.Background(),
		source.ID,
		batch,
		storage.RefreshMeta{RefreshedAt: time.Now(), ItemCount: 1},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}
}

// User sources appear beside the community catalog, even when it is offline.
func TestDiscoverIncludesEnabledUserSources(t *testing.T) {
	a, _ := discoverTestApp(t)
	saveRegion(t, a, "ID")
	seedUserSource(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	got := a.shell.Discover().State()
	if got.Kind != ui.DiscoverOffline {
		t.Fatalf("state kind %v, want DiscoverOffline", got.Kind)
	}
	if len(got.Channels) != 1 || got.Channels[0].Name != "My News" {
		t.Fatalf("channels %+v, want the user channel", got.Channels)
	}
}

// Narrowing runs off the UI thread and republishes matching rows.
func TestDiscoverFiltersChannels(t *testing.T) {
	a, f := discoverTestApp(t)
	seedBuiltinCatalog(t, a)
	seedUserSource(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	before := f.updates
	a.filterDiscover(ui.DiscoverFilters{Query: "my news"})
	f.waitUpdates(t, before+1)

	got := a.shell.Discover().State()
	if len(got.Channels) != 1 || got.Channels[0].Name != "My News" {
		t.Fatalf("channels %+v, want only the matching user channel", got.Channels)
	}
	if got.Total != 1 {
		t.Fatalf("total %d, want 1", got.Total)
	}
	if got.Kind != ui.DiscoverOffline {
		t.Fatalf("state kind %v, want the catalog state kept", got.Kind)
	}
}

// A newer query replaces an older one; the stale result is never published.
func TestDiscoverFilterCancelsStaleQuery(t *testing.T) {
	a, f := discoverTestApp(t)
	seedBuiltinCatalog(t, a)
	seedUserSource(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	before := f.updates
	a.filterDiscover(ui.DiscoverFilters{Query: "tvri"})
	a.filterDiscover(ui.DiscoverFilters{Query: "my news"})
	f.waitUpdates(t, before+1)

	got := a.shell.Discover().State()
	if len(got.Channels) != 1 || got.Channels[0].Name != "My News" {
		t.Fatalf("channels %+v, want only the newest query result", got.Channels)
	}
}

// Search publishes one batch at a time and keeps Total counting every match.
func TestDiscoverResultCap(t *testing.T) {
	index := newDiscoverIndex()
	channels := make([]catalog.Channel, discoverResultLimit+25)
	for i := range channels {
		channels[i] = catalog.Channel{
			SourceID:   "source-a",
			ProviderID: fmt.Sprintf("provider-%d", i),
			Name:       fmt.Sprintf("Channel %d", i),
		}
	}
	index.setCatalog(channels, discoverFacets{}, discoverBase{
		kind:   ui.DiscoverReady,
		region: "ID",
	})

	filters := ui.DiscoverFilters{Query: "channel"}
	rows, total, truncated := index.view(filters)
	if len(rows) != discoverBatch {
		t.Fatalf("rows %d, want the first batch of %d", len(rows), discoverBatch)
	}
	if total != len(channels) || !truncated {
		t.Fatalf("total %d truncated %v, want %d and true", total, truncated, len(channels))
	}

	for len(rows) < discoverResultLimit {
		index.growLimit()
		rows, total, truncated = index.view(filters)
	}
	if len(rows) != discoverResultLimit {
		t.Fatalf("rows %d, want the %d row cap", len(rows), discoverResultLimit)
	}
	if total != len(channels) || !truncated {
		t.Fatalf("total %d truncated %v, want %d and true", total, truncated, len(channels))
	}
}

// Browsing loads batches until every match is reachable.
func TestDiscoverBrowseLoadsBatches(t *testing.T) {
	index := newDiscoverIndex()
	channels := make([]catalog.Channel, 45)
	for i := range channels {
		channels[i] = catalog.Channel{
			SourceID:   "source-a",
			ProviderID: fmt.Sprintf("provider-%d", i),
			Name:       fmt.Sprintf("Channel %d", i),
		}
	}
	index.setCatalog(channels, discoverFacets{}, discoverBase{
		kind:   ui.DiscoverReady,
		region: "ID",
	})

	rows, total, truncated := index.view(ui.DiscoverFilters{})
	if len(rows) != discoverBatch || total != 45 || !truncated {
		t.Fatalf("first batch rows %d total %d truncated %v, want %d of 45", len(rows), total, truncated, discoverBatch)
	}

	index.growLimit()
	rows, total, truncated = index.view(ui.DiscoverFilters{})
	if len(rows) != 45 || total != 45 || truncated {
		t.Fatalf("grown batch rows %d total %d truncated %v, want all 45", len(rows), total, truncated)
	}
	if rows[0].Key == rows[44].Key {
		t.Fatalf("rows repeat key %q", rows[0].Key)
	}
}

// Category counts are real match counts and ignore the selected category.
func TestDiscoverCategoryCounts(t *testing.T) {
	index := newDiscoverIndex()
	index.setCatalog([]catalog.Channel{
		{SourceID: "s", ProviderID: "a", Name: "A", Group: "News"},
		{SourceID: "s", ProviderID: "b", Name: "B", Group: "News"},
		{SourceID: "s", ProviderID: "c", Name: "C", Group: "Sports"},
	}, discoverFacets{}, discoverBase{kind: ui.DiscoverReady, region: "ID"})

	counts := index.categoryCounts(ui.DiscoverFilters{Category: "News"})
	if len(counts) != 2 {
		t.Fatalf("counts %+v, want two categories", counts)
	}
	if counts[0].Name != "News" || counts[0].Count != 2 {
		t.Fatalf("counts %+v, want News first with 2", counts)
	}
}

// A fresh install shows no country and never fetches a country playlist.
func TestDiscoverStartsWithoutCountry(t *testing.T) {
	a, f := discoverTestApp(t)
	refreshed := 0
	a.refreshBuiltin = func(context.Context, *sources.Service, string) (catalog.Source, error) {
		refreshed++

		return catalog.Source{}, errors.New("refresh must not run")
	}

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)
	f.waitUpdates(t, 1)

	if refreshed != 0 {
		t.Fatalf("country playlist fetches %d, want 0", refreshed)
	}
	got := a.shell.Discover().State()
	if got.Kind != ui.DiscoverNoCountry {
		t.Fatalf("state kind %v, want DiscoverNoCountry", got.Kind)
	}
	if got.Region != "" || len(got.Channels) != 0 {
		t.Fatalf("state %+v, want no region and no channels", got)
	}
}

// Imported sources stay usable before any catalog country is chosen.
func TestDiscoverImportsWithoutCountry(t *testing.T) {
	a, f := discoverTestApp(t)
	seedUserSource(t, a)
	region := a.shell.Discover().Region()
	if region != "" {
		t.Fatalf("region %q, want none", region)
	}

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)
	f.waitUpdates(t, 1)

	got := a.shell.Discover().State()
	if got.Kind != ui.DiscoverReady {
		t.Fatalf("state kind %v, want DiscoverReady for the import", got.Kind)
	}
	if got.Region != "" || len(got.Channels) != 1 || got.Channels[0].Name != "My News" {
		t.Fatalf("state %+v, want the imported channel without a country", got)
	}
}

// A saved country is kept on restart and drives the cached catalog.
func TestDiscoverKeepsSavedCountry(t *testing.T) {
	a, _ := discoverTestApp(t)
	seedBuiltinCatalog(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	got := a.shell.Discover().State()
	if got.Region != "ID" {
		t.Fatalf("region %q, want the saved ID", got.Region)
	}
	if got.Kind != ui.DiscoverOffline {
		t.Fatalf("state kind %v, want DiscoverOffline with cached rows", got.Kind)
	}
}

// A saved country the catalog no longer lists is reported, never replaced.
func TestDiscoverDropsUnknownSavedCountry(t *testing.T) {
	a, f := discoverTestApp(t)
	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if err := service.SetRegion(context.Background(), "ZZ"); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)
	f.waitUpdates(t, 1)

	countries := make([]ui.DiscoverCountry, 0, 12)
	for i := range 12 {
		countries = append(countries, ui.DiscoverCountry{Code: fmt.Sprintf("C%d", i), Name: fmt.Sprintf("Country %d", i)})
	}
	a.dropUnknownDiscoverRegion(countries)
	f.waitUpdates(t, 2)

	got := a.shell.Discover().State()
	if got.Kind != ui.DiscoverNoCountry || got.Region != "" {
		t.Fatalf("state %+v, want the no-country state", got)
	}
	if !strings.Contains(got.Message, "no longer in the catalog") {
		t.Fatalf("message %q, want an explicit saved-country notice", got.Message)
	}
}

// Republishing the index needs no network work.
func TestDiscoverRepublishWithoutNetwork(t *testing.T) {
	a, _ := discoverTestApp(t)
	refreshed := 0
	a.refreshBuiltin = func(context.Context, *sources.Service, string) (catalog.Source, error) {
		refreshed++

		return catalog.Source{}, errors.New("a republish must not refetch")
	}

	channels := make([]catalog.Channel, 45)
	for i := range channels {
		channels[i] = catalog.Channel{
			SourceID:   "source-a",
			ProviderID: fmt.Sprintf("provider-%d", i),
			Name:       fmt.Sprintf("Channel %d", i),
		}
	}
	a.discoverIndex.setCatalog(channels, discoverFacets{names: map[string]string{}}, discoverBase{
		kind:   ui.DiscoverReady,
		region: "ID",
	})
	gen := a.discover.generation()

	a.publishView(discoverBase{kind: ui.DiscoverReady, region: "ID"},
		func() bool { return a.discover.current(gen) })

	got := a.shell.Discover().State()
	if got.Total != 45 || len(got.Channels) != discoverBatch || !got.HasMore {
		t.Fatalf("state total %d rows %d hasMore %v, want the first batch of 45", got.Total, len(got.Channels), got.HasMore)
	}
	if refreshed != 0 {
		t.Fatalf("network refreshes %d, want 0", refreshed)
	}
}

// Choosing a country persists it before the catalog reloads.
func TestDiscoverRegionChoicePersists(t *testing.T) {
	a, f := discoverTestApp(t)
	refreshed := 0
	a.refreshBuiltin = func(context.Context, *sources.Service, string) (catalog.Source, error) {
		refreshed++

		return catalog.Source{}, errors.New("offline")
	}

	before := f.updates
	a.setDiscoverRegion("my")
	f.waitUpdates(t, before+1)

	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	region, err := service.Region(context.Background())
	if err != nil || region != "MY" {
		t.Fatalf("Region = %q, %v, want MY", region, err)
	}
	if got := a.shell.Discover().Region(); got != "MY" {
		t.Fatalf("page region %q, want MY", got)
	}
	if refreshed > 1 {
		t.Fatalf("refresh runs %d, want at most one country fetch", refreshed)
	}
}

// Facets carry distinct labels and the row attribution.
func TestDiscoverFacetsAndAttribution(t *testing.T) {
	a, _ := discoverTestApp(t)
	seedUserSource(t, a)

	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	ctx := context.Background()
	users, err := service.UserChannels(ctx)
	if err != nil {
		t.Fatalf("UserChannels: %v", err)
	}

	facets := discoverFacetsFor(ctx, service, users)
	if facets.names[users[0].SourceID] != "My list" {
		t.Fatalf("names %+v, want the source name", facets.names)
	}

	rows, _, _ := newDiscoverIndexWith(users, facets).view(ui.DiscoverFilters{})
	if len(rows) != 1 || rows[0].SourceName != "My list" {
		t.Fatalf("rows %+v, want source attribution", rows)
	}
}

// Selecting a row opens Watch with that channel.
func TestDiscoverSelectionOpensWatch(t *testing.T) {
	a, f := discoverTestApp(t)
	seedBuiltinCatalog(t, a)
	a.refreshBuiltin = failRefresh

	gen, ctx := a.discover.begin(a.ctx)
	a.loadDiscover(ctx, gen)

	got := a.shell.Discover().State()
	if len(got.Channels) == 0 {
		t.Fatal("no rows to select")
	}

	a.selectDiscoverChannel(got.Channels[0].Key)

	// The controller notifier publishes on its own goroutine; read under the
	// dispatcher lock so the check cannot race the next frame.
	var channel string
	f.read(func() { channel = a.shell.Watch().State().Channel })
	if channel != "TVRI Nasional" {
		t.Fatalf("watch channel %q, want TVRI Nasional", channel)
	}
	if a.shell.Destination() != "/watch" {
		t.Fatalf("destination %q, want Watch", a.shell.Destination())
	}
}

// newDiscoverIndexWith indexes channels and their facets for filter tests.
func newDiscoverIndexWith(channels []catalog.Channel, facets discoverFacets) *discoverIndex {
	index := newDiscoverIndex()
	index.setCatalog(channels, facets, discoverBase{kind: ui.DiscoverReady, region: "ID"})

	return index
}
