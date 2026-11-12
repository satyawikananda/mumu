package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// ControlCenterCard is one context action above the player. The card owns
// its artwork and text; the action hint under a focused card belongs to the
// caller so the row keeps its height.
type ControlCenterCard struct {
	Key        string
	Label      string
	Title      string
	Meta       string
	Art        Artwork
	Icon       *ui.SVG
	Width      float32
	Height     float32
	AutoFocus  bool
	Disabled   bool
	OnActivate func()
}

// View draws the card.
func (cc ControlCenterCard) View(c *ui.Context) ui.Element {
	t := c.Theme()
	p := paletteFor(t.Dark)
	if cc.Key != "" {
		c = c.Key(cc.Key)
	}

	width := cc.Width
	if width <= 0 {
		width = 262
	}
	height := cc.Height
	if height <= 0 {
		height = 158
	}
	title := strings.TrimSpace(cc.Title)
	if title == "" {
		title = "Untitled"
	}
	label := strings.ToUpper(strings.TrimSpace(cc.Label))
	if label == "" {
		label = "OPEN"
	}
	textColor := ui.Hex("#FFFFFF")
	mutedColor := ui.RGBA(255, 255, 255, 0.58)
	if cc.Disabled {
		textColor = p.muted
		mutedColor = p.muted
	}

	card := ui.ButtonBase(c).Size(width, height).Radius(radiusTile).
		ClipInset(0, 0, 0, 0, radiusTile).FocusRingOffset(-2).
		LinearGradient(ui.LinearGradient{
			From:  artworkColor(c, cc.Art),
			To:    ui.Hex("#101014"),
			Angle: 135,
			Oklab: true,
		})
	card.Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(92).
			IgnoreInput(true).LinearGradient(ui.LinearGradient{
			From:  ui.RGBA(0, 0, 0, 0.18),
			To:    ui.Transparent,
			Angle: 180,
		})

		if cc.Icon != nil {
			ui.Icon(c, cc.Icon).Size(44, 44).TextColor(ui.RGBA(255, 255, 255, 0.34)).
				Absolute().Left(16).Top(18).AccessHidden(true)
		} else {
			art := strings.ToUpper(title)
			if fields := strings.Fields(title); len(fields) > 0 {
				art = strings.ToUpper(fields[0])
			}
			if len([]rune(art)) > 9 {
				art = artworkMonogram(cc.Art, title)
			}
			ui.Text(c, art).Font(displayFont()).FontSize(textSize(t, 28)).
				FontWeight(700).TextColor(ui.RGBA(255, 255, 255, 0.19)).
				SingleLine().Absolute().Left(16).Right(16).Top(18)
		}

		ui.Column(c).Absolute().Left(16).Right(16).Bottom(14).Gap(2).Children(func() {
			ui.Text(c, label).Font(bodyFont()).FontSize(textSize(t, 10)).
				FontWeight(700).LetterSpacing(0.7).TextColor(mutedColor).SingleLine()
			ui.Text(c, title).Font(bodyFont()).FontSize(textSize(t, 16)).
				FontWeight(600).TextColor(textColor).SingleLine()
			if meta := strings.TrimSpace(cc.Meta); meta != "" {
				ui.Text(c, meta).Font(bodyFont()).FontSize(textSize(t, 11)).
					TextColor(mutedColor).SingleLine()
			}
		})
	})

	accessible := label + ", " + title
	if meta := strings.TrimSpace(cc.Meta); meta != "" {
		accessible += ", " + meta
	}
	card = card.Label(accessible).Disabled(cc.Disabled).DimDisabled(false)
	if cc.AutoFocus {
		card = card.AutoFocus()
	}
	pose := ui.Pose{}
	if card.Hovered() && !cc.Disabled {
		pose = ui.Pose{Scale: 1.02}
	}
	card.AnimatePose(pose, hoverMotion)

	if card.Clicked() && !cc.Disabled && cc.OnActivate != nil {
		cc.OnActivate()
	}

	return card
}

// ControlCenterState is the real data behind the overlay. Counts and names
// come from the published Watch and Library states; nothing is fabricated.
type ControlCenterState struct {
	Resume         bool
	Active         bool
	Channel        string
	Source         string
	FavoriteCount  int
	FavoriteNames  []string
	PlaylistCount  int
	PlaylistNames  []string
	LibraryLoading bool
	LibraryFailed  bool
	PiPActive      bool
	Current        string
	Hints          []Hint
}

// ControlCenterActions are the overlay's intents.
type ControlCenterActions struct {
	Close     func()
	Home      func()
	Discover  func()
	Watch     func()
	PiP       func()
	Favorites func()
	Playlists func()
	Resume    func()
}

// ControlCenter is the native overlay opened above the current screen.
type ControlCenter struct {
	State   ControlCenterState
	Actions ControlCenterActions
}

type controlCenterItem struct {
	Label  string
	Title  string
	Meta   string
	Icon   *ui.SVG
	Action func()
	Hint   string
}

