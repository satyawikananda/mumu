package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// discoverAllCategories is the category rail's "no restriction" label.
	discoverAllCategories = "All channels"
	// discoverChooseCountry is the country chip before any country is chosen.
	discoverChooseCountry = "Choose a country"
	// discoverSearchFocus is the FocusBind value of the search input.
	discoverSearchFocus = "discover-search"
	// discoverRailWidth is the category rail's reference width at medium.
	discoverRailWidth float32 = 236
	// discoverGridFocus prefixes a grid tile's focus value, so a channel in the
	// grid cannot collide with the search list's own selection.
	discoverGridFocus = "cell:"
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
	// DiscoverNoCountry reports the approved first-launch state: no catalog
	// country is chosen yet, so no country playlist was fetched.
	DiscoverNoCountry
	// DiscoverDisabled reports that the community catalog is turned off.
	DiscoverDisabled
)

// DiscoverChannel is one channel tile or search row.
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

// DiscoverCategory is one category rail entry with its real match count.
type DiscoverCategory struct {
	Name  string
	Count int
}

// DiscoverFilters is the page's current narrowing. Zero values mean "all".
type DiscoverFilters struct {
	Query    string
	Category string
}

// Empty reports whether the filters narrow nothing.
func (f DiscoverFilters) Empty() bool {
	return strings.TrimSpace(f.Query) == "" && f.Category == ""
}

// DiscoverState is what the app publishes to the Discover page.
type DiscoverState struct {
	Kind     DiscoverKind
	Region   string
	Channels []DiscoverChannel
	// Message overrides the page's default text for the state.
	Message string
	// Total counts every channel that matched, before any cap or page.
	Total int
	// Truncated reports that Channels is a capped prefix of Total.
	Truncated bool
	// HasMore reports that more matches exist than Channels holds.
	HasMore bool
}

// Discover is the native Discover page. The app publishes the catalog state;
// the page dispatches intents through its callbacks and owns no network work.
type Discover struct {
	state      DiscoverState
	countries  []DiscoverCountry
	categories []DiscoverCategory
	logos      map[string]*ui.Bitmap
	logoColors map[string]ui.Color
	favorites  map[string]bool
	list       ui.ListState
	scroll     ui.ScrollState

	// focus is the desired grid keyboard target; it stays empty in search
	// mode, where the list owns focus.
	focus   string
	columns int
	// moreRequested keeps one scroll from asking for several batches.
	moreRequested bool
	// searchFocus is the FocusBind field: assigning discoverSearchFocus asks
	// the search input for the keyboard focus.
	searchFocus string

	// categoryList and categoryScroll keep the rail bounded and scrolled.
	categoryList   ui.ListState
	categoryScroll ui.ScrollState

	// country picker state. The list is local, bounded work over published
	// countries; it never fetches anything itself.
	pickerOpen     bool
	pickerQuery    string
	pickerSearch   ui.Handle
	pickerList     ui.ListState
	pickerSelected int
	countriesReady bool
	countriesError string

	query       string
	category    string
	selected    int
	selectedKey string
	notice      string

	// OnRegion asks the app to persist and load one catalog country.
	OnRegion func(code string)
	// OnRetry asks the app to load the catalog again.
	OnRetry func()
	// OnReloadCountries asks the app to reload the country list.
	OnReloadCountries func()
	// OnAddPlaylist sends the viewer to the playlist import surface.
	OnAddPlaylist func()
	// OnOpenSettings sends the viewer to Settings.
	OnOpenSettings func()
	// OnFilter reports a new narrowing; the app debounces and filters.
	OnFilter func(filters DiscoverFilters)
	// OnLoadMore asks the app for the next batch of browse results.
	OnLoadMore func()
	// OnSelect asks the app to open the channel with this key.
	OnSelect func(key string)
	// OnToggleFavorite adds or removes one channel from favorites.
	OnToggleFavorite func(key string)
}

// NewDiscover returns the approved first-launch state: no country chosen.
func NewDiscover() *Discover {
	d := &Discover{
		state:          DiscoverState{Kind: DiscoverNoCountry},
		selected:       -1,
		pickerSelected: -1,
	}
	d.list.Selected = &d.selected
	d.list.Label = d.rowLabel
	d.pickerList.Selected = &d.pickerSelected

	return d
}

