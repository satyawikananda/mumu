package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	// discoverAllCategories is the category selector's "no restriction" label.
	discoverAllCategories = "All categories"
	// discoverAllSources is the source selector's "no restriction" label.
	discoverAllSources = "All sources"
)

// DiscoverKind is the state the Discover page shows.
type DiscoverKind int

const (
	// DiscoverLoading reports catalog work in progress.
	DiscoverLoading DiscoverKind = iota + 1
	// DiscoverReady shows the current catalog.
	DiscoverReady
	// DiscoverOffline shows cached channels with a refresh warning.
	DiscoverOffline
	// DiscoverEmpty reports an enabled catalog with no usable channels.
	DiscoverEmpty
	// DiscoverError reports a catalog failure with nothing cached.
	DiscoverError
)

// DiscoverChannel is one row of the Discover channel list.
//
// Key is a stable identity, not the display name. Country is a region code
// that the page resolves through its published countries; Category is the
// playlist group; SourceName attributes the row to its playlist.
type DiscoverChannel struct {
	Key        string
	Name       string
	Country    string
	Category   string
	SourceName string
	LogoURL    string
	Live       bool
}

// DiscoverCountry is one selectable catalog region.
type DiscoverCountry struct {
	Code string
	Name string
}

// DiscoverFacet is one selectable filter option with a stable ID.
type DiscoverFacet struct {
	ID   string
	Name string
}

// DiscoverFilters is the page's current narrowing. Zero values mean "all".
type DiscoverFilters struct {
	Query    string
	Category string
	SourceID string
}

// Empty reports whether the filters narrow nothing.
func (f DiscoverFilters) Empty() bool {
	return strings.TrimSpace(f.Query) == "" && f.Category == "" && f.SourceID == ""
}

// DiscoverState is what the app publishes to the Discover page.
type DiscoverState struct {
	Kind     DiscoverKind
	Region   string
	Channels []DiscoverChannel
	// Message overrides the page's default text for the state.
	Message string
	// Total counts every channel that matched, before the row cap.
	Total int
	// Truncated reports that Channels is a capped prefix of Total.
	Truncated bool
}

// Discover is the native Discover page. The app publishes the catalog state;
// the page dispatches intents through its callbacks and owns no network work.
type Discover struct {
	state      DiscoverState
	countries  []DiscoverCountry
	categories []string
	sources    []DiscoverFacet
	logos      map[string]*ui.Bitmap
	list       ui.ListState
	selected   int
	notice     string

	query       string
	category    string
	sourceID    string
	selectedKey string

	// OnRegion asks the app to switch the catalog region.
	OnRegion func(code string)
	// OnRetry asks the app to load the catalog again.
	OnRetry func()
	// OnAddPlaylist sends the viewer to the playlist import surface.
	OnAddPlaylist func()
	// OnFilter reports a new narrowing; the app debounces and filters.
	OnFilter func(filters DiscoverFilters)
	// OnSelect asks the app to open the channel with this key.
	OnSelect func(key string)
}

// NewDiscover returns an empty Discover page with no region yet.
func NewDiscover() *Discover {
	d := &Discover{
		state:    DiscoverState{Kind: DiscoverEmpty},
		selected: -1,
	}
	d.list.Selected = &d.selected
	d.list.Label = d.rowLabel

	return d
}

// SetState shows state; an empty region keeps the current one.
func (d *Discover) SetState(state DiscoverState) {
	if state.Region == "" {
		state.Region = d.state.Region
	}
	if state.Total == 0 && len(state.Channels) > 0 {
		state.Total = len(state.Channels)
	}
	d.state = state
	d.selected = d.indexForKey(d.selectedKey)
}

// indexForKey finds a row by its stable key, -1 when it is gone.
func (d *Discover) indexForKey(key string) int {
	if key == "" {
		return -1
	}
	for i, channel := range d.state.Channels {
		if channel.Key == key {
			return i
		}
	}

	return -1
}

// State returns what the page shows, for the app and for tests.
func (d *Discover) State() DiscoverState { return d.state }

// Region returns the selected catalog region code.
func (d *Discover) Region() string { return d.state.Region }

