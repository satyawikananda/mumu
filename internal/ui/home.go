package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// HomeKind is the state the Home feed shows.
type HomeKind int

const (
	// HomeLoading reports a pending library read.
	HomeLoading HomeKind = iota + 1
	// HomeEmpty reports nothing to resume and no saved channels.
	HomeEmpty
	// HomeResume offers the last playable channel without autoplay.
	HomeResume
	// HomeActive reports a session that is still playing.
	HomeActive
	// HomeFailed reports that the saved library could not be read.
	HomeFailed
)

// HomeRail names one Home rail for the See all intent.
type HomeRail int

const (
	// HomeRailRecent is the Recently watched rail.
	HomeRailRecent HomeRail = iota + 1
	// HomeRailFavorites is the Favorites rail.
	HomeRailFavorites
)

// HomeChannel is one channel on the Home feed.
//
// Key is the stable source-scoped identity shared with Discover, Watch and
// Library. Country is a resolved region name, Quality a real stream variant
// label; an absent value is never replaced by a placeholder.
type HomeChannel struct {
	Key           string
	Name          string
	SourceName    string
	Group         string
	Country       string
	Quality       string
	LogoURL       string
	Live          bool
	Favorite      bool
	Playable      bool
	SourceEnabled bool
	PlayedAt      time.Time
}

// HomeState is what the app publishes to the Home feed.
type HomeState struct {
	Kind HomeKind
	// Hero is the valid last-watched channel, or the active session.
	Hero HomeChannel
	// Recent and Favorites are the rail cards, already capped by the app.
	Recent    []HomeChannel
	Favorites []HomeChannel
	// RecentCount and FavoriteCount are the full saved counts.
	RecentCount   int
	FavoriteCount int
	// Message overrides the page's default text for the state.
	Message string
}

// Home is the landing screen when a valid recent channel exists. It never
// creates a player or starts playback by itself.
type Home struct {
	state      HomeState
	logos      map[string]*ui.Bitmap
	logoColors map[string]ui.Color

	// focus is the desired keyboard focus target. values are unique per rail,
	// so one channel may appear in both rails without a duplicate binding.
	focus string
	// visible is the committed focus target read during the last build.
	visible string
	// scroll keeps the page offset so the top bar can follow it.
	scroll ui.ScrollState
	// now keeps relative timestamps deterministic in tests.
	now func() time.Time

	// OnResume starts the offered channel after an explicit click.
	OnResume func()
	// OnWatch opens the active Watch screen.
	OnWatch func()
	// OnPlay opens one Home card by its stable key.
	OnPlay func(key string)
	// OnBrowse sends the viewer to Discover.
	OnBrowse func()
	// OnAddPlaylist opens the Library import sheet.
	OnAddPlaylist func()
	// OnRetry asks the app to read the saved library again.
	OnRetry func()
	// OnSeeAll opens the full Library section behind a rail.
	OnSeeAll func(rail HomeRail)
}

// Home focus targets. The rails prefix their channel keys so a channel saved
// twice cannot bind the same value twice.
const (
	homeFocusResume     = "resume"
	homeFocusOther      = "other"
	homeFocusRecentBase = "recent:"
	homeFocusFavBase    = "favorite:"
)

// NewHome returns an empty Home screen.
func NewHome() *Home {
	return &Home{
		state: HomeState{Kind: HomeEmpty},
		focus: homeFocusResume,
		now:   time.Now,
	}
}

// SetState shows state.
func (h *Home) SetState(state HomeState) {
	h.state = state
}

// State returns what Home shows, for the app and tests.
func (h *Home) State() HomeState { return h.state }

// SetLogos shows decoded channel logos by URL.
func (h *Home) SetLogos(logos map[string]*ui.Bitmap) { h.logos = logos }

// SetLogoColors shows the bounded artwork tints by URL.
func (h *Home) SetLogoColors(colors map[string]ui.Color) { h.logoColors = colors }

// HasHero reports whether the feed offers a hero channel.
func (h *Home) HasHero() bool {
	return (h.state.Kind == HomeResume || h.state.Kind == HomeActive) && h.state.Hero.Key != ""
}

// OnCardFocus reports whether keyboard focus is on a rail card.
func (h *Home) OnCardFocus() bool {
	return h.visible != "" && h.visible != homeFocusResume && h.visible != homeFocusOther
}

// FocusWithinRail reports whether ←/→ can move focus.
func (h *Home) FocusWithinRail() bool {
	return strings.HasPrefix(h.visible, homeFocusRecentBase) ||
		strings.HasPrefix(h.visible, homeFocusFavBase)
}

// CanActivate reports whether Enter has a target.
func (h *Home) CanActivate() bool {
	return h.HasHero() || len(h.state.Recent) > 0 || len(h.state.Favorites) > 0
}

