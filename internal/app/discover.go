package app

import (
	"cmp"
	"context"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"mumu/internal/catalog"
	"mumu/internal/logos"
	"mumu/internal/netguard"
	"mumu/internal/sources"
	"mumu/internal/ui"

	myui "github.com/egoist/mygo/ui"
)

const (
	// discoverLogoBudget bounds the logos fetched after a catalog publish.
	discoverLogoBudget = 120
	// discoverLogoWorkers bounds concurrent logo downloads.
	discoverLogoWorkers = 4
	// discoverBitmapLimit bounds the bitmaps kept for the channel list.
	discoverBitmapLimit = 256
	// discoverResultLimit caps the rows published for one search.
	discoverResultLimit = 2000
	// discoverFilterDelay debounces typing before the catalog is filtered.
	discoverFilterDelay = 150 * time.Millisecond
	// discoverCategoryLimit bounds the category filter options.
	discoverCategoryLimit = 200
	// discoverBatch is how many rows one infinite-scroll step adds.
	discoverBatch = 40
)

// catalogRefresher fetches one region's community catalog. Tests replace it
// to keep the bridge offline.
type catalogRefresher func(context.Context, *sources.Service, string) (catalog.Source, error)

// serviceRefreshBuiltin refreshes through the catalog service.
func serviceRefreshBuiltin(
	ctx context.Context,
	service *sources.Service,
	region string,
) (catalog.Source, error) {
	return service.RefreshBuiltin(ctx, region)
}

// discoverFacets are the category names and row attributions for a catalog.
type discoverFacets struct {
	categories []string
	names      map[string]string
}

// discoverIndex holds the whole catalog for the current region and narrows it
// off the UI thread. The app publishes the filtered rows; the page owns only
// the input values it shows.
type discoverIndex struct {
	mu         sync.Mutex
	channels   []catalog.Channel
	keys       map[string]catalog.Channel
	names      map[string]string
	categories []string
	baseKind   ui.DiscoverKind
	baseRegion string
	baseMsg    string
	filters    ui.DiscoverFilters
	limit      int
}

// newDiscoverIndex returns an index with no catalog.
func newDiscoverIndex() *discoverIndex {
	return &discoverIndex{
		keys:  map[string]catalog.Channel{},
		names: map[string]string{},
		limit: discoverBatch,
	}
}

// base is the catalog state the filtered rows belong to.
type discoverBase struct {
	kind    ui.DiscoverKind
	region  string
	message string
}

// setCatalog replaces the indexed catalog and prunes stale filter values.
func (i *discoverIndex) setCatalog(
	channels []catalog.Channel,
	facets discoverFacets,
	base discoverBase,
) ui.DiscoverFilters {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.channels = channels
	i.names = facets.names
	i.categories = facets.categories
	i.baseKind = base.kind
	i.baseRegion = base.region
	i.baseMsg = base.message
	i.limit = discoverBatch

	i.keys = make(map[string]catalog.Channel, len(channels))
	for position, channel := range channels {
		i.keys[discoverKey(channel, position)] = channel
	}

	if i.filters.Category != "" && !slices.Contains(facets.categories, i.filters.Category) {
		i.filters.Category = ""
	}

	return i.filters
}

// setFilters remembers the page's narrowing for refreshes. A new narrowing
// always starts at the first page.
func (i *discoverIndex) setFilters(filters ui.DiscoverFilters) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.filters = filters
	i.limit = discoverBatch
}

// currentFilters returns the narrowing the page last reported.
func (i *discoverIndex) currentFilters() ui.DiscoverFilters {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.filters
}

// base returns the state the filtered rows belong to.
func (i *discoverIndex) base() discoverBase {
	i.mu.Lock()
	defer i.mu.Unlock()

	return discoverBase{kind: i.baseKind, region: i.baseRegion, message: i.baseMsg}
}

// drawerChannels maps the indexed catalog to the fallback Watch drawer rows
// offered while no channel is active.
func (i *discoverIndex) drawerChannels(limit int) []ui.WatchChannel {
	i.mu.Lock()
	channels := i.channels
	names := i.names
	i.mu.Unlock()

	count := min(len(channels), limit)
	rows := make([]ui.WatchChannel, 0, count)
	for position := 0; position < count; position++ {
		channel := channels[position]
		row, _ := drawerRow(channel, position, names[channel.SourceID])
		rows = append(rows, row)
	}

	return rows
}