// Filters returns the page's current narrowing.
func (d *Discover) Filters() DiscoverFilters {
	return DiscoverFilters{
		Query:    strings.TrimSpace(d.query),
		Category: d.category,
		SourceID: d.sourceID,
	}
}

// SetCountries publishes the region list for the selector.
func (d *Discover) SetCountries(countries []DiscoverCountry) {
	d.countries = countries
}

// SetFacets publishes the category and source filter options. A selection
// that no longer exists falls back to "all".
func (d *Discover) SetFacets(categories []string, sources []DiscoverFacet) {
	d.categories = categories
	d.sources = sources

	if d.category != "" && !slices.Contains(categories, d.category) {
		d.category = ""
	}
	if d.sourceID != "" && !facetExists(sources, d.sourceID) {
		d.sourceID = ""
	}
}

// SetNotice publishes the catalog's attribution and rights note. An empty
// notice hides it.
func (d *Discover) SetNotice(notice string) {
	d.notice = strings.TrimSpace(notice)
}

// Notice returns the attribution line, empty when hidden.
func (d *Discover) Notice() string { return d.notice }

// SetLogos publishes logo bitmaps keyed by URL; a missing one falls back to
// the channel's initials.
func (d *Discover) SetLogos(logos map[string]*ui.Bitmap) {
	d.logos = logos
}

// View draws the page on the MyGo main thread.
func (d *Discover) View(c *ui.Context, theme *ui.Theme) {
	d.header(c, theme)
	d.filters(c, theme)
	d.body(c, theme)
}

// header draws the page heading, region selector and Add Playlist.
func (d *Discover) header(c *ui.Context, _ *ui.Theme) {
	labels := d.regionLabels()

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
		pageHeading(c, "Discover").Grow(1)

		if len(labels) > 0 {
			region := d.regionLabel()
			selector := ui.Select(c, &region, labels).Label("Region")
			if selector.Changed() {
				if code, ok := d.countryCode(region); ok && code != d.state.Region {
					d.state.Region = code
					if d.OnRegion != nil {
						d.OnRegion(code)
					}
				}
			}
		}

		ui.Button(c, "Add Playlist").OnClick(func() {
			if d.OnAddPlaylist != nil {
				d.OnAddPlaylist()
			}
		})
	})
}

// filters draws the search field and the category and source selectors.
func (d *Discover) filters(c *ui.Context, _ *ui.Theme) {
	search := ui.SearchField(c, &d.query).Label("Search channels")
	if search.Changed() {
		d.emitFilter()
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		if len(d.categories) > 0 {
			labels := append([]string{discoverAllCategories}, d.categories...)
			choice := d.category
			if choice == "" {
				choice = discoverAllCategories
			}

			selector := ui.Select(c, &choice, labels).Label("Category").MaxWidth(180)
			if selector.Changed() {
				category := strings.TrimSpace(choice)
				if category == discoverAllCategories {
					category = ""
				}
				if category != d.category {
					d.category = category
					d.emitFilter()
				}
			}
		}

		if len(d.sources) > 0 {
			labels := []string{discoverAllSources}
			for _, source := range d.sources {
				labels = append(labels, source.Name)
			}

			choice := d.sourceLabel()
			selector := ui.Select(c, &choice, labels).Label("Source").MaxWidth(180)
			if selector.Changed() {
				id := facetID(d.sources, choice)
				if id != d.sourceID {
					d.sourceID = id
					d.emitFilter()
				}
			}
		}

		if !d.Filters().Empty() {
			ui.Button(c, "Clear filters").OnClick(d.clearFilters)
		}
	})
}

// body draws the state, with a virtualized list when channels are known.
func (d *Discover) body(c *ui.Context, theme *ui.Theme) {
	switch d.state.Kind {
	case DiscoverLoading:
		pageStatus(c, Status{
			Kind:    StatusLoading,
			Title:   "Loading channels",
			Message: d.defaultMessage(),
		})
	case DiscoverError:
		pageStatus(c, Status{
			Kind:     StatusError,
			Title:    "Channels unavailable",
			Message:  d.defaultMessage(),
			Action:   "Retry",
			OnAction: d.retry,
		})
	case DiscoverOffline:
		d.offline(c, theme)
	case DiscoverReady:
		if len(d.state.Channels) == 0 {
			d.empty(c)
			return
		}
		d.channelList(c, theme)
	default:
		d.empty(c)
	}
}