// SetState shows state. The app always sends the region it means, including an
// empty one for the no-country state, so the page never guesses.
func (d *Discover) SetState(state DiscoverState) {
	if state.Total == 0 && len(state.Channels) > 0 {
		state.Total = len(state.Channels)
	}
	d.state = state
	d.selected = d.indexForKey(d.selectedKey)
	d.moreRequested = false
	d.settleFocus()
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

// Scrolled reports whether the catalog is scrolled away from its start.
func (d *Discover) Scrolled() bool { return d.scroll.Y > 1 }

// Region returns the selected catalog region code, empty for no country.
func (d *Discover) Region() string { return d.state.Region }

// Filters returns the page's current narrowing.
func (d *Discover) Filters() DiscoverFilters {
	return DiscoverFilters{
		Query:    strings.TrimSpace(d.query),
		Category: d.category,
	}
}

// SetCountries publishes the region list for the picker.
func (d *Discover) SetCountries(countries []DiscoverCountry) {
	d.countries = countries
	d.countriesReady = true
	d.countriesError = ""
}

// SetCountriesError reports a country-list failure the picker must explain.
func (d *Discover) SetCountriesError(message string) {
	d.countriesReady = false
	d.countriesError = strings.TrimSpace(message)
}

// Countries returns the published region list.
func (d *Discover) Countries() []DiscoverCountry { return d.countries }

// SetFacets publishes the category rail. A selection that no longer exists
// falls back to "All channels".
func (d *Discover) SetFacets(categories []DiscoverCategory) {
	d.categories = categories

	if d.category != "" && !categoryExists(categories, d.category) {
		d.category = ""
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

// SetLogoColors publishes bounded artwork tints keyed by logo URL.
func (d *Discover) SetLogoColors(colors map[string]ui.Color) {
	d.logoColors = colors
}

// SetFavorites publishes which channel keys are favorites. Rows and tiles read
// the map, so a favorite change never rebuilds the indexed catalog.
func (d *Discover) SetFavorites(favorites map[string]bool) {
	d.favorites = favorites
}

// Favorite reports whether key is a favorite, for the app and for tests.
func (d *Discover) Favorite(key string) bool { return d.favorites[key] }

// FocusSearch requests the search input's focus.
func (d *Discover) FocusSearch() { d.searchFocus = discoverSearchFocus }

// HasRows reports whether the page currently shows channels.
func (d *Discover) HasRows() bool { return len(d.state.Channels) > 0 }

// HasSelection reports whether a channel is the current keyboard target.
func (d *Discover) HasSelection() bool { return d.currentKey() != "" }

// ToggleSelectedFavorite toggles the current channel's favorite state.
func (d *Discover) ToggleSelectedFavorite() {
	key := d.currentKey()
	if key == "" || d.OnToggleFavorite == nil {
		return
	}
	d.OnToggleFavorite(key)
}

// View draws the page on the MyGo main thread.
func (d *Discover) View(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())
	d.columns = max(layout.DiscoverColumns, 1)
	d.settleFocus()
	d.requestMore()

	d.header(c, theme)
	d.filters(c, theme)
	d.body(c, theme, layout)
	d.countryPicker(c, theme)
}

// header draws the page heading and the Add Playlist action.
func (d *Discover) header(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
		pageHeading(c, "Discover").Grow(1)

		add := ui.ButtonBase(c).Height(30).Padding(7, 11).Radius(radiusChip).
			Border(1, theme.Border).FocusRingOffset(-2).Gap(6).
			Label("Add Playlist").Tooltip("Import a playlist or stream")
		if add.Hovered() {
			add = add.Background(theme.SurfaceHover)
		}
		add.Children(func() {
			ui.Icon(c, icons().plus).Size(14, 14).TextColor(theme.Text).AccessHidden(true)
			ui.Text(c, "Add Playlist").Font(bodyFont()).FontSize(textSize(theme, 13)).
				FontWeight(600).TextColor(theme.Text).SingleLine()
		})
		if add.Clicked() {
			d.addPlaylist()
		}
	})
}

// filters draws the search field and the country filter.
func (d *Discover) filters(c *ui.Context, theme *ui.Theme) {
	field := ui.SearchField(c, &d.query).Label("Search channels").
		FocusBind(&d.searchFocus, discoverSearchFocus)
	if field.Changed() {
		d.emitFilter()
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "Country").Font(bodyFont()).FontSize(textSize(theme, 12)).
			TextColor(theme.TextMuted).SingleLine()
		d.countryChip(c)

		ui.Spacer(c)

		if !d.Filters().Empty() {
			ui.Button(c, "Clear filters").OnClick(d.clearFilters)
		}
	})
}