// drawerForSource maps the indexed channels of one source to drawer rows and
// their row lookup. It reports whether the index holds any channel of that
// source, so the caller can fall back to storage when it does not.
func (i *discoverIndex) drawerForSource(
	sourceID string,
	limit int,
) ([]ui.WatchChannel, map[string]drawerEntry, bool) {
	i.mu.Lock()
	channels := i.channels
	names := i.names
	i.mu.Unlock()

	sourceName := names[sourceID]
	rows := make([]ui.WatchChannel, 0, min(len(channels), limit))
	entries := make(map[string]drawerEntry)
	for position, channel := range channels {
		if channel.SourceID != sourceID {
			continue
		}

		row, entry := drawerRow(channel, position, sourceName)
		rows = append(rows, row)
		entries[row.Key] = entry
		if len(rows) >= limit {
			break
		}
	}

	return rows, entries, len(rows) > 0
}

// sourceName returns a source's display name, or an empty string.
func (i *discoverIndex) sourceName(id string) string {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.names[id]
}

// channel resolves one published row key back to its catalog channel.
func (i *discoverIndex) channel(key string) (catalog.Channel, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	channel, ok := i.keys[key]

	return channel, ok
}

// view returns the loaded rows, the total match count, and whether more rows
// exist than were loaded. It runs off the UI thread.
func (i *discoverIndex) view(filters ui.DiscoverFilters) ([]ui.DiscoverChannel, int, bool) {
	i.mu.Lock()
	channels := i.channels
	names := i.names
	limit := i.limit
	i.mu.Unlock()

	match := catalogFilter(filters)
	limit = min(max(limit, discoverBatch), discoverResultLimit)
	rows, total := browseChannels(channels, names, match, limit)

	return rows, total, total > len(rows)
}

// growLimit adds one batch to the browse limit, up to the safety cap.
func (i *discoverIndex) growLimit() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.limit = min(i.limit+discoverBatch, discoverResultLimit)
}

// categoryCounts counts real matches per category for the current narrowing.
// The selected category is ignored, so its own count stays truthful.
func (i *discoverIndex) categoryCounts(filters ui.DiscoverFilters) []ui.DiscoverCategory {
	i.mu.Lock()
	channels := i.channels
	i.mu.Unlock()

	match := catalogFilter(filters)
	match.Category = ""

	counts := map[string]int{}
	for _, channel := range channels {
		if !match.Match(channel) {
			continue
		}
		group := strings.TrimSpace(channel.Group)
		if group != "" {
			counts[group]++
		}
	}

	categories := make([]ui.DiscoverCategory, 0, min(len(counts), discoverCategoryLimit))
	for name, count := range counts {
		categories = append(categories, ui.DiscoverCategory{Name: name, Count: count})
	}
	slices.SortFunc(categories, func(a, b ui.DiscoverCategory) int {
		if a.Count != b.Count {
			return cmp.Compare(b.Count, a.Count)
		}

		return strings.Compare(a.Name, b.Name)
	})
	if len(categories) > discoverCategoryLimit {
		categories = categories[:discoverCategoryLimit]
	}

	return categories
}

// browseChannels collects the first limit matches in one pass and counts all.
func browseChannels(
	channels []catalog.Channel,
	names map[string]string,
	match catalog.Filter,
	limit int,
) ([]ui.DiscoverChannel, int) {
	rows := make([]ui.DiscoverChannel, 0, min(len(channels), limit))
	total := 0
	for position, channel := range channels {
		if !match.Match(channel) {
			continue
		}
		if len(rows) < limit {
			rows = append(rows, discoverRow(channel, position, names))
		}
		total++
	}

	return rows, total
}

// catalogFilter maps the page's narrowing to the domain filter.
func catalogFilter(filters ui.DiscoverFilters) catalog.Filter {
	return catalog.Filter{
		Query:    filters.Query,
		Category: filters.Category,
	}
}