// empty draws the empty state: filters when they explain it, retry otherwise.
func (d *Discover) empty(c *ui.Context) {
	if !d.Filters().Empty() {
		pageStatus(c, Status{
			Kind:     StatusEmpty,
			Title:    "No channels match",
			Message:  d.defaultMessage(),
			Action:   "Clear filters",
			OnAction: d.clearFilters,
		})

		return
	}

	pageStatus(c, Status{
		Kind:     StatusEmpty,
		Title:    "No channels yet",
		Message:  d.defaultMessage(),
		Action:   "Retry",
		OnAction: d.retry,
	})
}

// offline draws cached channels with a refresh warning above them, or the
// offline status when nothing was cached.
func (d *Discover) offline(c *ui.Context, theme *ui.Theme) {
	if len(d.state.Channels) == 0 {
		pageStatus(c, Status{
			Kind:     StatusOffline,
			Title:    "You are offline",
			Message:  "Connect to the internet and retry, or add your own playlist.",
			Action:   "Retry",
			OnAction: d.retry,
		})

		return
	}

	message := d.state.Message
	if message == "" {
		message = "Showing saved channels. The catalog could not be refreshed."
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 12).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Text(c, "!").TextColor(theme.Warning).AccessHidden(true)
		ui.Text(c, message).TextColor(theme.TextMuted).Grow(1)
		ui.Button(c, "Retry").OnClick(d.retry)
	})

	d.channelList(c, theme)
}

// channelList draws the result summary and the virtualized channel rows.
func (d *Discover) channelList(c *ui.Context, theme *ui.Theme) {
	if d.notice != "" {
		ui.Text(c, d.notice).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
	}
	if summary := d.resultSummary(); summary != "" {
		ui.Text(c, summary).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
	}

	list := ui.List(c, &d.list, len(d.state.Channels), func(i int) {
		d.row(c, theme, i)
	}).
		Grow(1).
		Label("Channels").
		ItemKey(func(i int) any { return d.state.Channels[i].Key })
	if d.selected >= 0 && d.selected < len(d.state.Channels) {
		d.selectedKey = d.state.Channels[d.selected].Key
	} else {
		d.selectedKey = ""
	}
	if list.Submitted() {
		d.submit()
	}
}

// row draws one channel with its logo, title, metadata and format label.
func (d *Discover) row(c *ui.Context, theme *ui.Theme, i int) {
	channel := d.state.Channels[i]

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 10).Children(func() {
		ui.Avatar(c, channel.Name, d.logos[channel.LogoURL])

		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, channel.Name).FontSize(theme.Rem(1.08)).FontWeight(500).SingleLine()
			if meta := d.channelMeta(channel); meta != "" {
				ui.Text(c, meta).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
			}
		})

		if channel.Live {
			ui.Text(c, "LIVE").FontSize(theme.Rem(0.8)).FontWeight(600).
				TextColor(theme.TextMuted).Padding(2, 7).Radius(4).
				Background(theme.Surface).Border(1, theme.Border)
		}
	})
}

// submit opens the focused channel on Enter or a double click.
func (d *Discover) submit() {
	if d.selected < 0 || d.selected >= len(d.state.Channels) {
		return
	}
	if d.OnSelect != nil {
		d.OnSelect(d.state.Channels[d.selected].Key)
	}
}

// retry dispatches the retry intent.
func (d *Discover) retry() {
	if d.OnRetry != nil {
		d.OnRetry()
	}
}

// emitFilter reports the current narrowing.
func (d *Discover) emitFilter() {
	if d.OnFilter != nil {
		d.OnFilter(d.Filters())
	}
}

// clearFilters resets the narrowing and reports it.
func (d *Discover) clearFilters() {
	d.query = ""
	d.category = ""
	d.sourceID = ""
	d.emitFilter()
}

// rowLabel names a row for assistive technology and type-select.
func (d *Discover) rowLabel(i int) string {
	if i < 0 || i >= len(d.state.Channels) {
		return ""
	}

	channel := d.state.Channels[i]
	parts := []string{channel.Name}
	if meta := d.channelMeta(channel); meta != "" {
		parts = append(parts, meta)
	}
	if channel.Live {
		parts = append(parts, "live format")
	}

	return strings.Join(parts, ", ")
}