// countryChip shows the chosen country, or the approved empty prompt.
func (d *Discover) countryChip(c *ui.Context) {
	label := d.regionLabel()
	if label == "" {
		label = discoverChooseCountry
	}

	Chip{
		Key:    "discover-country",
		Label:  label,
		Icon:   icons().globe,
		Active: d.state.Region != "",
		OnClick: func() {
			d.openCountryPicker()
		},
	}.View(c)
}

// body draws the state, with the browse grid or the search list.
func (d *Discover) body(c *ui.Context, theme *ui.Theme, layout Layout) {
	switch d.state.Kind {
	case DiscoverLoading:
		d.skeleton(c, theme, layout)
	case DiscoverError:
		pageStatus(c, Status{
			Kind:     StatusError,
			Title:    "Channels unavailable",
			Message:  d.defaultMessage(),
			Action:   "Retry",
			OnAction: d.retry,
		})
	case DiscoverNoCountry:
		d.noCountry(c, theme, layout)
	case DiscoverDisabled:
		d.disabled(c, theme)
	case DiscoverOffline:
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
		d.results(c, theme, layout)
	case DiscoverReady:
		d.results(c, theme, layout)
	default:
		d.empty(c)
	}
}

// results draws the category rail or chips beside the channel results.
func (d *Discover) results(c *ui.Context, theme *ui.Theme, layout Layout) {
	showCategories := layout.CategoryColumn && d.showCategories()

	body := ui.Grid(c).Grow(1).FillWidth().Gap(20)
	if showCategories {
		body = body.ColumnTracks(discoverBodyTracks(layout)...)
	} else {
		body = body.ColumnTracks(ui.Fr(1))
	}

	body.Children(func() {
		if showCategories {
			d.categoryColumn(c, theme)
		}

		ui.Column(c).Gap(12).Children(func() {
			if !layout.CategoryColumn && d.showCategories() {
				d.categoryChips(c)
			}
			d.heading(c, theme)

			if d.state.Kind == DiscoverOffline {
				d.offlineNote(c, theme)
			}
			if d.notice != "" {
				ui.Text(c, d.notice).FontSize(textSize(theme, 12)).
					TextColor(theme.TextMuted).SingleLine()
			}

			if d.searchMode() {
				d.searchList(c)

				return
			}
			if len(d.state.Channels) == 0 {
				d.empty(c)

				return
			}

			d.grid(c, layout)
		})
	})
}

// discoverBodyTracks splits the body between the rail and the channels. The
// wide reference keeps the rail to a third so the grid stays dominant; smaller
// windows keep the compact rail.
func discoverBodyTracks(layout Layout) []ui.Track {
	if layout.Breakpoint == BreakpointWide {
		return []ui.Track{ui.Fr(1), ui.Fr(2)}
	}

	return []ui.Track{ui.Fixed(discoverRailWidth), ui.Fr(1)}
}

// showCategories reports whether categories are meaningful right now. They
// wait for a country choice, as the approved no-country frame shows.
func (d *Discover) showCategories() bool {
	return len(d.categories) > 0 && (d.state.Region != "" || d.searchMode())
}

// skeleton shows the grid's shape while the first catalog page loads. MyGo has
// no skeleton widget, so it uses the documented Box and Loop pattern.
func (d *Discover) skeleton(c *ui.Context, theme *ui.Theme, layout Layout) {
	columns := max(layout.DiscoverColumns, 1)

	ui.Grid(c).Grow(1).FillWidth().Columns(columns).Gap(12).
		Label("Loading channels").Busy(true).Children(func() {
		for range columns * 2 {
			card := ui.Box(c).Height(layout.TileHeight).Radius(radiusTile).
				Background(theme.Surface).ClipInset(0, 0, 0, 0, radiusTile)
			pulse := card.Loop("discover-skeleton", 1600*time.Millisecond, ui.Bounce(ui.EaseInOut))
			card.Opacity(0.45 + 0.35*pulse)
			card.Children(func() {
				ui.Box(c).Absolute().Left(15).Top(16).Right(64).Height(14).Radius(7).
					Background(theme.Border).AccessHidden(true)
				ui.Box(c).Absolute().Left(15).Bottom(16).Width(48).Height(12).Radius(6).
					Background(theme.Border).AccessHidden(true)
			})
		}
	})
}