// discoverKey is a row's stable identity across sources.
func discoverKey(channel catalog.Channel, position int) string {
	key := channel.ProviderID
	if key == "" {
		return fmt.Sprintf("row-%d", position)
	}
	if channel.SourceID != "" {
		return channel.SourceID + ":" + key
	}

	return key
}

// discoverFacetsFor builds the category names and row attributions.
func discoverFacetsFor(
	ctx context.Context,
	service *sources.Service,
	channels []catalog.Channel,
) discoverFacets {
	names := map[string]string{}
	if service != nil {
		// Display names come from storage; a failure only costs attribution.
		if stored, err := service.List(ctx); err == nil {
			for _, source := range stored {
				names[source.ID] = strings.TrimSpace(source.DisplayName)
			}
		}
	}

	categorySet := map[string]struct{}{}
	for _, channel := range channels {
		if category := strings.TrimSpace(channel.Group); category != "" {
			categorySet[category] = struct{}{}
		}
	}

	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	if len(categories) > discoverCategoryLimit {
		categories = categories[:discoverCategoryLimit]
	}

	return discoverFacets{categories: categories, names: names}
}

// logoCache keeps channel logo bitmaps made on the UI thread, bounded by URL.
type logoCache struct {
	fetcher *logos.Fetcher
	limit   int
	bitmaps map[string]*myui.Bitmap
	colors  map[string]myui.Color
	order   []string
}

// newDiscoverLogos returns a logo cache for public catalog destinations.
func newDiscoverLogos() *logoCache {
	return &logoCache{
		fetcher: logos.NewFetcher(netguard.Policy{}),
		limit:   discoverBitmapLimit,
		bitmaps: map[string]*myui.Bitmap{},
		colors:  map[string]myui.Color{},
	}
}

// add converts img to a bitmap and stores the tint computed off the UI
// thread. Call it on the UI thread only.
func (c *logoCache) add(url string, img image.Image, tint color.RGBA) {
	if url == "" || img == nil {
		return
	}
	if _, ok := c.bitmaps[url]; ok {
		return
	}

	c.bitmaps[url] = myui.NewBitmap(img)
	c.colors[url] = myui.RGBA(tint.R, tint.G, tint.B, 1)
	c.order = append(c.order, url)

	for len(c.order) > c.limit {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.bitmaps, oldest)
		delete(c.colors, oldest)
	}
}

// lookup returns one cached bitmap and tint without copying the cache, for a
// small set of drawer rows on the UI thread.
func (c *logoCache) lookup(url string) (*myui.Bitmap, myui.Color) {
	return c.bitmaps[url], c.colors[url]
}

// snapshot copies the cache for a page's next frame.
func (c *logoCache) snapshot() map[string]*myui.Bitmap {
	out := make(map[string]*myui.Bitmap, len(c.bitmaps))
	for url, bitmap := range c.bitmaps {
		out[url] = bitmap
	}

	return out
}

// snapshotColors copies the bounded artwork tints for a page's next frame.
func (c *logoCache) snapshotColors() map[string]myui.Color {
	out := make(map[string]myui.Color, len(c.colors))
	for url, tint := range c.colors {
		out[url] = tint
	}

	return out
}

// startDiscover wires the page and loads what it needs without choosing a
// country. A fresh install shows the approved no-country state, and no country
// playlist is fetched until the viewer chooses one.
func (a *App) startDiscover() {
	page := a.shell.Discover()
	page.OnRegion = a.setDiscoverRegion
	page.OnRetry = a.retryDiscover
	page.OnReloadCountries = a.reloadDiscoverCountries
	page.OnFilter = a.filterDiscover
	page.OnLoadMore = a.loadMoreDiscover
	page.OnSelect = a.selectDiscoverChannel
	page.OnToggleFavorite = a.toggleFavoriteKey

	gen, ctx := a.discover.begin(a.ctx)
	a.Go(func(context.Context) error {
		a.loadDiscover(ctx, gen)

		return nil
	})

	countryGen, countryCtx := a.discoverCountries.begin(a.ctx)
	a.Go(func(context.Context) error {
		a.loadDiscoverCountries(countryCtx, countryGen)

		return nil
	})
}

