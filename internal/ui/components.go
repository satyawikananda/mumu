package ui

import (
	"math"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// Artwork is a decoded channel logo with the bounded color derived from it.
// Loading and Failed keep the tile geometry stable while the app loads or
// rejects an image.
type Artwork struct {
	Bitmap   *ui.Bitmap
	Color    ui.Color
	Monogram string
	Loading  bool
	Failed   bool
}

// BadgeKind chooses the status treatment of a Badge.
type BadgeKind int

const (
	// BadgeNeutral is a secondary label such as VOD or a resolution.
	BadgeNeutral BadgeKind = iota + 1
	// BadgeLive is the live-format label.
	BadgeLive
	// BadgeWarning is buffering or a limited capability.
	BadgeWarning
	// BadgeDanger is an unavailable or failed state.
	BadgeDanger
)

// Badge is a compact, non-interactive status label. Text always carries the
// meaning; color is a secondary signal.
type Badge struct {
	Kind BadgeKind
	Text string
	Dot  bool
}

// View draws the badge.
func (b Badge) View(c *ui.Context) ui.Element {
	t := c.Theme()
	p := paletteFor(t.Dark)
	text := strings.TrimSpace(b.Text)
	if text == "" {
		text = b.defaultText()
	}

	background := ui.RGBA(255, 255, 255, 0.10)
	foreground := t.TextMuted
	border := ui.Transparent
	dot := b.Dot
	switch b.Kind {
	case BadgeLive:
		background = p.brandFill
		foreground = p.brandText
		dot = true
	case BadgeWarning:
		background = t.Surface
		foreground = t.Warning
		border = t.Warning.Alpha(0.45)
	case BadgeDanger:
		background = t.Surface
		foreground = t.Danger
		border = t.Danger.Alpha(0.45)
	}
	if !t.Dark && b.Kind == BadgeNeutral {
		background = ui.RGBA(0, 0, 0, 0.06)
	}

	return ui.Row(c).AlignItems(ui.Center).Gap(4).Padding(3, 7).Radius(3).
		Background(background).Border(1, border).Children(func() {
		if dot {
			ui.Box(c).Size(4, 4).Radius(2).Background(foreground).AccessHidden(true)
		}
		ui.Text(c, text).Font(bodyFont()).FontSize(textSize(t, 10)).FontWeight(700).
			LetterSpacing(0.7).TextColor(foreground).SingleLine()
	})
}

// defaultText is the label for a badge that did not set one.
func (b Badge) defaultText() string {
	switch b.Kind {
	case BadgeLive:
		return "LIVE"
	case BadgeWarning:
		return "BUFFERING"
	case BadgeDanger:
		return "UNAVAILABLE"
	default:
		return ""
	}
}

// Chip is an interactive filter chip. Active state changes fill, border and
// weight together, not only the color.
type Chip struct {
	Key   string
	Label string
	// Icon is an optional leading glyph; the label always carries the meaning.
	Icon     *ui.SVG
	Active   bool
	Disabled bool
	OnClick  func()
}

// View draws the chip.
func (ch Chip) View(c *ui.Context) ui.Element {
	t := c.Theme()
	if ch.Key != "" {
		c = c.Key(ch.Key)
	}

	label := strings.TrimSpace(ch.Label)
	fill := ui.Transparent
	border := ui.Transparent
	textColor := t.TextMuted
	weight := 500
	if ch.Active {
		if t.Dark {
			fill = ui.RGBA(255, 255, 255, 0.09)
			border = ui.RGBA(255, 255, 255, 0.20)
		} else {
			fill = ui.RGBA(0, 0, 0, 0.06)
			border = ui.RGBA(0, 0, 0, 0.16)
		}
		textColor = t.Text
		weight = 600
	}

	chip := ui.ButtonBase(c).Height(30).Padding(7, 11).Radius(radiusChip).
		Border(1, border).Background(fill)
	if chip.Hovered() && !ch.Disabled {
		if t.Dark {
			chip = chip.Background(ui.RGBA(255, 255, 255, 0.12))
		} else {
			chip = chip.Background(ui.RGBA(0, 0, 0, 0.08))
		}
	}
	chip = chip.Disabled(ch.Disabled).AccessSelected(ch.Active).Label(label)
	if ch.Icon != nil {
		chip = chip.Gap(6)
	}
	chip.Children(func() {
		if ch.Icon != nil {
			ui.Icon(c, ch.Icon).Size(14, 14).TextColor(textColor).AccessHidden(true)
		}
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(t, 13)).FontWeight(weight).
			TextColor(textColor).SingleLine()
	})
	chip.Transition(ui.ElementTransition{Colors: true, Duration: hoverMotion.Duration})
	if chip.Clicked() && !ch.Disabled && ch.OnClick != nil {
		ch.OnClick()
	}

	return chip
}

