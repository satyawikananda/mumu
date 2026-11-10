package app

import (
	"context"
	"fmt"
	"image"
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
	// discoverResultLimit caps the rows published for one filter.
	discoverResultLimit = 2000
	// discoverFilterDelay debounces typing before the catalog is filtered.
	discoverFilterDelay = 150 * time.Millisecond
	// discoverCategoryLimit bounds the category filter options.
	discoverCategoryLimit = 200
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

// discoverFacets are the filter options and row attributions for a catalog.
type discoverFacets struct {
	categories []string
	sources    []ui.DiscoverFacet
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
	baseKind   ui.DiscoverKind
	baseRegion string
	baseMsg    string
	filters    ui.DiscoverFilters
}

// newDiscoverIndex returns an index with no catalog.
func newDiscoverIndex() *discoverIndex {
	return &discoverIndex{
		keys:  map[string]catalog.Channel{},
		names: map[string]string{},
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
	i.baseKind = base.kind
	i.baseRegion = base.region
	i.baseMsg = base.message

	i.keys = make(map[string]catalog.Channel, len(channels))
	for position, channel := range channels {
		i.keys[discoverKey(channel, position)] = channel
	}

	if i.filters.Category != "" && !slices.Contains(facets.categories, i.filters.Category) {
		i.filters.Category = ""
	}
	if i.filters.SourceID != "" && !facetIDExists(facets.sources, i.filters.SourceID) {
		i.filters.SourceID = ""
	}

	return i.filters
}

// setFilters remembers the page's narrowing for refreshes.
func (i *discoverIndex) setFilters(filters ui.DiscoverFilters) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.filters = filters
}

// base returns the state the filtered rows belong to.
func (i *discoverIndex) base() discoverBase {
	i.mu.Lock()
	defer i.mu.Unlock()

	return discoverBase{kind: i.baseKind, region: i.baseRegion, message: i.baseMsg}
}

// drawerChannels maps the indexed catalog to Watch drawer rows.
func (i *discoverIndex) drawerChannels(limit int) []ui.WatchChannel {
	i.mu.Lock()
	channels := i.channels
	names := i.names
	i.mu.Unlock()

	count := min(len(channels), limit)
	rows := make([]ui.WatchChannel, 0, count)
	for position := 0; position < count; position++ {
		channel := channels[position]
		meta := strings.TrimSpace(channel.Group)
		if name := names[channel.SourceID]; name != "" {
			if meta != "" {
				meta += " · "
			}
			meta += name
		}
		rows = append(rows, ui.WatchChannel{
			Key:  discoverKey(channel, position),
			Name: channel.Name,
			Meta: meta,
		})
	}

	return rows
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

// filter narrows the catalog, capped at discoverResultLimit rows.
func (i *discoverIndex) filter(filters ui.DiscoverFilters) ([]ui.DiscoverChannel, int, bool) {
	i.mu.Lock()
	channels := i.channels
	names := i.names
	i.mu.Unlock()

	capacity := min(len(channels), discoverResultLimit)
	rows := make([]ui.DiscoverChannel, 0, capacity)
	match := catalog.Filter{
		Query:    filters.Query,
		Category: filters.Category,
		SourceID: filters.SourceID,
	}

	total := 0
	for position, channel := range channels {
		if !match.Match(channel) {
			continue
		}
		total++
		if len(rows) == discoverResultLimit {
			continue
		}
		rows = append(rows, discoverRow(channel, position, names))
	}

	return rows, total, total > len(rows)
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

// facetIDExists reports whether a source facet carries id.
func facetIDExists(facets []ui.DiscoverFacet, id string) bool {
	for _, facet := range facets {
		if facet.ID == id {
			return true
		}
	}

	return false
}

// discoverFacetsFor builds the category and source options for channels.
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
	sourceSet := map[string]struct{}{}

	for _, channel := range channels {
		if category := strings.TrimSpace(channel.Group); category != "" {
			categorySet[category] = struct{}{}
		}
		if channel.SourceID != "" {
			sourceSet[channel.SourceID] = struct{}{}
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

	sourceIDs := make([]string, 0, len(sourceSet))
	for id := range sourceSet {
		sourceIDs = append(sourceIDs, id)
	}
	slices.Sort(sourceIDs)

	sources := make([]ui.DiscoverFacet, 0, len(sourceIDs))
	used := map[string]int{}
	for _, id := range sourceIDs {
		name := names[id]
		if name == "" {
			name = "Playlist"
		}
		// Two sources can share a display name; the selector needs distinct
		// labels, so a repeat gets a stable numeric suffix.
		used[name]++
		if count := used[name]; count > 1 {
			name = fmt.Sprintf("%s (%d)", name, count)
		}
		names[id] = name
		sources = append(sources, ui.DiscoverFacet{ID: id, Name: name})
	}

	return discoverFacets{categories: categories, sources: sources, names: names}
}

// logoCache keeps channel logo bitmaps made on the UI thread, bounded by URL.
type logoCache struct {
	fetcher *logos.Fetcher
	limit   int
	bitmaps map[string]*myui.Bitmap
	order   []string
}

// newDiscoverLogos returns a logo cache for public catalog destinations.
func newDiscoverLogos() *logoCache {
	return &logoCache{
		fetcher: logos.NewFetcher(netguard.Policy{}),
		limit:   discoverBitmapLimit,
		bitmaps: map[string]*myui.Bitmap{},
	}
}

// add converts img to a bitmap. Call it on the UI thread only.
func (c *logoCache) add(url string, img image.Image) {
	if url == "" || img == nil {
		return
	}
	if _, ok := c.bitmaps[url]; ok {
		return
	}

	c.bitmaps[url] = myui.NewBitmap(img)
	c.order = append(c.order, url)

	for len(c.order) > c.limit {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.bitmaps, oldest)
	}
}

// snapshot copies the cache for a page's next frame.
func (c *logoCache) snapshot() map[string]*myui.Bitmap {
	out := make(map[string]*myui.Bitmap, len(c.bitmaps))
	for url, bitmap := range c.bitmaps {
		out[url] = bitmap
	}

	return out
}

// startDiscover wires the page, shows the cached catalog and refreshes it.
// Catalog failures stay inside Discover; they never raise the global banner.
func (a *App) startDiscover() {
	page := a.shell.Discover()
	page.OnRegion = a.setDiscoverRegion
	page.OnRetry = a.retryDiscover
	page.OnFilter = a.filterDiscover
	page.OnSelect = a.selectDiscoverChannel
	page.SetState(ui.DiscoverState{
		Kind:   ui.DiscoverLoading,
		Region: sources.DefaultRegion,
	})

	gen, ctx := a.discover.begin(a.ctx)
	a.Go(func(context.Context) error {
		a.loadDiscover(ctx, gen)

		return nil
	})
	a.Go(func(ctx context.Context) error {
		a.loadDiscoverCountries(ctx)

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
			Message: "mumu could not open its local library. Retry, or add your own playlist.",
		})

		return
	}

	region, err := service.Region(ctx)
	if err != nil {
		a.publishDiscover(gen, ui.DiscoverState{
			Kind:    ui.DiscoverError,
			Message: "mumu could not read its saved settings. Retry, or add your own playlist.",
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
			Message: "mumu could not read the catalog settings. Retry, or add your own playlist.",
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
				Kind:    ui.DiscoverEmpty,
				Region:  region,
				Message: "The community catalog is turned off. Add your own playlist to start watching.",
			})
		}

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

// loadDiscoverCountries publishes the region selector list.
func (a *App) loadDiscoverCountries(ctx context.Context) {
	service, err := a.Catalog()
	if err != nil {
		return
	}

	countryCtx, cancel := context.WithTimeout(ctx, catalogRefreshTimeout)
	defer cancel()

	countries, _, err := service.Countries(countryCtx)
	if err != nil {
		slog.Debug("catalog region list unavailable")

		return
	}

	list := make([]ui.DiscoverCountry, 0, len(countries))
	for _, country := range countries {
		name := strings.TrimSpace(country.Name)
		if name == "" {
			name = country.Code
		}
		list = append(list, ui.DiscoverCountry{Code: country.Code, Name: name})
	}

	a.publish(func() {
		a.shell.Discover().SetCountries(list)
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
	a.shell.Discover().SetState(ui.DiscoverState{Kind: ui.DiscoverLoading, Region: region})

	a.GoQuiet(func(context.Context) {
		service, err := a.Catalog()
		if err != nil {
			a.publishDiscover(gen, ui.DiscoverState{
				Kind:    ui.DiscoverError,
				Region:  region,
				Message: "mumu could not open its local library. Retry, or add your own playlist.",
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
				Message: "mumu could not change the catalog region. Retry, or add your own playlist.",
			})

			return
		}

		a.loadDiscover(ctx, gen)
	})
}

// retryDiscover reloads the catalog for the current region. It runs on the
// UI thread.
func (a *App) retryDiscover() {
	region := a.shell.Discover().Region()

	gen, ctx := a.discover.begin(a.ctx)
	a.shell.Discover().SetState(ui.DiscoverState{Kind: ui.DiscoverLoading, Region: region})

	a.Go(func(context.Context) error {
		a.loadDiscover(ctx, gen)

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

// fetchLogos loads urls with bounded concurrency and publishes the decoded
// images once. Failures stay silent: rows fall back to initials.
func (a *App) fetchLogos(ctx context.Context, gen uint64, urls []string) {
	var (
		mu     sync.Mutex
		images = make(map[string]image.Image, len(urls))
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

			mu.Lock()
			images[url] = img
			mu.Unlock()
		}(url)
	}

	wg.Wait()

	if len(images) == 0 || !a.discover.current(gen) {
		return
	}

	a.publish(func() {
		if !a.discover.current(gen) {
			return
		}
		for url, img := range images {
			a.logos.add(url, img)
		}
		a.shell.Discover().SetLogos(a.logos.snapshot())
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
	filters := a.discoverIndex.setCatalog(channels, facets, base)
	rows, total, truncated := a.discoverIndex.filter(filters)
	drawer := a.discoverIndex.drawerChannels(discoverResultLimit)

	a.publish(func() {
		if !a.discover.current(gen) {
			return
		}

		page := a.shell.Discover()
		page.SetFacets(facets.categories, facets.sources)
		page.SetState(ui.DiscoverState{
			Kind:      kind,
			Region:    region,
			Message:   message,
			Channels:  rows,
			Total:     total,
			Truncated: truncated,
		})
		page.SetLogos(a.logos.snapshot())

		a.mu.Lock()
		a.media.drawer = drawer
		a.mu.Unlock()
		a.shell.Watch().SetState(a.watchState())
	})
}

// filterDiscover narrows the indexed catalog off the UI thread. Typing
// debounces; a newer filter cancels the previous one.
func (a *App) filterDiscover(filters ui.DiscoverFilters) {
	a.discoverIndex.setFilters(filters)

	gen, ctx := a.discoverQuery.begin(a.ctx)

	a.GoQuiet(func(context.Context) {
		if !sleepContext(ctx, discoverFilterDelay) {
			return
		}
		if ctx.Err() != nil {
			return
		}

		rows, total, truncated := a.discoverIndex.filter(filters)
		base := a.discoverIndex.base()

		switch base.kind {
		case ui.DiscoverReady, ui.DiscoverOffline:
		default:
			return
		}

		if ctx.Err() != nil || !a.discoverQuery.current(gen) {
			return
		}

		a.publish(func() {
			if !a.discoverQuery.current(gen) {
				return
			}
			a.shell.Discover().SetState(ui.DiscoverState{
				Kind:      base.kind,
				Region:    base.region,
				Message:   base.message,
				Channels:  rows,
				Total:     total,
				Truncated: truncated,
			})
		})
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