// loadDiscover publishes the cached catalog, then refreshes it when the run
// is still current.
func (a *App) loadDiscover(ctx context.Context, gen uint64) {
	service, err := a.Catalog()
	if err != nil {
		a.publishDiscover(gen, ui.DiscoverState{
			Kind:    ui.DiscoverError,
			Message: ui.ProductName + " could not open its local library. Retry, or add your own playlist.",
		})

		return
	}

	region, err := service.Region(ctx)
	if err != nil {
		a.publishDiscover(gen, ui.DiscoverState{
			Kind:    ui.DiscoverError,
			Message: ui.ProductName + " could not read its saved settings. Retry, or add your own playlist.",
		})

		return
	}
	if !a.discover.current(gen) {
		return
	}

	enabled, err := service.BuiltinEnabled(ctx)
	if err != nil {
		a.publishDiscover(gen, ui.DiscoverState{
			Kind:    ui.DiscoverError,
			Region:  region,
			Message: ui.ProductName + " could not read the catalog settings. Retry, or add your own playlist.",
		})

		return
	}
	if !enabled {
		a.publish(func() {
			if a.discover.current(gen) {
				a.shell.Discover().SetNotice("")
			}
		})
		users, userErr := service.UserChannels(ctx)
		if userErr == nil && len(users) > 0 {
			a.publishCatalog(ctx, gen, ui.DiscoverReady, region, users)
			a.prefetchLogos(ctx, gen, users)
		} else {
			a.publishDiscover(gen, ui.DiscoverState{
				Kind:   ui.DiscoverDisabled,
				Region: region,
			})
		}

		return
	}

	if region == "" {
		a.publishNoCountry(ctx, gen, service)

		return
	}

	_, builtin, _, err := service.BuiltinSource(ctx, region)
	if err != nil {
		a.publishDiscover(gen, discoverFailure(region))

		return
	}
	if !a.discover.current(gen) {
		return
	}

	a.publish(func() {
		if a.discover.current(gen) {
			a.shell.Discover().SetNotice(discoverNotice(true))
		}
	})

	users, userErr := service.UserChannels(ctx)
	if userErr != nil {
		users = nil
	}

	channels := mergeDiscoverChannels(builtin, users)
	hasChannels := len(channels) > 0
	if hasChannels {
		a.publishCatalog(ctx, gen, ui.DiscoverReady, region, channels)
		a.prefetchLogos(ctx, gen, channels)
	} else {
		// No cache yet: keep the loading state until the refresh answers.
		a.publishDiscover(gen, ui.DiscoverState{Kind: ui.DiscoverLoading, Region: region})
	}

	a.refreshDiscover(ctx, gen, service, region, hasChannels, channels)
}

// publishNoCountry shows the approved first-launch state without fetching a
// country playlist. Imported sources stay usable, so they are shown instead of
// the empty prompt.
func (a *App) publishNoCountry(ctx context.Context, gen uint64, service *sources.Service) {
	users, err := service.UserChannels(ctx)
	if err != nil || len(users) == 0 {
		a.publishDiscover(gen, ui.DiscoverState{Kind: ui.DiscoverNoCountry})

		return
	}

	a.publish(func() {
		if a.discover.current(gen) {
			a.shell.Discover().SetNotice("")
		}
	})
	a.publishCatalog(ctx, gen, ui.DiscoverReady, "", users)
	a.prefetchLogos(ctx, gen, users)
}

