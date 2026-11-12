package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// ChannelTile is the shared grid and rail card for a channel.
type ChannelTile struct {
	Key    string
	Name   string
	Meta   string
	Live   bool
	Art    Artwork
	Width  float32
	Height float32
	Grow   bool
	// FocusField and FocusValue bind keyboard focus to an app-owned cursor.
	// Leave FocusValue empty for a tile outside a rail.
	FocusField *string
	FocusValue string
	Selected   bool
	Disabled   bool
	OnActivate func()
}

// View draws the tile. Hover scales the tile; keyboard focus uses the theme
// ring. The two states never share a visual.
func (ch ChannelTile) View(c *ui.Context) ui.Element {
	t := c.Theme()
	p := paletteFor(t.Dark)
	if ch.Key != "" {
		c = c.Key(ch.Key)
	}

	height := ch.Height
	if height <= 0 {
		height = layoutFor(c.Size()).TileHeight
	}
	artColor := artworkColor(ch.Art)
	logoSize := min(float32(92), max(float32(64), height*0.44))
	logoOffset := logoSize * 0.18
	logoRadius := logoSize * 0.15
	title := strings.TrimSpace(ch.Name)
	if title == "" {
		title = "Untitled channel"
	}
	titleColor := ui.Hex("#FFFFFF")
	if ch.Disabled {
		titleColor = p.muted
	}

	tile := ui.ButtonBase(c).Height(height).Radius(radiusTile).
		Border(2, ui.Transparent).ClipInset(0, 0, 0, 0, radiusTile).
		FocusRingOffset(-2).
		LinearGradient(ui.LinearGradient{
			From:  artColor,
			To:    ui.Hex("#0E0E12"),
			Angle: 140,
			Oklab: true,
		})
	switch {
	case ch.Width > 0:
		tile = tile.Width(ch.Width)
	case ch.Grow:
		tile = tile.Grow(1)
	default:
		tile = tile.FillWidth()
	}
	if ch.Selected {
		tile = tile.Border(2, p.brand)
	}

	tile.Children(func() {
		ui.Box(c).Absolute().Fill().IgnoreInput(true).
			RadialGradient(ui.RadialGradient{
				Center: ui.Point{X: 0.15},
				Radius: height * 0.95,
				Oklab:  true,
				Stops: []ui.GradientStop{
					{Color: artColor.Mix(ui.Hex("#FFFFFF"), 0.18).Alpha(0.35)},
					{Color: ui.Transparent, At: 1},
				},
			})

		ui.Text(c, title).Font(displayFont()).FontSize(textSize(t, 19)).FontWeight(700).
			TextColor(titleColor).Absolute().Left(15).Right(15).Top(14).
			MaxLines(2).Ellipsis("…")

		if ch.Live {
			ui.Box(c).Absolute().Left(15).Bottom(14).Children(func() {
				Badge{Kind: BadgeLive, Text: "LIVE"}.View(c)
			})
		}

		logo := ui.Box(c).Size(logoSize, logoSize).Radius(logoRadius).
			Background(ui.Hex("#FFFFFF")).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.40)).
			Transform(ui.Rotation(-18)).TransformOrigin(1, 1).
			Absolute().Right(-logoOffset).Bottom(-logoOffset)
		logo.Children(func() {
			ui.Column(c).Fill().Center().Children(func() {
				if ch.Art.Bitmap != nil {
					ui.Image(c, ch.Art.Bitmap).Size(logoSize*0.62, logoSize*0.62).
						Fit(ui.Contain).AccessHidden(true)
					return
				}
				ui.Text(c, artworkMonogram(ch.Art, title)).Font(bodyFont()).
					FontSize(logoSize * 0.30).FontWeight(700).
					TextColor(ui.Hex("#18181B")).SingleLine()
			})
		})
	})

	label := channelLabel(title, ch.Meta, ch.Live)
	tile = tile.Label(label).AccessSelected(ch.Selected).Busy(ch.Art.Loading).
		Disabled(ch.Disabled).DimDisabled(false)
	switch {
	case ch.Art.Loading:
		tile = tile.Description("Loading channel logo")
	case ch.Art.Failed:
		tile = tile.Description("Logo unavailable; showing the channel monogram")
	}

	pose := ui.Pose{}
	if tile.Hovered() && !ch.Disabled {
		pose = ui.Pose{Scale: 1.03}
	}
	tile.AnimatePose(pose, hoverMotion)

	if ch.FocusField != nil && ch.FocusValue != "" && !ch.Disabled {
		tile = tile.FocusBind(ch.FocusField, ch.FocusValue)
	}

	if tile.Clicked() && !ch.Disabled && ch.OnActivate != nil {
		ch.OnActivate()
	}

	return tile
}