// Keycap is one keyboard key inside a hint.
type Keycap struct {
	Label string
}

// View draws the keycap.
func (k Keycap) View(c *ui.Context) ui.Element {
	t := c.Theme()
	fill := ui.RGBA(255, 255, 255, 0.10)
	if !t.Dark {
		fill = ui.RGBA(0, 0, 0, 0.07)
	}

	return ui.Row(c).Height(19).MinWidth(19).Padding(3, 7).Radius(radiusKeycap).
		Background(fill).Center().Children(func() {
		ui.Text(c, k.Label).Font(bodyFont()).FontSize(textSize(t, 11)).
			FontWeight(600).TextColor(t.Text).SingleLine()
	})
}

// Hint is one key/action pair in a HintBar.
type Hint struct {
	Key    string
	Action string
}

// HintBar renders the shared shortcut strip. Callers supply hints from the
// scoped shortcut registry; the bar never owns a second hardcoded list.
type HintBar struct {
	Hints []Hint
}

// View draws up to five hints, centered.
func (h HintBar) View(c *ui.Context) ui.Element {
	t := c.Theme()
	hints := h.Hints
	if len(hints) > 5 {
		hints = hints[:5]
	}

	bar := ui.Row(c).Height(hintBarHeight).Gap(hintBarGap).
		Center().Label("Keyboard shortcuts").
		Role(ui.RoleGroup)
	bar.Children(func() {
		for _, hint := range hints {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				Keycap{Label: hint.Key}.View(c)
				ui.Text(c, hint.Action).Font(bodyFont()).FontSize(textSize(t, 11)).
					TextColor(t.TextMuted).SingleLine()
			})
		}
	})

	return bar
}

// FocusHint is the action label that follows the focused item in a Control
// Center or tile rail.
type FocusHint struct {
	Key     string
	Action  string
	Visible bool
}

// View draws the hint, fading in place when it appears.
func (f FocusHint) View(c *ui.Context) ui.Element {
	t := c.Theme()
	row := ui.Row(c).AlignItems(ui.Center).Gap(6).Label(f.Key + " " + f.Action)
	row.Children(func() {
		Keycap{Label: f.Key}.View(c)
		ui.Text(c, f.Action).Font(bodyFont()).FontSize(textSize(t, 12)).
			FontWeight(600).TextColor(t.Text).SingleLine()
	})

	pose := ui.Pose{}
	if !f.Visible {
		pose = ui.Pose{Y: 4, Fade: 1}
	}
	row.AnimatePose(pose, hoverMotion)
	row.IgnoreInput(!f.Visible).AccessHidden(!f.Visible)

	return row
}

// BrandLockup draws the tomato mark and product wordmark.
func BrandLockup(c *ui.Context, mark *ui.Bitmap) ui.Element {
	t := c.Theme()

	return ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		if mark != nil {
			ui.Image(c, mark).Size(44, 44).AccessHidden(true)
		}
		ui.Text(c, ProductName).Font(displayFont()).FontSize(textSize(t, 24)).
			FontWeight(700).TextColor(t.Text).SingleLine()
	})
}

// monogram returns up to two uppercase letters or digits from a channel name.
func monogram(name string) string {
	var b strings.Builder
	count := 0
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
		count++
		if count == 2 {
			break
		}
	}
	if b.Len() == 0 {
		return "TV"
	}

	return b.String()
}

// artworkMonogram returns the explicit monogram or derives one from the name.
func artworkMonogram(art Artwork, name string) string {
	if art.Monogram != "" {
		return art.Monogram
	}

	return monogram(name)
}

// artworkColor returns the bounded art color, or a dark neutral raised surface.
// White text and the monogram sit on this fill in both appearances, so the
// fallback stays dark instead of following a light theme surface.
func artworkColor(art Artwork) ui.Color {
	if art.Color.A > 0 {
		return boundedArtworkColor(art.Color)
	}

	return paletteFor(true).surface2
}

// boundedArtworkColor darkens a tint until white text passes 4.5:1. The app
// computes tints off the UI thread; this guard keeps a caller-provided color
// safe too.
func boundedArtworkColor(col ui.Color) ui.Color {
	col.A = 255
	for range 16 {
		if colorLuminance(col) <= 0.15 {
			return col
		}
		col.R = scaleColorChannel(col.R)
		col.G = scaleColorChannel(col.G)
		col.B = scaleColorChannel(col.B)
	}

	return col
}

func scaleColorChannel(value uint8) uint8 {
	return uint8(min(255, float64(value)*0.92))
}

func colorLuminance(col ui.Color) float64 {
	channel := func(value uint8) float64 {
		s := float64(value) / 255
		if s <= 0.03928 {
			return s / 12.92
		}

		return math.Pow((s+0.055)/1.055, 2.4)
	}

	return 0.2126*channel(col.R) + 0.7152*channel(col.G) + 0.0722*channel(col.B)
}