// refreshDiscover fetches a fresh catalog. A failure keeps cached channels
// and reports the offline state; with nothing cached it reports an error.
func (a *App) refreshDiscover(
	ctx context.Context,
	gen uint64,
	service *sources.Service,
	region string,
	hasChannels bool,
	channels []catalog.Channel,
) {
	refreshCtx, cancel := context.WithTimeout(ctx, catalogRefreshTimeout)

	_, err := a.refreshBuiltin(refreshCtx, service, region)
	cancel()

	if err != nil {
		if ctx.Err() != nil || !a.discover.current(gen) {
			return
		}
		slog.Debug("community catalog refresh unavailable", "region", region)

		if hasChannels {
			a.publishCatalog(ctx, gen, ui.DiscoverOffline, region, channels)
		} else {
			a.publishDiscover(gen, discoverFailure(region))
		}

		return
	}
	if ctx.Err() != nil || !a.discover.current(gen) {
		return
	}

	_, freshBuiltin, _, err := service.BuiltinSource(ctx, region)
	if err != nil {
		if hasChannels {
			a.publishCatalog(ctx, gen, ui.DiscoverOffline, region, channels)
		} else {
			a.publishDiscover(gen, discoverFailure(region))
		}

		return
	}

	users, userErr := service.UserChannels(ctx)
	if userErr != nil {
		users = nil
	}
	fresh := mergeDiscoverChannels(freshBuiltin, users)

	if len(fresh) == 0 {
		a.publishDiscover(gen, ui.DiscoverState{
			Kind:    ui.DiscoverEmpty,
			Region:  region,
			Message: "The community catalog returned no channels. Retry, or add your own playlist.",
		})

		return
	}

	a.publishCatalog(ctx, gen, ui.DiscoverReady, region, fresh)
	a.prefetchLogos(ctx, gen, fresh)
}

// loadDiscoverCountries publishes the country picker list. A failure keeps the
// page usable and offers a retry inside the picker.
func (a *App) loadDiscoverCountries(ctx context.Context, gen uint64) {
	service, err := a.Catalog()
	if err != nil {
		return
	}

	countryCtx, cancel := context.WithTimeout(ctx, catalogRefreshTimeout)
	defer cancel()

	countries, _, err := service.Countries(countryCtx)
	if err != nil {
		slog.Debug("catalog region list unavailable", "error", err)
		if ctx.Err() == nil && a.discoverCountries.current(gen) {
			a.publish(func() {
				if a.discoverCountries.current(gen) {
					a.shell.Discover().SetCountriesError(
						ui.ProductName + " could not load the country list. Check your connection and retry.",
					)
				}
			})
		}

		return
	}

	list := make([]ui.DiscoverCountry, 0, len(countries))
	names := make(map[string]string, len(countries))
	for _, country := range countries {
		name := strings.TrimSpace(country.Name)
		if name == "" {
			name = country.Code
		}
		list = append(list, ui.DiscoverCountry{Code: country.Code, Name: name})
		names[strings.ToUpper(country.Code)] = name
	}

	a.mu.Lock()
	a.countryNames = names
	a.mu.Unlock()

	a.publish(func() {
		if !a.discoverCountries.current(gen) {
			return
		}
		a.shell.Discover().SetCountries(list)
		a.publishHome()
	})

	a.dropUnknownDiscoverRegion(list)
}

// reloadDiscoverCountries runs the country list again after a failure. It runs
// on the UI thread.
func (a *App) reloadDiscoverCountries() {
	gen, ctx := a.discoverCountries.begin(a.ctx)

	a.Go(func(context.Context) error {
		a.loadDiscoverCountries(ctx, gen)

		return nil
	})
}

// dropUnknownDiscoverRegion returns to the no-country state when a saved
// region is not in the catalog list. It never substitutes another country.
func (a *App) dropUnknownDiscoverRegion(countries []ui.DiscoverCountry) {
	region := a.shell.Discover().Region()
	if region == "" || len(countries) < 10 {
		return
	}

	for _, country := range countries {
		if strings.EqualFold(country.Code, region) {
			return
		}
	}

	gen, _ := a.discover.begin(a.ctx)

	a.publishDiscover(gen, ui.DiscoverState{
		Kind:    ui.DiscoverNoCountry,
		Message: "Your saved country is no longer in the catalog. Choose a country to continue.",
	})
}