// RowState is what a virtualized list reports to a ChannelRow.
type RowState struct {
	Hovered  bool
	Pressed  bool
	Selected bool
	Focused  bool
}

// ChannelRow is the dense channel row for virtualized search and playlist
// results. The list owns focus and selection; the row draws them.
type ChannelRow struct {
	Name string
	Meta string
	// Note explains why the row is disabled, such as a disabled source.
	Note     string
	Live     bool
	Art      Artwork
	Disabled bool
	// Action is an optional trailing button, such as a favorite toggle.
	Action         string
	ActionSelected bool
	OnAction       func()
}

// ListView draws a row from ui.List's row state.
func (ch ChannelRow) ListView(c *ui.Context, row ui.ListRow) ui.Element {
	return ch.View(c, RowState{
		Hovered:  row.Hovered(),
		Pressed:  row.Pressed(),
		Selected: row.Selected(),
		Focused:  row.ListFocused(),
	})
}

// View draws a row with explicit state.
func (ch ChannelRow) View(c *ui.Context, state RowState) ui.Element {
	t := c.Theme()
	p := paletteFor(t.Dark)
	name := strings.TrimSpace(ch.Name)
	if name == "" {
		name = "Untitled channel"
	}
	nameColor := t.Text
	if ch.Disabled {
		nameColor = t.TextMuted
	}

	background := ui.Transparent
	switch {
	case ch.Disabled:
	case state.Pressed:
		if t.Dark {
			background = ui.RGBA(255, 255, 255, 0.10)
		} else {
			background = ui.RGBA(0, 0, 0, 0.08)
		}
	case state.Focused:
		if t.Dark {
			background = ui.RGBA(255, 255, 255, 0.08)
		} else {
			background = ui.RGBA(0, 0, 0, 0.06)
		}
	case state.Hovered:
		if t.Dark {
			background = ui.RGBA(255, 255, 255, 0.06)
		} else {
			background = ui.RGBA(0, 0, 0, 0.04)
		}
	}
	if state.Selected {
		background = p.brand.Alpha(0.12)
	}
	border := ui.Transparent
	if state.Focused {
		border = p.focus
	}

	row := ui.Row(c).Height(48).Padding(9, 12).Gap(12).Radius(radiusRow).
		AlignItems(ui.Center).Background(background).Border(1.5, border).
		Transition(ui.ElementTransition{Colors: true, Duration: hoverMotion.Duration})
	row.Children(func() {
		chip := ui.Box(c).Size(36, 36).Radius(radiusChip).Center().
			Background(artworkColor(ch.Art)).ClipInset(0, 0, 0, 0, radiusChip)
		chip.Children(func() {
			if ch.Art.Bitmap != nil {
				ui.Image(c, ch.Art.Bitmap).Size(24, 24).Fit(ui.Contain).AccessHidden(true)
				return
			}
			ui.Text(c, artworkMonogram(ch.Art, name)).Font(bodyFont()).
				FontSize(textSize(t, 12)).FontWeight(700).
				TextColor(ui.Hex("#FFFFFF")).SingleLine()
		})

		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, name).Font(bodyFont()).FontSize(textSize(t, 15)).
				FontWeight(600).TextColor(nameColor).SingleLine()
			if meta := strings.TrimSpace(ch.Meta); meta != "" {
				ui.Text(c, meta).Font(bodyFont()).FontSize(textSize(t, 12)).
					TextColor(t.TextMuted).SingleLine()
			}
			if note := strings.TrimSpace(ch.Note); note != "" {
				ui.Text(c, note).Font(bodyFont()).FontSize(textSize(t, 12)).
					TextColor(t.Warning).SingleLine()
			}
		})

		if ch.Live {
			Badge{Kind: BadgeLive, Text: "LIVE"}.View(c)
		}

		if ch.Action != "" && ch.OnAction != nil {
			ui.Button(c, ch.Action).AccessSelected(ch.ActionSelected).OnClick(ch.OnAction)
		}
	})

	row = row.Label(ch.rowLabel(name)).AccessSelected(state.Selected)
	if ch.Disabled {
		row = row.Disabled(true).DimDisabled(false)
	}

	return row
}