// channelMeta builds "Country · Category · Source" from a row.
func (d *Discover) channelMeta(channel DiscoverChannel) string {
	parts := make([]string, 0, 3)

	if name := d.countryName(channel.Country); name != "" {
		parts = append(parts, name)
	}
	if category := strings.TrimSpace(channel.Category); category != "" {
		parts = append(parts, category)
	}
	if source := strings.TrimSpace(channel.SourceName); source != "" {
		parts = append(parts, source)
	}

	return strings.Join(parts, " · ")
}

// resultSummary counts results, and says when the list is capped.
func (d *Discover) resultSummary() string {
	total := d.state.Total
	if total == 0 {
		total = len(d.state.Channels)
	}
	if total == 0 {
		return ""
	}
	if d.state.Truncated {
		return "Showing first " + strconv.Itoa(len(d.state.Channels)) +
			" of " + strconv.Itoa(total) + " channels. Refine your search."
	}
	if d.Filters().Empty() {
		return ""
	}

	return strconv.Itoa(total) + " channels"
}

// regionLabels returns the selector labels, or nil while the region list and
// the current region are both unknown.
func (d *Discover) regionLabels() []string {
	labels := make([]string, 0, len(d.countries))
	for _, country := range d.countries {
		if name := strings.TrimSpace(country.Name); name != "" {
			labels = append(labels, name)
		}
	}
	if len(labels) > 0 {
		return labels
	}
	if d.state.Region != "" {
		return []string{d.regionLabel()}
	}

	return nil
}

// regionLabel returns the current region's display name, or its code before
// the region list loads.
func (d *Discover) regionLabel() string {
	if name := d.countryName(d.state.Region); name != "" {
		return name
	}

	return d.state.Region
}

// countryName resolves a region code through the published countries.
func (d *Discover) countryName(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	for _, country := range d.countries {
		if strings.EqualFold(country.Code, code) {
			return country.Name
		}
	}

	return ""
}

// countryCode resolves a selector label back to its region code.
func (d *Discover) countryCode(label string) (string, bool) {
	if code, ok := d.codeInCountries(label); ok {
		return code, true
	}
	if strings.EqualFold(label, d.state.Region) {
		return d.state.Region, true
	}

	return "", false
}

func (d *Discover) codeInCountries(label string) (string, bool) {
	for _, country := range d.countries {
		if strings.EqualFold(country.Name, label) {
			return country.Code, true
		}
	}

	return "", false
}

// sourceLabel returns the selector label for the current source filter.
func (d *Discover) sourceLabel() string {
	if d.sourceID == "" {
		return discoverAllSources
	}
	for _, source := range d.sources {
		if source.ID == d.sourceID {
			return source.Name
		}
	}

	return discoverAllSources
}

// facetID resolves a filter label back to its stable ID; "all" is empty.
func facetID(facets []DiscoverFacet, label string) string {
	for _, facet := range facets {
		if facet.Name == label {
			return facet.ID
		}
	}

	return ""
}

// facetExists reports whether id is a published source facet.
func facetExists(facets []DiscoverFacet, id string) bool {
	for _, facet := range facets {
		if facet.ID == id {
			return true
		}
	}

	return false
}

// defaultMessage returns the state's message or the page's own copy.
func (d *Discover) defaultMessage() string {
	if d.state.Message != "" {
		return d.state.Message
	}

	if !d.Filters().Empty() {
		return "No channels match this search or filter. Clear the filters, or add your own playlist."
	}

	switch d.state.Kind {
	case DiscoverLoading:
		if name := d.regionLabel(); name != "" {
			return "Fetching the community catalog for " + name + "."
		}

		return "Fetching the community catalog."
	case DiscoverError:
		return "mumu could not load the community catalog. Check your connection and retry, or add your own playlist."
	case DiscoverOffline:
		return "Showing saved channels. The catalog could not be refreshed."
	default:
		if name := d.regionLabel(); name != "" {
			return "The community catalog has no channels for " + name + " right now. Retry, or add your own playlist."
		}

		return "The community catalog has no channels right now. Retry, or add your own playlist."
	}
}