// setDiscoverRegion stores a region choice and reloads the catalog. It runs
// on the UI thread.
func (a *App) setDiscoverRegion(code string) {
	region := strings.ToUpper(strings.TrimSpace(code))
	if region == "" {
		return
	}

	gen, ctx := a.discover.begin(a.ctx)
	// A filter run for the previous country must not republish over this load.
	a.discoverQuery.begin(a.ctx)
	a.shell.Discover().SetState(ui.DiscoverState{Kind: ui.DiscoverLoading, Region: region})

	a.GoQuiet(func(context.Context) {
		service, err := a.Catalog()
		if err != nil {
			a.publishDiscover(gen, ui.DiscoverState{
				Kind:    ui.DiscoverError,
				Region:  region,
				Message: ui.ProductName + " could not open its local library. Retry, or add your own playlist.",
			})

			return
		}

		saveCtx, cancel := context.WithTimeout(ctx, catalogRefreshTimeout)
		err = service.SetRegion(saveCtx, region)
		cancel()

		if err != nil {
			a.publishDiscover(gen, ui.DiscoverState{
				Kind:    ui.DiscoverError,
				Region:  region,
				Message: ui.ProductName + " could not change the catalog region. Retry, or add your own playlist.",
			})

			return
		}

		a.loadDiscover(ctx, gen)
	})
}

// retryDiscover reloads the catalog for the current region. It runs on the
// UI thread. With no country chosen it reloads the country list and imported
// sources only, so a retry never picks a country for the viewer.
func (a *App) retryDiscover() {
	region := a.shell.Discover().Region()

	gen, ctx := a.discover.begin(a.ctx)
	if region != "" {
		a.shell.Discover().SetState(ui.DiscoverState{Kind: ui.DiscoverLoading, Region: region})
	}

	a.Go(func(context.Context) error {
		a.loadDiscover(ctx, gen)

		return nil
	})

	countryGen, countryCtx := a.discoverCountries.begin(a.ctx)
	a.Go(func(context.Context) error {
		a.loadDiscoverCountries(countryCtx, countryGen)

		return nil
	})
}

// prefetchLogos downloads logos for the visible part of a catalog in the
// background, at most discoverLogoBudget channels.
func (a *App) prefetchLogos(ctx context.Context, gen uint64, channels []catalog.Channel) {
	urls := discoverLogoURLs(channels)
	if len(urls) == 0 {
		return
	}

	a.Go(func(context.Context) error {
		a.fetchLogos(ctx, gen, urls)

		return nil
	})
}

// prefetchArtwork loads the logos of saved channels for the Home feed and the
// Library rows, then republishes both. The fetcher dedupes cached and in-flight
// URLs, so a repeated library reload costs no network work.
func (a *App) prefetchArtwork(ctx context.Context, urls []string) {
	if len(urls) == 0 {
		return
	}

	a.Go(func(context.Context) error {
		images, tints := a.loadLogoImages(ctx, urls)
		if len(images) == 0 {
			return nil
		}

		a.publish(func() {
			for url, img := range images {
				a.logos.add(url, img, tints[url])
			}
			a.shell.Library().SetLogos(a.logos.snapshot())
			a.shell.Library().SetLogoColors(a.logos.snapshotColors())
			a.shell.Home().SetLogos(a.logos.snapshot())
			a.shell.Home().SetLogoColors(a.logos.snapshotColors())
		})

		return nil
	})
}

// loadLogoImages decodes urls with bounded concurrency. Failures stay silent:
// tiles fall back to a monogram.
func (a *App) loadLogoImages(ctx context.Context, urls []string) (map[string]image.Image, map[string]color.RGBA) {
	var (
		mu     sync.Mutex
		images = make(map[string]image.Image, len(urls))
		tints  = make(map[string]color.RGBA, len(urls))
		wg     sync.WaitGroup
	)

	slots := make(chan struct{}, discoverLogoWorkers)

	for _, url := range urls {
		if ctx.Err() != nil {
			break
		}

		slots <- struct{}{}
		wg.Add(1)

		go func(url string) {
			defer wg.Done()
			defer func() { <-slots }()

			img, err := a.logos.fetcher.Load(ctx, url)
			if err != nil || img == nil {
				return
			}

			tint := logos.DominantColor(img)
			mu.Lock()
			images[url] = img
			tints[url] = tint
			mu.Unlock()
		}(url)
	}

	wg.Wait()

	return images, tints
}