// View draws the overlay. It is built after the page so it paints above the
// retained video surface without changing the destination.
func (cc ControlCenter) View(c *ui.Context) {
	layout := layoutFor(c.Size())
	width, _ := c.Size()
	t := c.Theme()
	items := cc.items()
	available := width - 2*layout.Padding
	cardWidth := min(float32(262), (available-28)/3)
	if cardWidth < 160 {
		cardWidth = 160
	}

	ui.Overlay(c, func() {
		scrim := ui.Box(c).Absolute().Fill().Modal().
			LinearGradient(ui.LinearGradient{
				Angle: 180,
				Stops: []ui.GradientStop{
					{Color: ui.RGBA(10, 10, 12, 0), At: 0},
					{Color: ui.RGBA(10, 10, 12, 0.72), At: 0.32},
					{Color: ui.RGBA(10, 10, 12, 0.94), At: 1},
				},
			})
		scrim.Children(func() {
			panel := ui.Column(c).Absolute().Left(layout.Padding).Right(layout.Padding).
				Bottom(max(float32(24), layout.Padding/2)).Gap(14).
				Role(ui.RoleDialog).Label("Control Center")
			panel.Children(func() {
				ui.Text(c, "CONTROL CENTER").Font(bodyFont()).FontSize(textSize(t, 13)).
					FontWeight(700).LetterSpacing(1).TextColor(t.TextMuted).SingleLine()

				focusedHint := ""
				cards := ui.Row(c).Gap(14).FocusGroup(ui.Horizontal)
				cards.Children(func() {
					for _, item := range items {
						card := ControlCenterCard{
							Key:       "cc-" + item.Label,
							Label:     item.Label,
							Title:     item.Title,
							Meta:      item.Meta,
							Icon:      item.Icon,
							Width:     cardWidth,
							Height:    158,
							AutoFocus: item.Label == items[0].Label,
							Art: Artwork{
								Monogram: monogram(item.Title),
								Color:    paletteFor(t.Dark).surface2,
							},
							OnActivate: item.Action,
						}.View(c)
						if card.Focused() {
							focusedHint = item.Hint
						}
					}
				})
				FocusHint{Key: "⏎", Action: focusedHint, Visible: focusedHint != ""}.View(c)

				iconRow := ui.Row(c).FillWidth().Center().Gap(20).FocusGroup(ui.Horizontal)
				iconRow.Children(func() {
					cc.icon(c, icons().home, "Go to Home", cc.State.Current == destHome, cc.Actions.Home)
					cc.icon(c, icons().search, "Go to Discover", cc.State.Current == destDiscover, cc.Actions.Discover)
					cc.icon(c, icons().monitor, "Go to Watch", cc.State.Current == destWatch, cc.Actions.Watch)
					cc.icon(c, icons().pip, "Picture in Picture", cc.State.PiPActive, cc.Actions.PiP)
					cc.icon(c, icons().close, "Close Control Center", false, cc.Actions.Close)
				})

				HintBar{Hints: cc.State.Hints}.View(c)
			})

			if panel.PressedOutside() || panel.OverlayShortcut(0, ui.KeyEscape) {
				if cc.Actions.Close != nil {
					cc.Actions.Close()
				}
			}
		})
	})
}

// items builds the three context cards from real state.
func (cc ControlCenter) items() []controlCenterItem {
	items := make([]controlCenterItem, 0, 3)
	switch {
	case cc.State.Active:
		title := strings.TrimSpace(cc.State.Channel)
		if title == "" {
			title = "Watch"
		}
		items = append(items, controlCenterItem{
			Label: "WATCH", Title: title, Meta: strings.TrimSpace(cc.State.Source),
			Action: cc.Actions.Watch, Hint: "Open Watch",
		})
	case cc.State.Resume:
		title := strings.TrimSpace(cc.State.Channel)
		if title == "" {
			title = "Last channel"
		}
		items = append(items, controlCenterItem{
			Label: "RESUME", Title: title, Meta: strings.TrimSpace(cc.State.Source),
			Action: cc.Actions.Resume, Hint: "Resume",
		})
	default:
		items = append(items, controlCenterItem{
			Label: "BROWSE", Title: "Discover", Meta: "Community catalog",
			Icon: icons().compass, Action: cc.Actions.Discover, Hint: "Open Discover",
		})
	}

	favorites := "Favorites"
	favoriteMeta := "No favorites yet"
	switch {
	case cc.State.LibraryLoading:
		favoriteMeta = "Loading library..."
	case cc.State.LibraryFailed:
		favoriteMeta = "Library unavailable"
	case cc.State.FavoriteCount > 0:
		favorites = pluralCount(cc.State.FavoriteCount, "channel")
		favoriteMeta = strings.Join(cc.State.FavoriteNames, " · ")
	}
	items = append(items, controlCenterItem{
		Label: "FAVORITES", Title: favorites, Meta: favoriteMeta,
		Icon: icons().heart, Action: cc.Actions.Favorites, Hint: "Open Favorites",
	})

	playlists := "Playlists"
	playlistMeta := "No playlists yet"
	switch {
	case cc.State.LibraryLoading:
		playlistMeta = "Loading library..."
	case cc.State.LibraryFailed:
		playlistMeta = "Library unavailable"
	case cc.State.PlaylistCount > 0:
		playlists = pluralCount(cc.State.PlaylistCount, "source")
		playlistMeta = strings.Join(cc.State.PlaylistNames, " · ")
	}
	items = append(items, controlCenterItem{
		Label: "PLAYLIST", Title: playlists, Meta: playlistMeta,
		Icon: icons().listVideo, Action: cc.Actions.Playlists, Hint: "Open Playlists",
	})

	return items
}

// icon draws one of the five Control Center navigation/action icons.
func (cc ControlCenter) icon(c *ui.Context, icon *ui.SVG, label string, selected bool, action func()) {
	t := c.Theme()
	button := ui.ButtonBase(c).Size(32, 32).Radius(8).FocusRingOffset(-2)
	if selected {
		button = button.Background(t.SurfaceHover)
	}
	button = button.Label(label).Tooltip(label).AccessSelected(selected)
	button.Children(func() {
		ui.Icon(c, icon).Size(20, 20).TextColor(t.Text)
	})
	if button.Clicked() && action != nil {
		action()
	}
}

// pluralCount formats a real count without inventing channel names.
func pluralCount(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", n, noun)
}