// Resume dispatches the explicit resume intent.
func (h *Home) Resume() {
	if h.OnResume != nil {
		h.OnResume()
	}
}

// Activate runs Enter's target: the focused card, or the hero action when no
// card holds focus.
func (h *Home) Activate() {
	switch {
	case strings.HasPrefix(h.visible, homeFocusRecentBase):
		h.playChannel(strings.TrimPrefix(h.visible, homeFocusRecentBase))
	case strings.HasPrefix(h.visible, homeFocusFavBase):
		h.playChannel(strings.TrimPrefix(h.visible, homeFocusFavBase))
	case h.visible == homeFocusOther:
		h.browse()
	default:
		if h.state.Kind == HomeActive {
			if h.OnWatch != nil {
				h.OnWatch()
			}

			return
		}
		h.Resume()
	}
}

// MoveRail moves keyboard focus inside the focused rail. It runs on the UI
// thread from the shortcut registry, after the page recorded its focus.
func (h *Home) MoveRail(delta int) {
	switch {
	case strings.HasPrefix(h.visible, homeFocusRecentBase):
		h.moveInRail(h.state.Recent, homeFocusRecentBase, h.visible, delta)
	case strings.HasPrefix(h.visible, homeFocusFavBase):
		h.moveInRail(h.state.Favorites, homeFocusFavBase, h.visible, delta)
	}
}

// moveInRail moves the focus target one focusable card within cards.
func (h *Home) moveInRail(cards []HomeChannel, prefix, current string, delta int) {
	key := strings.TrimPrefix(current, prefix)
	index := -1
	for i, card := range cards {
		if card.Key == key {
			index = i

			break
		}
	}
	if index < 0 {
		return
	}

	for next := index + delta; next >= 0 && next < len(cards); next += delta {
		if cardFocusable(cards[next]) {
			h.focus = prefix + cards[next].Key

			return
		}
	}
}

// Scrolled reports whether the feed is scrolled away from its start.
func (h *Home) Scrolled() bool { return h.scroll.Y > 1 }

// View draws the Home feed on the MyGo main thread.
func (h *Home) View(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())
	t := theme
	width, height := c.Size()

	h.visible = ui.FocusedValue(c, &h.focus)
	h.settleFocus()

	ui.Column(c).Fill().Children(func() {
		h.backdrop(c, t, width, height, layout.Padding)

		ui.Scroll(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			TrackScroll(&h.scroll).Children(func() {
			ui.Column(c).FillWidth().Gap(20).Children(func() {
				switch h.state.Kind {
				case HomeLoading:
					pageStatus(c, Status{
						Kind:    StatusLoading,
						Title:   "Loading Home",
						Message: "Reading favorites and recently watched channels on this Mac.",
					})
				case HomeFailed:
					message := strings.TrimSpace(h.state.Message)
					if message == "" {
						message = ProductName + " could not read your saved channels. Retry, or browse Discover."
					}
					pageStatus(c, Status{
						Kind:     StatusError,
						Title:    "Home unavailable",
						Message:  message,
						Action:   "Retry",
						OnAction: h.retry,
					})
				case HomeEmpty:
					h.empty(c, t)
				default:
					h.feed(c, t, layout)
				}
			})
		})
	})
}

// backdrop draws the synthesized art layer behind the feed from the hero
// channel's own tint: a dark gradient, a soft glow from the same color, and
// the fade into the page canvas. It bleeds past the page padding so the art
// reaches the window edges, and it never adds a brand-colored wash.
func (h *Home) backdrop(c *ui.Context, theme *ui.Theme, width, height, pad float32) {
	p := paletteFor(theme.Dark)
	art := artworkColor(h.heroArt())
	// Light appearance keeps the art as a faint tint so the dark text stays
	// readable; dark appearance lets the channel color lead.
	base := art
	glow := art.Mix(ui.Hex("#FFFFFF"), 0.28)
	glowAlpha := float32(0.22)
	glowMid := float32(0.06)
	if !theme.Dark {
		base = art.Mix(p.canvas, 0.86)
		glow = base
		glowAlpha, glowMid = 0.10, 0.03
	}

	layer := ui.Box(c).Absolute().
		Left(-pad).Right(-pad).Top(-pad/2 - chromeHeight).Bottom(-pad / 2).
		IgnoreInput(true).AccessHidden(true)
	layer.LinearGradient(ui.LinearGradient{
		Angle: 145,
		Oklab: true,
		Stops: []ui.GradientStop{
			{Color: base, At: 0},
			{Color: base.Mix(p.canvas, 0.62), At: 0.42},
			{Color: p.canvas, At: 1},
		},
	})

	spanWidth := width + pad*2
	spanHeight := height + pad + chromeHeight
	halo := ui.Box(c).Absolute().
		Size(max(spanWidth*0.78, 640), max(spanHeight*1.2, 620)).
		Right(-spanWidth * 0.06).Top(-spanHeight * 0.46).
		IgnoreInput(true).AccessHidden(true)
	halo.RadialGradient(ui.RadialGradient{
		Center: ui.Point{X: 0.5, Y: 0.5},
		Radius: max(spanWidth*0.46, 380),
		Stops: []ui.GradientStop{
			{Color: glow.Alpha(glowAlpha), At: 0},
			{Color: glow.Alpha(glowMid), At: 0.55},
			{Color: ui.Transparent, At: 1},
		},
	})

	ui.Box(c).Absolute().Left(-pad).Right(-pad).Bottom(-pad / 2).Height(spanHeight * 0.44).
		IgnoreInput(true).AccessHidden(true).
		LinearGradient(ui.LinearGradient{
			Angle: 180,
			Stops: []ui.GradientStop{
				{Color: p.canvas.Alpha(0), At: 0},
				{Color: p.canvas.Alpha(0.93), At: 0.55},
				{Color: p.canvas, At: 1},
			},
		})
}