// fetchLogos loads urls with bounded concurrency and publishes the decoded
// images once. Failures stay silent: rows fall back to initials.
func (a *App) fetchLogos(ctx context.Context, gen uint64, urls []string) {
	images, tints := a.loadLogoImages(ctx, urls)
	if len(images) == 0 || !a.discover.current(gen) {
		return
	}

	a.publish(func() {
		if !a.discover.current(gen) {
			return
		}
		for url, img := range images {
			a.logos.add(url, img, tints[url])
		}
		a.shell.Discover().SetLogos(a.logos.snapshot())
		a.shell.Discover().SetLogoColors(a.logos.snapshotColors())
	})
}

// publishDiscover shows state on the UI thread unless a newer run replaced it.
func (a *App) publishDiscover(gen uint64, state ui.DiscoverState) {
	empty := len(state.Channels) == 0
	if empty {
		a.discoverIndex.setCatalog(nil, discoverFacets{}, discoverBase{
			kind:    state.Kind,
			region:  state.Region,
			message: state.Message,
		})
	}

	a.publish(func() {
		if !a.discover.current(gen) {
			return
		}

		page := a.shell.Discover()
		page.SetState(state)
		page.SetLogos(a.logos.snapshot())
		page.SetLogoColors(a.logos.snapshotColors())
	})
}

// publishCatalog indexes a catalog and publishes its narrowed rows. Facet work
// and filtering stay off the UI thread; only the finished rows are published.
func (a *App) publishCatalog(
	ctx context.Context,
	gen uint64,
	kind ui.DiscoverKind,
	region string,
	channels []catalog.Channel,
) {
	service, _ := a.Catalog()
	facets := discoverFacetsFor(ctx, service, channels)

	a.publishCatalogState(gen, kind, region, "", channels, facets)
}

// publishCatalogState publishes a catalog with facets already built.
func (a *App) publishCatalogState(
	gen uint64,
	kind ui.DiscoverKind,
	region string,
	message string,
	channels []catalog.Channel,
	facets discoverFacets,
) {
	base := discoverBase{kind: kind, region: region, message: message}
	a.discoverIndex.setCatalog(channels, facets, base)

	guard := func() bool { return a.discover.current(gen) }
	a.publishView(base, guard)
}

// publishView narrows the indexed catalog off the UI thread and shows every
// match, capped for safety. It returns the rows so the caller can prefetch.
func (a *App) publishView(
	base discoverBase,
	guard func() bool,
) []ui.DiscoverChannel {
	filters := a.discoverIndex.currentFilters()
	rows, total, truncated := a.discoverIndex.view(filters)

	categories := a.discoverIndex.categoryCounts(filters)
	browseDrawer := a.discoverIndex.drawerChannels(discoverResultLimit)

	a.publish(func() {
		if guard != nil && !guard() {
			return
		}

		discoverPage := a.shell.Discover()
		discoverPage.SetFacets(categories)
		discoverPage.SetFavorites(a.libraryData.favoriteKeys())
		discoverPage.SetState(ui.DiscoverState{
			Kind:      base.kind,
			Region:    base.region,
			Message:   base.message,
			Channels:  rows,
			Total:     total,
			Truncated: truncated,
			HasMore:   total > len(rows),
		})
		discoverPage.SetLogos(a.logos.snapshot())
		discoverPage.SetLogoColors(a.logos.snapshotColors())

		a.mu.Lock()
		a.media.browseDrawer = browseDrawer
		a.mu.Unlock()
		a.shell.Watch().SetState(a.watchState())
	})

	return rows
}

// filterDiscover narrows the indexed catalog off the UI thread. Typing
// debounces; a newer filter cancels the previous one.
func (a *App) filterDiscover(filters ui.DiscoverFilters) {
	a.discoverIndex.setFilters(filters)
	a.refreshView(discoverFilterDelay)
}

// loadMoreDiscover adds one batch for the infinite scroll. It runs on the UI
// thread and republishes from the index without touching the network.
func (a *App) loadMoreDiscover() {
	a.discoverIndex.growLimit()
	a.refreshView(0)
}