// categoryColumn draws the bounded category rail with real counts.
func (d *Discover) categoryColumn(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Text(c, "CATEGORIES").Font(bodyFont()).FontSize(textSize(theme, 11)).
			FontWeight(700).LetterSpacing(1.4).TextColor(theme.TextMuted).SingleLine()

		ui.List(c, &d.categoryList, len(d.categories)+1).Grow(1).FillWidth().
			Label("Categories").
			TrackScroll(&d.categoryScroll).
			Rows(func(row ui.ListRow) {
				d.categoryRow(c, theme, row)
			})
	})
}

// categoryRow draws one rail entry: All channels, then each real category.
func (d *Discover) categoryRow(c *ui.Context, theme *ui.Theme, row ui.ListRow) {
	name, count, active := discoverAllCategories, d.state.Total, d.category == ""
	if row.Index > 0 {
		category := d.categories[row.Index-1]
		name, count, active = category.Name, category.Count, d.category == category.Name
	}

	d.categoryButton(c, theme, name, count, active)
}

// categoryChips draws the compact form of the rail.
func (d *Discover) categoryChips(c *ui.Context) {
	ui.ScrollHorizontal(c).FillWidth().Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			Chip{
				Key:    "discover-category-all",
				Label:  discoverAllCategories,
				Active: d.category == "",
				OnClick: func() {
					d.setCategory("")
				},
			}.View(c)
			for _, category := range d.categories {
				name := category.Name
				Chip{
					Key:    "discover-category:" + name,
					Label:  name,
					Active: d.category == name,
					OnClick: func() {
						d.setCategory(name)
					},
				}.View(c)
			}
		})
	})
}

// categoryButton is one accessible rail row with its count.
func (d *Discover) categoryButton(c *ui.Context, theme *ui.Theme, name string, count int, active bool) {
	fill := ui.Transparent
	if active {
		if theme.Dark {
			fill = ui.RGBA(255, 255, 255, 0.07)
		} else {
			fill = ui.RGBA(0, 0, 0, 0.05)
		}
	}

	button := ui.ButtonBase(c).FillWidth().Height(34).Padding(9, 10).Radius(8).
		Background(fill).Gap(8).AlignItems(ui.Center).FocusRingOffset(-2).
		AccessSelected(active).Label(categoryLabel(name, count))
	if button.Hovered() && !active {
		button = button.Background(ui.RGBA(255, 255, 255, 0.04))
	}
	button.Children(func() {
		ui.Text(c, name).Font(bodyFont()).FontSize(textSize(theme, 14)).
			FontWeight(500).TextColor(theme.TextMuted).Grow(1).SingleLine()
		ui.Text(c, strconv.Itoa(count)).Font(bodyFont()).FontSize(textSize(theme, 12)).
			TextColor(theme.TextMuted).SingleLine()
	})
	if button.Clicked() {
		d.setCategory(name)
	}
}

// heading draws the results title and its truthful subtitle.
func (d *Discover) heading(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		ui.Text(c, d.resultsTitle()).Font(displayFont()).FontSize(textSize(theme, 17)).
			FontWeight(600).TextColor(theme.Text).SingleLine()
		ui.Text(c, d.resultsSubtitle()).Font(bodyFont()).FontSize(textSize(theme, 12)).
			TextColor(theme.TextMuted).SingleLine()
	})
}

// grid draws the loaded matches and scrolls for more. Scroll position decides
// when the next batch is asked for, so browsing never paginates.
func (d *Discover) grid(c *ui.Context, layout Layout) {
	ui.Scroll(c).Grow(1).FillWidth().TrackScroll(&d.scroll).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			ChannelGrid{
				Key:     "discover-grid",
				Tiles:   d.tiles(),
				Columns: layout.DiscoverColumns,
				Height:  layout.TileHeight,
			}.View(c)

			if d.state.HasMore {
				d.skeletonRow(c, layout)
			}
		})
	})
}

// tiles maps the loaded rows to shared channel tiles.
func (d *Discover) tiles() []ChannelTile {
	current := d.currentKey()

	tiles := make([]ChannelTile, 0, len(d.state.Channels))
	for _, channel := range d.state.Channels {
		key := channel.Key
		tiles = append(tiles, ChannelTile{
			Key:        "discover-tile:" + key,
			Name:       channel.Name,
			Meta:       d.channelMeta(channel),
			Live:       channel.Live,
			LiveCorner: true,
			Art:        d.art(channel),
			FocusField: &d.focus,
			FocusValue: discoverGridFocus + key,
			Selected:   key == current,
			OnActivate: func() { d.selectKey(key) },
		})
	}

	return tiles
}

