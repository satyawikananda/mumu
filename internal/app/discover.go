package app

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"strings"
	"sync"

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
			a.publishDiscover(gen, discoverState(ui.DiscoverReady, region, users))
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
		a.publishDiscover(gen, discoverState(ui.DiscoverReady, region, channels))
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
			a.publishDiscover(gen, discoverState(ui.DiscoverOffline, region, channels))
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
			a.publishDiscover(gen, discoverState(ui.DiscoverOffline, region, channels))
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

	a.publishDiscover(gen, discoverState(ui.DiscoverReady, region, fresh))
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

	a.Go(func(context.Context) error {
		service, err := a.Catalog()
		if err != nil {
			a.publishDiscover(gen, ui.DiscoverState{
				Kind:    ui.DiscoverError,
				Region:  region,
				Message: "mumu could not open its local library. Retry, or add your own playlist.",
			})

			return nil
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

			return nil
		}

		a.loadDiscover(ctx, gen)

		return nil
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
	a.publish(func() {
		if !a.discover.current(gen) {
			return
		}

		page := a.shell.Discover()
		page.SetState(state)
		page.SetLogos(a.logos.snapshot())
	})
}

// discoverState maps catalog channels to page rows.
func discoverState(kind ui.DiscoverKind, region string, channels []catalog.Channel) ui.DiscoverState {
	return ui.DiscoverState{
		Kind:     kind,
		Region:   region,
		Channels: discoverChannels(channels),
	}
}

// discoverFailure reports a catalog that failed with nothing cached.
func discoverFailure(region string) ui.DiscoverState {
	return ui.DiscoverState{Kind: ui.DiscoverError, Region: region}
}

// discoverChannels maps the domain to channel rows.
func discoverChannels(channels []catalog.Channel) []ui.DiscoverChannel {
	rows := make([]ui.DiscoverChannel, 0, len(channels))

	for i, channel := range channels {
		key := channel.ProviderID
		if key == "" {
			key = fmt.Sprintf("row-%d", i)
		} else if channel.SourceID != "" {
			// Provider IDs are unique within a source, not across sources.
			key = channel.SourceID + ":" + key
		}

		rows = append(rows, ui.DiscoverChannel{
			Key:      key,
			Name:     channel.Name,
			Country:  channel.Country,
			Category: channel.Group,
			LogoURL:  channel.LogoURL,
			Live:     channelLive(channel),
		})
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