// refreshView republishes the current narrowing, guarded by both the filter
// run and the catalog run so a stale result never wins.
func (a *App) refreshView(delay time.Duration) {
	gen, ctx := a.discoverQuery.begin(a.ctx)
	discoverGen := a.discover.generation()
	guard := func() bool {
		return a.discoverQuery.current(gen) && a.discover.current(discoverGen)
	}

	a.GoQuiet(func(context.Context) {
		if delay > 0 && !sleepContext(ctx, delay) {
			return
		}
		if ctx.Err() != nil {
			return
		}

		base := a.discoverIndex.base()
		switch base.kind {
		case ui.DiscoverReady, ui.DiscoverOffline:
		default:
			return
		}

		rows := a.publishView(base, guard)
		a.prefetchRowLogos(ctx, discoverGen, rows)
	})
}

// prefetchRowLogos loads the logos of the rows just published.
func (a *App) prefetchRowLogos(ctx context.Context, gen uint64, rows []ui.DiscoverChannel) {
	urls := rowLogoURLs(rows)
	if len(urls) == 0 {
		return
	}

	a.Go(func(context.Context) error {
		a.fetchLogos(ctx, gen, urls)

		return nil
	})
}

// selectDiscoverChannel resolves a row key and opens Watch on that channel.
func (a *App) selectDiscoverChannel(key string) {
	channel, ok := a.discoverIndex.channel(key)
	if !ok {
		return
	}

	a.playChannel(channel, a.discoverIndex.sourceName(channel.SourceID))
}

// sleepContext waits for d unless ctx ends first.
func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// discoverFailure reports a catalog that failed with nothing cached.
func discoverFailure(region string) ui.DiscoverState {
	return ui.DiscoverState{Kind: ui.DiscoverError, Region: region}
}

// discoverRow maps one domain channel to a page row.
func discoverRow(channel catalog.Channel, position int, names map[string]string) ui.DiscoverChannel {
	return ui.DiscoverChannel{
		Key:        discoverKey(channel, position),
		Name:       channel.Name,
		Country:    channel.Country,
		Category:   channel.Group,
		SourceName: names[channel.SourceID],
		LogoURL:    channel.LogoURL,
		Live:       channelLive(channel),
	}
}

// discoverChannels maps the domain to channel rows.
func discoverChannels(channels []catalog.Channel) []ui.DiscoverChannel {
	rows := make([]ui.DiscoverChannel, 0, len(channels))

	for i, channel := range channels {
		rows = append(rows, discoverRow(channel, i, nil))
	}

	return rows
}

// mergeDiscoverChannels combines the community catalog with user sources.
// Duplicate display names stay separate because every row keeps its own key.
func mergeDiscoverChannels(builtin, users []catalog.Channel) []catalog.Channel {
	channels := make([]catalog.Channel, 0, len(builtin)+len(users))
	channels = append(channels, builtin...)
	channels = append(channels, users...)

	return channels
}

// discoverNotice is the rights and attribution line for the community list.
func discoverNotice(enabled bool) string {
	if !enabled {
		return ""
	}

	return "Community channel list · iptv-org · not verified"
}

// channelLive reports whether any variant is a live format. It says nothing
// about upstream availability.
func channelLive(channel catalog.Channel) bool {
	for _, stream := range channel.Streams {
		if stream.Live {
			return true
		}
	}

	return false
}

// discoverLogoURLs collects distinct logo URLs for the first channels.
func discoverLogoURLs(channels []catalog.Channel) []string {
	urls := make([]string, 0, discoverLogoBudget)
	seen := make(map[string]struct{}, discoverLogoBudget)

	for _, channel := range channels {
		url := strings.TrimSpace(channel.LogoURL)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}

		seen[url] = struct{}{}
		urls = append(urls, url)

		if len(urls) == discoverLogoBudget {
			break
		}
	}

	return urls
}

// rowLogoURLs collects distinct published row logos, bounded like the catalog
// prefetch so one page cannot start unbounded work.
func rowLogoURLs(rows []ui.DiscoverChannel) []string {
	urls := make([]string, 0, min(len(rows), discoverLogoBudget))
	seen := make(map[string]struct{}, len(rows))

	for _, row := range rows {
		url := strings.TrimSpace(row.LogoURL)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}

		seen[url] = struct{}{}
		urls = append(urls, url)

		if len(urls) == discoverLogoBudget {
			break
		}
	}

	return urls
}