// skeletonRow shows the grid's shape while the next batch arrives.
func (d *Discover) skeletonRow(c *ui.Context, layout Layout) {
	columns := max(layout.DiscoverColumns, 1)
	theme := c.Theme()

	ui.Grid(c).FillWidth().Columns(columns).Gap(12).Children(func() {
		for range columns {
			card := ui.Box(c).Height(layout.TileHeight).Radius(radiusTile).
				Background(theme.Surface).ClipInset(0, 0, 0, 0, radiusTile)
			pulse := card.Loop("discover-skeleton-more", 1600*time.Millisecond, ui.Bounce(ui.EaseInOut))
			card.Opacity(0.45 + 0.35*pulse)
			card.AccessHidden(true)
		}
	})
}

// requestMore asks for the next batch when the grid is near its end.
func (d *Discover) requestMore() {
	if d.searchMode() || !d.state.HasMore || d.moreRequested || d.OnLoadMore == nil {
		return
	}
	if d.scroll.MaxY <= 0 || d.scroll.Y < d.scroll.MaxY-400 {
		return
	}

	d.moreRequested = true
	d.OnLoadMore()
}

// searchList draws the virtualized results for a query.
func (d *Discover) searchList(c *ui.Context) {
	if len(d.state.Channels) == 0 {
		d.empty(c)

		return
	}

	list := ui.List(c, &d.list, len(d.state.Channels)).
		Grow(1).FillWidth().
		Label("Channels").
		ItemKey(func(i int) any { return d.state.Channels[i].Key }).
		TrackScroll(&d.scroll).
		Rows(func(row ui.ListRow) {
			d.searchRow(row)
		})
	if list.Submitted() {
		d.submit()
	}
}

// searchRow draws one channel with the shared dense row.
func (d *Discover) searchRow(row ui.ListRow) {
	channel := d.state.Channels[row.Index]
	favorite := d.favorites[channel.Key]
	label := "Add to Favorites"
	if favorite {
		label = "Remove from Favorites"
	}

	ChannelRow{
		Name:           channel.Name,
		Meta:           d.channelMeta(channel),
		Live:           channel.Live,
		Art:            d.art(channel),
		Action:         label,
		ActionSelected: favorite,
		OnAction:       func() { d.toggleFavorite(channel.Key) },
	}.ListView(row.Context, row)
}

// noCountry draws the approved first-launch state with explicit actions.
func (d *Discover) noCountry(c *ui.Context, theme *ui.Theme, layout Layout) {
	message := "Use the country filter above to discover live channels.\nNo country is selected yet."
	if custom := strings.TrimSpace(d.state.Message); custom != "" {
		message = custom
	}

	body := ui.Grid(c).Grow(1).FillWidth().Gap(20)
	if layout.CategoryColumn {
		body = body.ColumnTracks(discoverBodyTracks(layout)...)
	} else {
		body = body.ColumnTracks(ui.Fr(1))
	}

	body.Children(func() {
		if layout.CategoryColumn {
			ui.Column(c).Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "CATEGORIES").Font(bodyFont()).FontSize(textSize(theme, 11)).
					FontWeight(700).LetterSpacing(1.4).TextColor(theme.TextMuted).SingleLine()
				ui.Text(c, "Choose a country to see available categories.").
					Font(bodyFont()).FontSize(textSize(theme, 13)).
					TextColor(theme.TextMuted)
			})
		}

		ui.Column(c).Gap(12).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				ui.Text(c, "Channels").Font(displayFont()).FontSize(textSize(theme, 17)).
					FontWeight(600).TextColor(theme.Text).SingleLine()
				ui.Text(c, "Select a country to load channels from this catalog.").
					Font(bodyFont()).FontSize(textSize(theme, 13)).
					TextColor(theme.TextMuted).SingleLine()
			})

			d.prompt(c, theme, icons().globe, "Choose a country to get started",
				message,
				"Select a country",
				func() { d.openCountryPicker() },
				"", nil,
			)
		})
	})
}

// disabled draws the community-catalog-off state with a real way forward.
func (d *Discover) disabled(c *ui.Context, theme *ui.Theme) {
	message := d.state.Message
	if message == "" {
		message = "Turn the community catalog back on in Settings, or add your own playlist."
	}

	d.prompt(c, theme, icons().sliders, "Community catalog is off",
		message,
		"Open Settings", d.openSettings,
		"", nil,
	)
}