// feed draws the hero and the rails.
func (h *Home) feed(c *ui.Context, theme *ui.Theme, layout Layout) {
	if h.HasHero() {
		h.hero(c, theme, layout)
	}

	if len(h.state.Recent) > 0 {
		Rail{
			Key:         "home-recent",
			Title:       "Recently watched",
			Count:       channelCount(h.state.RecentCount),
			SeeAllLabel: "See all recently watched",
			OnSeeAll:    func() { h.seeAll(HomeRailRecent) },
			Tiles:       h.tiles(h.state.Recent, homeFocusRecentBase),
		}.View(c)
	}

	if len(h.state.Favorites) > 0 {
		Rail{
			Key:         "home-favorites",
			Title:       "Favorites",
			Count:       channelCount(h.state.FavoriteCount),
			SeeAllLabel: "See all favorites",
			OnSeeAll:    func() { h.seeAll(HomeRailFavorites) },
			Tiles:       h.tiles(h.state.Favorites, homeFocusFavBase),
		}.View(c)
	}
}

// tiles maps Home cards to the shared rail tiles.
func (h *Home) tiles(cards []HomeChannel, prefix string) []ChannelTile {
	tiles := make([]ChannelTile, 0, len(cards))
	for _, card := range cards {
		key := card.Key
		tiles = append(tiles, ChannelTile{
			Key:        prefix + key,
			Name:       card.Name,
			Meta:       joinMeta(card.Group, card.SourceName),
			Live:       card.Live,
			Art:        h.art(card),
			FocusField: &h.focus,
			FocusValue: prefix + key,
			Disabled:   !card.Playable || !card.SourceEnabled,
			OnActivate: func() { h.playChannel(key) },
		})
	}

	return tiles
}

// hero draws the explicit resume or active-session action.
func (h *Home) hero(c *ui.Context, theme *ui.Theme, layout Layout) {
	hero := h.state.Hero
	active := h.state.Kind == HomeActive
	label := "LAST WATCHED"
	action := "Resume"
	if active {
		label = "NOW PLAYING"
		action = "Open Watch"
	}

	name := strings.TrimSpace(hero.Name)
	if name == "" {
		name = "Unknown channel"
	}
	width, _ := c.Size()

	ui.Column(c).Width(min(max(width*0.55, 380), 760)).Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(9).Children(func() {
			if hero.Live {
				Badge{Kind: BadgeLive, Text: "LIVE"}.View(c)
			}
			ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 10)).
				FontWeight(700).LetterSpacing(0.7).TextColor(theme.TextMuted).SingleLine()
		})

		ui.Text(c, name).Font(displayFont()).
			FontSize(textSize(theme, layout.HeroTitle)).FontWeight(700).
			TextColor(theme.Text).MaxLines(2).Ellipsis("…")

		if meta := h.heroMeta(hero); meta != "" {
			ui.Text(c, meta).Font(bodyFont()).FontSize(textSize(theme, 15)).
				TextColor(theme.TextMuted).MaxLines(2).Ellipsis("…")
		}

		ui.Row(c).Gap(10).Children(func() {
			primary := ui.PrimaryButton(c, action).
				FocusBind(&h.focus, homeFocusResume).OnClick(func() {
				if active {
					if h.OnWatch != nil {
						h.OnWatch()
					}

					return
				}
				h.Resume()
			})
			primary.Label(action)

			ui.Button(c, "Other channels").FocusBind(&h.focus, homeFocusOther).
				OnClick(h.browse)
		})
	})
}