// rowLabel is the accessible name of one row, including why it is disabled.
func (ch ChannelRow) rowLabel(name string) string {
	parts := []string{channelLabel(name, ch.Meta, ch.Live)}
	if note := strings.TrimSpace(ch.Note); note != "" {
		parts = append(parts, note)
	}

	return strings.Join(parts, ", ")
}

// ChannelGrid lays out tiles in the responsive 5/4/4 column grid.
type ChannelGrid struct {
	Key     string
	Tiles   []ChannelTile
	Columns int
	Height  float32
}

// View draws the grid. Keep the caller's page at or below 20 tiles.
func (g ChannelGrid) View(c *ui.Context) ui.Element {
	layout := layoutFor(c.Size())
	columns := g.Columns
	if columns <= 0 {
		columns = layout.DiscoverColumns
	}
	height := g.Height
	if height <= 0 {
		height = layout.TileHeight
	}

	grid := ui.Grid(c).Columns(columns).Gap(12)
	if g.Key != "" {
		grid = grid.Key(g.Key)
	}
	grid.Children(func() {
		for _, tile := range g.Tiles {
			tile.Height = height
			tile.Grow = false
			tile.View(c)
		}
	})

	return grid
}

// Rail is a titled card row. The responsive card count comes from layoutFor.
type Rail struct {
	Key   string
	Title string
	Count string
	Tiles []ChannelTile
	Cards int
	// SeeAllLabel is the accessible name of the optional See all action.
	SeeAllLabel string
	OnSeeAll    func()
}

// View draws the rail.
func (r Rail) View(c *ui.Context) ui.Element {
	t := c.Theme()
	layout := layoutFor(c.Size())
	cards := r.Cards
	if cards <= 0 {
		cards = layout.RailCards
	}
	if len(r.Tiles) > cards {
		r.Tiles = r.Tiles[:cards]
	}

	rail := ui.Column(c).FillWidth().Gap(12)
	if r.Key != "" {
		rail = rail.Key(r.Key)
	}
	rail.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, r.Title).Font(displayFont()).FontSize(textSize(t, 19)).
				FontWeight(700).TextColor(t.Text).Grow(1).SingleLine()
			if r.Count != "" {
				ui.Text(c, r.Count).Font(bodyFont()).FontSize(textSize(t, 12)).
					TextColor(t.TextMuted).SingleLine()
			}
			if r.OnSeeAll != nil {
				label := r.SeeAllLabel
				if label == "" {
					label = "See all " + strings.ToLower(r.Title)
				}
				seeAll := ui.ButtonBase(c).Padding(4, 9).Radius(radiusChip).
					FocusRingOffset(-2).Label(label).Tooltip("Open in Library")
				if seeAll.Hovered() {
					seeAll = seeAll.Background(t.SurfaceHover)
				}
				seeAll.Children(func() {
					ui.Text(c, "See all").Font(bodyFont()).FontSize(textSize(t, 12)).
						FontWeight(600).TextColor(t.TextMuted).SingleLine()
				})
				seeAll.Transition(ui.ElementTransition{Colors: true, Duration: hoverMotion.Duration})
				if seeAll.Clicked() {
					r.OnSeeAll()
				}
			}
		})

		row := ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Stretch)
		row.Children(func() {
			for _, tile := range r.Tiles {
				tile.Grow = true
				tile.Height = layout.TileHeight
				tile.View(c)
			}
		})
	})

	return rail
}

// channelLabel is the accessible name shared by tiles and rows.
func channelLabel(name, meta string, live bool) string {
	parts := []string{strings.TrimSpace(name)}
	if meta = strings.TrimSpace(meta); meta != "" {
		parts = append(parts, meta)
	}
	if live {
		parts = append(parts, "live format")
	}

	return strings.Join(parts, ", ")
}