// offlineNote explains cached results above them.
func (d *Discover) offlineNote(c *ui.Context, theme *ui.Theme) {
	message := d.state.Message
	if message == "" {
		message = "Showing saved channels. The catalog could not be refreshed."
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(8, 12).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Icon(c, icons().alert).Size(14, 14).TextColor(theme.Warning).AccessHidden(true)
		ui.Text(c, message).Font(bodyFont()).FontSize(textSize(theme, 12)).
			TextColor(theme.TextMuted).Grow(1).SingleLine()
		ui.Button(c, "Retry").OnClick(d.retry)
	})
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

// prompt draws a centered icon, title, guidance and up to two actions.
func (d *Discover) prompt(
	c *ui.Context,
	theme *ui.Theme,
	icon *ui.SVG,
	title, message, primary string,
	onPrimary func(),
	secondary string,
	onSecondary func(),
) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(20).Padding(0, 0, 48, 0).Children(func() {
		ui.Box(c).Size(64, 64).Radius(32).Background(theme.Surface).Center().Children(func() {
			ui.Icon(c, icon).Size(26, 26).TextColor(theme.TextMuted).AccessHidden(true)
		})

		ui.Column(c).Center().Gap(8).Children(func() {
			ui.Text(c, title).Font(displayFont()).FontSize(textSize(theme, 26)).
				FontWeight(600).LetterSpacing(-0.6).TextColor(theme.Text).
				TextAlign(ui.Center).MaxWidth(560).MaxLines(2)
			ui.Text(c, message).Font(bodyFont()).FontSize(textSize(theme, 14)).
				TextColor(theme.TextMuted).TextAlign(ui.Center).MaxWidth(520)
		})

		ui.Row(c).Gap(10).Children(func() {
			ui.PrimaryButton(c, primary).OnClick(onPrimary)
			if secondary != "" && onSecondary != nil {
				ui.Button(c, secondary).OnClick(onSecondary)
			}
		})
	})
}

// countryPicker draws the bounded, searchable country modal.
func (d *Discover) countryPicker(c *ui.Context, theme *ui.Theme) {
	if !d.pickerOpen {
		return
	}

	matches := d.countryMatches()

	panel := ui.DialogBase(c, &d.pickerOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(420).MaxWidthPercent(92).Padding(18).Gap(12).Radius(12).
			Background(theme.Background).Border(1, theme.Border).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.35)).
			Label(discoverChooseCountry)

		ui.Text(c, discoverChooseCountry).Font(displayFont()).
			FontSize(textSize(theme, 20)).FontWeight(600).TextColor(theme.Text)

		switch {
		case d.countriesError != "":
			ui.Text(c, d.countriesError).Font(bodyFont()).FontSize(textSize(theme, 13)).
				TextColor(theme.Danger)
			ui.Button(c, "Retry").OnClick(d.reloadCountries)
		case !d.countriesReady:
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Spinner(c).Size(16, 16).Label("Loading countries")
				ui.Text(c, "Loading countries…").Font(bodyFont()).
					FontSize(textSize(theme, 13)).TextColor(theme.TextMuted)
			})
		default:
			ui.SearchField(c, &d.pickerQuery).Label("Search countries").
				Bind(&d.pickerSearch).AutoFocus()

			if len(matches) == 0 {
				ui.Text(c, "No countries match your search.").Font(bodyFont()).
					FontSize(textSize(theme, 13)).TextColor(theme.TextMuted)
			} else {
				list := ui.List(c, &d.pickerList, len(matches)).MaxHeight(360).
					Label("Countries").
					ItemKey(func(i int) any { return matches[i].Code }).
					Rows(func(row ui.ListRow) {
						d.countryRow(c, theme, row, matches)
					})
				if list.Submitted() {
					d.pickCountry(matches)
				}
			}

			ui.Row(c).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() { d.pickerOpen = false })
			})
		}
	})
	if panel.Valid() && panel.Dismissed() {
		d.closeCountryPicker()
	}
}

