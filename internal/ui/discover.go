package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
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
// playlist group.
type DiscoverChannel struct {
	Key      string
	Name     string
	Country  string
	Category string
	LogoURL  string
	Live     bool
}

// DiscoverCountry is one selectable catalog region.
type DiscoverCountry struct {
	Code string
	Name string
}

// DiscoverState is what the app publishes to the Discover page.
type DiscoverState struct {
	Kind     DiscoverKind
	Region   string
	Channels []DiscoverChannel
	// Message overrides the page's default text for the state.
	Message string
}

// Discover is the native Discover page. The app publishes the catalog state;
// the page dispatches intents through its callbacks and owns no network work.
type Discover struct {
	state     DiscoverState
	countries []DiscoverCountry
	logos     map[string]*ui.Bitmap
	list      ui.ListState
	selected  int
	notice    string

	// OnRegion asks the app to switch the catalog region.
	OnRegion func(code string)
	// OnRetry asks the app to load the catalog again.
	OnRetry func()
	// OnAddPlaylist sends the viewer to the playlist import surface.
	OnAddPlaylist func()
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
	d.state = state
	if d.selected >= len(state.Channels) {
		d.selected = -1
	}
}

// State returns what the page shows, for the app and for tests.
func (d *Discover) State() DiscoverState { return d.state }

// Region returns the selected catalog region code.
func (d *Discover) Region() string { return d.state.Region }

// SetCountries publishes the region list for the selector.
func (d *Discover) SetCountries(countries []DiscoverCountry) {
	d.countries = countries
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
	d.body(c, theme)
}

// header draws the page heading, region selector and Add Playlist.
func (d *Discover) header(c *ui.Context, theme *ui.Theme) {
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

// empty draws the empty state with its retry action.
func (d *Discover) empty(c *ui.Context) {
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

// channelList draws the virtualized channel rows.
func (d *Discover) channelList(c *ui.Context, theme *ui.Theme) {
	if d.notice != "" {
		ui.Text(c, d.notice).FontSize(theme.Rem(0.85)).TextColor(theme.TextMuted).SingleLine()
	}

	ui.List(c, &d.list, len(d.state.Channels), func(i int) {
		d.row(c, theme, i)
	}).
		Grow(1).
		Label("Channels").
		ItemKey(func(i int) any { return d.state.Channels[i].Key })
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

// retry dispatches the retry intent.
func (d *Discover) retry() {
	if d.OnRetry != nil {
		d.OnRetry()
	}
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

// channelMeta builds "Country · Category" from a row.
func (d *Discover) channelMeta(channel DiscoverChannel) string {
	parts := make([]string, 0, 2)

	if name := d.countryName(channel.Country); name != "" {
		parts = append(parts, name)
	}
	if category := strings.TrimSpace(channel.Category); category != "" {
		parts = append(parts, category)
	}

	return strings.Join(parts, " · ")
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

// defaultMessage returns the state's message or the page's own copy.
func (d *Discover) defaultMessage() string {
	if d.state.Message != "" {
		return d.state.Message
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