// empty draws the explicit empty Home without a welcome wizard.
func (h *Home) empty(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().Padding(32).Gap(14).MaxWidth(560).Children(func() {
		ui.Text(c, "Nothing to watch yet").Font(displayFont()).
			FontSize(textSize(theme, 28)).FontWeight(700).TextColor(theme.Text)
		ui.Text(c, "Choose a channel in Discover, or add your own playlist. Nothing plays until you ask.").
			Font(bodyFont()).FontSize(textSize(theme, 14)).TextColor(theme.TextMuted)

		ui.Row(c).Gap(10).Children(func() {
			ui.PrimaryButton(c, "Browse Discover").OnClick(h.browse)
			ui.Button(c, "Add Playlist").OnClick(h.addPlaylist)
		})
	})
}

// heroMeta joins the real hero facts; an absent value is omitted.
func (h *Home) heroMeta(hero HomeChannel) string {
	parts := make([]string, 0, 4)
	for _, value := range []string{hero.Country, hero.Group, hero.Quality} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if !hero.PlayedAt.IsZero() {
		parts = append(parts, relativeTimeLong(h.now(), hero.PlayedAt))
	}

	return strings.Join(parts, " · ")
}

// settleFocus drops a desired target that no longer exists.
func (h *Home) settleFocus() {
	if !h.CanActivate() {
		h.focus = ""

		return
	}
	if h.focus == "" {
		h.focus = homeFocusResume

		return
	}
	switch {
	case h.focus == homeFocusResume, h.focus == homeFocusOther:
		if !h.HasHero() {
			h.focus = h.firstCardFocus()
		}
	case strings.HasPrefix(h.focus, homeFocusRecentBase):
		if !hasFocusableKey(h.state.Recent, strings.TrimPrefix(h.focus, homeFocusRecentBase)) {
			h.focus = h.firstCardFocus()
		}
	case strings.HasPrefix(h.focus, homeFocusFavBase):
		if !hasFocusableKey(h.state.Favorites, strings.TrimPrefix(h.focus, homeFocusFavBase)) {
			h.focus = h.firstCardFocus()
		}
	default:
		h.focus = h.firstCardFocus()
	}
}

// firstCardFocus returns the first focusable card, or the hero when none exists.
func (h *Home) firstCardFocus() string {
	for _, card := range h.state.Recent {
		if cardFocusable(card) {
			return homeFocusRecentBase + card.Key
		}
	}
	for _, card := range h.state.Favorites {
		if cardFocusable(card) {
			return homeFocusFavBase + card.Key
		}
	}
	if h.HasHero() {
		return homeFocusResume
	}

	return ""
}

// cardFocusable reports whether a card can take keyboard focus.
func cardFocusable(card HomeChannel) bool {
	return card.Key != "" && card.Playable && card.SourceEnabled
}

// hasFocusableKey reports whether cards contains a focusable card with key.
func hasFocusableKey(cards []HomeChannel, key string) bool {
	for _, card := range cards {
		if card.Key == key && cardFocusable(card) {
			return true
		}
	}

	return false
}

// heroArt returns the artwork the backdrop and hero use.
func (h *Home) heroArt() Artwork {
	return h.art(h.state.Hero)
}

// art returns one channel's published artwork.
func (h *Home) art(channel HomeChannel) Artwork {
	url := channel.LogoURL

	return Artwork{
		Bitmap:   h.logos[url],
		Color:    h.logoColors[url],
		Monogram: monogram(channel.Name),
	}
}

// channelCount names a number of channels.
func channelCount(n int) string {
	if n < 0 {
		n = 0
	}
	if n == 1 {
		return "1 channel"
	}

	return fmt.Sprintf("%d channels", n)
}

// joinMeta joins the non-empty parts of a tile's meta line.
func joinMeta(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}

	return strings.Join(kept, " · ")
}

// playChannel dispatches one card's open intent.
func (h *Home) playChannel(key string) {
	if key == "" || h.OnPlay == nil {
		return
	}
	h.OnPlay(key)
}

// browse dispatches the Discover intent.
func (h *Home) browse() {
	if h.OnBrowse != nil {
		h.OnBrowse()
	}
}

// addPlaylist dispatches the playlist import intent.
func (h *Home) addPlaylist() {
	if h.OnAddPlaylist != nil {
		h.OnAddPlaylist()
	}
}

// retry dispatches a library reload.
func (h *Home) retry() {
	if h.OnRetry != nil {
		h.OnRetry()
	}
}

// seeAll opens one Library section.
func (h *Home) seeAll(rail HomeRail) {
	if h.OnSeeAll != nil {
		h.OnSeeAll(rail)
	}
}