// countryRow draws one selectable country with its code.
func (d *Discover) countryRow(
	c *ui.Context,
	theme *ui.Theme,
	row ui.ListRow,
	matches []DiscoverCountry,
) {
	country := matches[row.Index]
	active := strings.EqualFold(country.Code, d.state.Region)

	button := ui.ButtonBase(c).FillWidth().Height(36).Padding(8, 10).Radius(8).
		Gap(8).AlignItems(ui.Center).FocusRingOffset(-2).
		AccessSelected(active).Label(countryLabel(country))
	if button.Hovered() {
		button = button.Background(ui.RGBA(255, 255, 255, 0.05))
	}
	button.Children(func() {
		ui.Text(c, country.Name).Font(bodyFont()).FontSize(textSize(theme, 14)).
			TextColor(theme.Text).Grow(1).SingleLine()
		ui.Text(c, strings.ToUpper(country.Code)).Font(bodyFont()).
			FontSize(textSize(theme, 12)).TextColor(theme.TextMuted).SingleLine()
	})
	if button.Clicked() {
		d.pickerSelected = row.Index
		d.pickCountry(matches)
	}
}

// countryMatches narrows the published countries by the picker query.
func (d *Discover) countryMatches() []DiscoverCountry {
	query := strings.ToLower(strings.TrimSpace(d.pickerQuery))
	if query == "" {
		return d.countries
	}

	matches := make([]DiscoverCountry, 0, len(d.countries))
	for _, country := range d.countries {
		name := strings.ToLower(country.Name)
		code := strings.ToLower(country.Code)
		if strings.Contains(name, query) || strings.Contains(code, query) {
			matches = append(matches, country)
		}
	}

	return matches
}

// pickCountry closes the picker and reports the chosen code.
func (d *Discover) pickCountry(matches []DiscoverCountry) {
	index := d.pickerSelected
	if index < 0 || index >= len(matches) {
		return
	}

	code := matches[index].Code
	d.pickerOpen = false
	d.pickerSelected = -1
	if code == "" {
		return
	}

	d.state.Region = code
	d.selectedKey = ""
	d.selected = -1
	if d.OnRegion != nil {
		d.OnRegion(code)
	}
}

// openCountryPicker shows the picker and clears its local query.
func (d *Discover) openCountryPicker() {
	d.pickerOpen = true
	d.pickerQuery = ""
	d.pickerSelected = -1
	d.pickerList.Selected = &d.pickerSelected
}

// closeCountryPicker hides the picker and drops its local query.
func (d *Discover) closeCountryPicker() {
	d.pickerOpen = false
	d.pickerQuery = ""
}

// reloadCountries dispatches the country-list reload intent.
func (d *Discover) reloadCountries() {
	if d.OnReloadCountries != nil {
		d.OnReloadCountries()
	}
}

// submit opens the selected search row.
func (d *Discover) submit() {
	if d.selected < 0 || d.selected >= len(d.state.Channels) {
		return
	}
	d.selectKey(d.state.Channels[d.selected].Key)
}

// selectKey opens one channel and remembers it as the target.
func (d *Discover) selectKey(key string) {
	if key == "" {
		return
	}
	d.selectedKey = key
	if d.OnSelect != nil {
		d.OnSelect(key)
	}
}

// toggleFavorite toggles one row's favorite state.
func (d *Discover) toggleFavorite(key string) {
	if key == "" || d.OnToggleFavorite == nil {
		return
	}
	d.OnToggleFavorite(key)
}

// retry dispatches the retry intent.
func (d *Discover) retry() {
	if d.OnRetry != nil {
		d.OnRetry()
	}
}

// addPlaylist dispatches the import intent.
func (d *Discover) addPlaylist() {
	if d.OnAddPlaylist != nil {
		d.OnAddPlaylist()
	}
}

// openSettings dispatches the Settings intent.
func (d *Discover) openSettings() {
	if d.OnOpenSettings != nil {
		d.OnOpenSettings()
	}
}

// setCategory narrows by category and reports it.
func (d *Discover) setCategory(category string) {
	if category == discoverAllCategories {
		category = ""
	}
	if category == d.category {
		return
	}
	d.category = category
	d.emitFilter()
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
	d.emitFilter()
}

// searchMode reports whether the page uses the virtualized row list.
func (d *Discover) searchMode() bool {
	return strings.TrimSpace(d.query) != ""
}

// MoveTile moves the grid cursor by one row or column. It runs on the UI
// thread from the shortcut registry, after the page recorded its columns.
func (d *Discover) MoveTile(dx, dy int) {
	total := len(d.state.Channels)
	if d.searchMode() || total == 0 {
		return
	}

	columns := max(d.columns, 1)
	index := d.indexForKey(d.currentKey())
	if index < 0 {
		index = 0
	}

	row, column := index/columns, index%columns
	column = min(max(column+dx, 0), columns-1)
	row = min(max(row+dy, 0), (total-1)/columns)
	next := min(row*columns+column, total-1)

	d.selected = next
	d.selectedKey = d.state.Channels[next].Key
	d.focus = discoverGridFocus + d.selectedKey
}

// currentKey returns the channel the keyboard targets, empty for none.
func (d *Discover) currentKey() string {
	if d.searchMode() {
		if d.selected >= 0 && d.selected < len(d.state.Channels) {
			return d.state.Channels[d.selected].Key
		}

		return ""
	}
	if key := d.gridKey(); key != "" && d.indexForKey(key) >= 0 {
		return key
	}
	if d.indexForKey(d.selectedKey) >= 0 {
		return d.selectedKey
	}

	return ""
}

// gridKey returns the channel key behind the grid focus value.
func (d *Discover) gridKey() string {
	key, ok := strings.CutPrefix(d.focus, discoverGridFocus)
	if !ok {
		return ""
	}

	return key
}

// settleFocus drops a grid target the loaded rows no longer show. It never
// claims the keyboard on its own: the search field or a click owns the first
// focus, and the arrow keys move the grid cursor after that.
func (d *Discover) settleFocus() {
	if d.searchMode() || len(d.state.Channels) == 0 {
		d.focus = ""

		return
	}
	if key := d.gridKey(); key != "" && d.indexForKey(key) >= 0 {
		d.selectedKey = key

		return
	}
	d.focus = ""
}

// rowLabel names a search row for assistive technology and type-select.
func (d *Discover) rowLabel(i int) string {
	if i < 0 || i >= len(d.state.Channels) {
		return ""
	}

	return channelLabel(d.state.Channels[i].Name, d.channelMeta(d.state.Channels[i]), d.state.Channels[i].Live)
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

// art returns one channel's published artwork and monogram fallback.
func (d *Discover) art(channel DiscoverChannel) Artwork {
	url := channel.LogoURL

	return Artwork{
		Bitmap:   d.logos[url],
		Color:    d.logoColors[url],
		Monogram: monogram(channel.Name),
	}
}

// resultsTitle names the current result set.
func (d *Discover) resultsTitle() string {
	if d.searchMode() {
		return "Search results"
	}
	if name := d.countryName(d.state.Region); name != "" {
		return name + " channels"
	}

	return "Channels"
}

// resultsSubtitle reports the real count, view and source.
func (d *Discover) resultsSubtitle() string {
	total := d.state.Total
	if total == 0 {
		total = len(d.state.Channels)
	}

	view := "grid view"
	if d.searchMode() {
		view = "list view"
	}

	parts := []string{resultCount(total), view}
	if d.state.Truncated {
		parts = append(parts, "first "+strconv.Itoa(len(d.state.Channels))+" shown")
	}

	return strings.Join(parts, " · ")
}

// resultCount names a number of results.
func resultCount(n int) string {
	if n == 1 {
		return "1 result"
	}

	return strconv.Itoa(n) + " results"
}

// categoryLabel is a rail row's accessible name.
func categoryLabel(name string, count int) string {
	if count == 1 {
		return name + ", 1 channel"
	}

	return name + ", " + strconv.Itoa(count) + " channels"
}

// countryLabel is a country row's accessible name.
func countryLabel(country DiscoverCountry) string {
	name := strings.TrimSpace(country.Name)
	code := strings.ToUpper(strings.TrimSpace(country.Code))
	if code == "" {
		return name
	}
	if name == "" {
		return code
	}

	return name + ", " + code
}

// categoryExists reports whether categories carries name.
func categoryExists(categories []DiscoverCategory, name string) bool {
	for _, category := range categories {
		if category.Name == name {
			return true
		}
	}

	return false
}

// regionLabel returns the current region's display name, or its code before
// the region list loads.
func (d *Discover) regionLabel() string {
	if name := d.countryName(d.state.Region); name != "" {
		return name
	}

	return strings.ToUpper(strings.TrimSpace(d.state.Region))
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
	case DiscoverNoCountry:
		return "Choose a country to discover live channels."
	default:
		if name := d.regionLabel(); name != "" {
			return "The community catalog has no channels for " + name + " right now. Retry, or add your own playlist."
		}

		return "The community catalog has no channels right now. Retry, or add your own playlist."
	}
}
