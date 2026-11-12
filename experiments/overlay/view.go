//go:build darwin && arm64

package main

import "github.com/egoist/mygo/ui"

// Design tokens for the spike, taken from the Watch frames in
// mumu-design.pen. The M5 revamp owns the real palette in #33; the spike only
// has to look like the Watch design closely enough to judge scrims, contrast
// and hit targets over live video.
var (
	// spikeBase is the surface the window paints where no video shows, and
	// only when the video is not a native element under MyGo's surface.
	spikeBase = ui.RGB(5, 6, 10)
	// spikeChip is a control's surface over video: alpha, never opaque, so a
	// bright frame still reads as video through it.
	spikeChip = ui.RGBA(24, 24, 27, 0.72)
	// spikePanel is the drawer's surface (#08090CF2 in the frames).
	spikePanel = ui.RGBA(8, 9, 12, 0.949)
	// spikeCard backs the pause block over the dimmed video.
	spikeCard = ui.RGBA(8, 9, 12, 0.55)
	// spikeHairline edges a chip or a panel.
	spikeHairline = ui.RGBA(255, 255, 255, 0.14)
	// spikeFaint fills a resting row inside the drawer.
	spikeFaint = ui.RGBA(255, 255, 255, 0.06)
	// spikeCharcoal is the text on the tomato accent.
	spikeCharcoal = ui.RGB(9, 9, 11)
	spikeText     = ui.RGB(244, 244, 245)
	spikeMuted    = ui.RGB(161, 161, 170)
	// spikeAccent is the brand tomato, kept for the primary action, the live
	// pill, the active channel and the click counter's dot, and nowhere else.
	spikeAccent = ui.RGB(248, 32, 40)
	// Semantic tints for the status dot only, so player state never borrows
	// the accent that means "act here".
	spikeGood  = ui.RGB(74, 222, 128)
	spikeWarn  = ui.RGB(250, 204, 21)
	spikeError = ui.RGB(248, 113, 113)
)

const (
	// noMods is the Modifiers zero value: MyGo 0.9 names no "none" modifier
	// (ui.Shift, ui.Ctrl, ui.Alt and ui.Super are the only ones), so a
	// shortcut without modifiers passes the zero value, as its tests do.
	noMods = ui.Modifiers(0)
	// spikeChannel names what the spike plays. The spike takes one -stream and
	// keeps no channel name in appState, so the chrome shows a fixture label
	// rather than inventing a selection the state does not hold.
	spikeChannel = "Overlay fixture"
	// designW and designH are the frame size the Pencil measurements were
	// taken at; every inset scales from them instead of being hardcoded.
	designW float32 = 1470
	designH float32 = 956
)

// chromeMetrics are the sizes one window needs, scaled from the Pencil frame.
// A 320x240 panel clamps to the smallest usable values instead of shrinking
// its labels out of legibility.
type chromeMetrics struct {
	w, h float32
	// scale is the design scale, clamped so small windows stay usable.
	scale float32
	// insetX and insetY are the 48 units of the frames, by axis.
	insetX, insetY float32
	// gap is the 18 unit gap of the frames.
	gap float32
	// drawerW is 28% of the width, between 280 and the 420 of the frames.
	drawerW float32
	// radius is the 14 unit corner of the frames.
	radius float32
	// titleSize is the channel name above the controls, bodySize a muted
	// line or a status line.
	titleSize, bodySize float32
	// buttonSize is the label of a control, controlH its height.
	buttonSize, controlH float32
	// padX is the horizontal padding of a control, drawerPad the drawer's.
	padX, drawerPad float32
	// sliderW and statusMaxW keep the volume and the status line from
	// crowding the transport at 640 points.
	sliderW, statusMaxW float32
}

// metricsFor measures the window the frame is built for.
func metricsFor(c *ui.Context) chromeMetrics {
	w, h := c.Size()
	// A frame can build before the window reports its size. Fall back to the
	// smallest window rather than a zero-width layout.
	w, h = max(w, 320), max(h, 240)

	scale := max(min(w/designW, h/designH), 0.42)
	scale = min(scale, 1)

	return chromeMetrics{
		w:          w,
		h:          h,
		scale:      scale,
		insetX:     max(16, 48*scale),
		insetY:     max(14, 48*scale),
		gap:        max(8, 18*scale),
		drawerW:    max(280, min(420, w*0.28)),
		radius:     max(8, 14*scale),
		titleSize:  max(17, 32*scale),
		bodySize:   max(11, 15*scale),
		buttonSize: max(12, 15*scale),
		controlH:   max(32, 40*scale),
		padX:       max(10, 20*scale),
		drawerPad:  max(14, 24*scale),
		sliderW:    max(90, 160*scale),
		statusMaxW: max(300, 360*scale),
	}
}

// view builds one window's interface: the video box first, then chrome above it.
//
// A mode that shows the retained view under MyGo's surface must leave the root
// transparent: an opaque root hides the video entirely. Chrome built after the
// video element paints above it, and only scrims, bars and panels carry fills,
// all of them with alpha.
func (a *app) view(c *ui.Context, mini bool) {
	if a.keepFront {
		activate()
	}
	a.drainPending()
	if a.s.Mode.transparentRoot() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(spikeBase)
	}

	ui.Column(c).Fill().Children(func() {
		a.videoElement(c)
	})

	a.bindKeys(c, mini)

	if mini {
		a.pipChrome(c)

		return
	}

	a.watchChrome(c)
}

// bindKeys installs the window's shortcuts. c is the window's root context, so
// c.OnShortcut registers window shortcuts: they fire wherever the focus is,
// including while the chrome is hidden.
func (a *app) bindKeys(c *ui.Context, mini bool) {
	c.OnShortcut(noMods, ui.KeyH, func() {
		a.do.ToggleOverlay()
	})
	c.OnShortcut(noMods, ui.KeySpace, func() {
		a.do.Pause()
	})
	c.OnShortcut(noMods, ui.KeyF, func() {
		a.do.Fullscreen()
	})
	c.OnShortcut(noMods, ui.KeyP, func() {
		a.do.MovePiP()
	})
	c.OnShortcut(ui.Cmd, ui.KeyQ, func() {
		a.do.Quit()
	})

	// The drawer belongs to the Watch window only, so its keys do nothing in
	// the floating panel.
	if mini {
		return
	}

	c.OnShortcut(noMods, ui.KeyD, func() {
		a.do.ToggleDrawer()
	})
	c.OnShortcut(noMods, ui.KeyEscape, func() {
		if a.s.Drawer {
			a.do.ToggleDrawer()
		}
	})
}

// watchChrome draws the Watch surface over the video box: the paused dim, the
// two scrims, the bars, the pause block and last the channel drawer.
func (a *app) watchChrome(c *ui.Context) {
	m := metricsFor(c)

	ui.Overlay(c, func() {
		// The dim darkens the video only: the bars paint above it, so their
		// controls stay at full contrast while paused.
		if a.s.Paused {
			pauseDim(c)
		}

		if a.s.Overlay {
			topScrim(c, m)
			bottomScrim(c, m)
			topBar(c, a, m)
			bottomContent(c, a, m)
		} else {
			chromeHint(c, m, a)
		}

		if a.s.Paused {
			pauseBlock(c, m, false)
		}

		if a.s.Drawer {
			drawerBackdrop(c)
			drawerPanel(c, a, m)
		}
	})
}

// pipChrome draws the floating window's chrome: a slim title strip, a slim
// control strip and, while paused, the same dim and pause block as Watch.
func (a *app) pipChrome(c *ui.Context) {
	m := metricsFor(c)

	ui.Overlay(c, func() {
		if a.s.Paused {
			pauseDim(c)
		}

		if a.s.Overlay {
			pipTitle(c, a, m)
			pipBar(c, a, m)
		} else {
			chromeHint(c, m, a)
		}

		if a.s.Paused {
			pauseBlock(c, m, true)
		}
	})
}

// topScrim is the top 21% of the frame: #000000D9 to #00000066 at 45%, clear at
// the bottom.
func topScrim(c *ui.Context, m chromeMetrics) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(m.h * 0.21).
		LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: ui.RGBA(0, 0, 0, 0.851), At: 0},
			{Color: ui.RGBA(0, 0, 0, 0.4), At: 0.45},
			{Color: ui.Transparent, At: 1},
		}}).IgnoreInput(true)
}

// bottomScrim is the bottom 46% of the frame: clear at 54% down, #000000B3 at
// 35% of the scrim, #000000F2 at the bottom.
func bottomScrim(c *ui.Context, m chromeMetrics) {
	ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(m.h * 0.46).
		LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: ui.Transparent, At: 0},
			{Color: ui.RGBA(0, 0, 0, 0.702), At: 0.35},
			{Color: ui.RGBA(0, 0, 0, 0.949), At: 1},
		}}).IgnoreInput(true)
}

// topBar is the live pill, the channel and its meta, then the chrome controls.
func topBar(c *ui.Context, a *app, m chromeMetrics) {
	ui.Row(c).Absolute().Left(m.insetX).Right(m.insetX).Top(m.insetY).
		Gap(m.gap).AlignItems(ui.Center).Children(func() {
		livePill(c, m)

		ui.Text(c, spikeChannel).Bold().FontSize(max(13, 20*m.scale)).
			TextColor(spikeText).SingleLine()
		ui.Text(c, "mode "+string(a.s.Mode)).FontSize(max(10, 13*m.scale)).
			TextColor(spikeMuted).SingleLine().
			MaxWidth(max(80, 220*m.scale)).Shrink(1)

		ui.Box(c).Grow(1).Height(1)

		topChip(c, m, "Drawer", "Channel drawer (D)", func() {
			a.do.Hit("drawer")
			a.do.ToggleDrawer()
		})
		topChip(c, m, "Fullscreen", "Fullscreen (F)", func() {
			a.do.Hit("fullscreen")
			a.do.Fullscreen()
		})
		topChip(c, m, "PiP", "Move to PiP (P)", func() {
			a.do.Hit("pip")
			a.do.MovePiP()
		})
	})
}

// livePill is the stream type chip of the frames. The spike plays one stream
// and cannot tell live from on demand, so the pill is a design constant, not a
// claim read from state.
func livePill(c *ui.Context, m chromeMetrics) {
	ui.Row(c).Height(max(18, 26*m.scale)).Padding(4, 9).Radius(4).
		Background(spikeAccent).AlignItems(ui.Center).Shrink(0).
		Label("Live stream").Children(func() {
		ui.Text(c, "LIVE").Bold().FontSize(max(10, 12*m.scale)).
			LetterSpacing(0.6).TextColor(spikeCharcoal)
	})
}

// bottomContent is the meta block and the controls row, inset 48 from the left
// and the bottom, over the bottom scrim.
func bottomContent(c *ui.Context, a *app, m chromeMetrics) {
	ui.Column(c).Absolute().Left(m.insetX).Right(m.insetX).Bottom(m.insetY).
		Gap(m.gap).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, spikeChannel).Bold().FontSize(m.titleSize).
				TextColor(spikeText).SingleLine()
			ui.Text(c, "mode "+string(a.s.Mode)+" · one retained libmpv session").
				FontSize(m.bodySize).TextColor(spikeMuted).SingleLine().
				Tooltip("The spike keeps one libmpv session across Watch and PiP")
		})

		ui.Row(c).FillWidth().Gap(m.gap).AlignItems(ui.Center).Wrap().
			Children(func() {
				transport(c, a, m)
				volumeGroup(c, a, m)
				statusText(c, a, m)
				clickCounter(c, a, m)
			})
	})
}

// transport is the player's controls. The accent follows the state: Play when
// nothing plays, Pause while playing, Resume while paused.
func transport(c *ui.Context, a *app, m chromeMetrics) {
	paused := a.s.Paused

	ui.Row(c).Gap(m.gap).AlignItems(ui.Center).Children(func() {
		transportButton(c, m, "Play", !paused && !a.s.Playing, "Load the stream again", func() {
			a.do.Hit("play")
			a.do.Play()
		})
		transportButton(c, m, pauseLabel(a), paused || a.s.Playing, "Space", func() {
			a.do.Hit("pause")
			a.do.Pause()
		})
		transportButton(c, m, "Stop", false, "Stop the stream", func() {
			a.do.Hit("stop")
			a.do.Stop()
		})
		transportButton(c, m, "Hide chrome", false, "Hide chrome (H)", func() {
			a.do.Hit("toggle-overlay")
			a.do.ToggleOverlay()
		})
	})
}

// volumeGroup is the volume slider and its value. It draws its own rail and
// knob so no system accent color cuts into the charcoal chrome.
func volumeGroup(c *ui.Context, a *app, m chromeMetrics) {
	volume := float64(a.s.Volume)
	knob := max(9, 11*m.scale)
	pad := knob/2 + 1

	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		if m.w >= 860 {
			ui.Text(c, "Volume").FontSize(m.bodySize).TextColor(spikeMuted)
		}

		slider := ui.SliderBase(c, &volume, 0, 100).Width(m.sliderW).
			Height(max(20, 24*m.scale)).PaddingX(pad).Label("Volume")
		slider.Draw(func(p *ui.Painter, r ui.Rect) {
			frac := max(0, min(1, float32(volume/100)))
			rail := ui.Rect{X: r.X + pad, Y: r.Y + r.H/2 - 2, W: max(r.W-2*pad, 0), H: 4}
			p.Fill(rail, ui.RGBA(255, 255, 255, 0.22), 2)
			if filled := rail.W * frac; filled > 0 {
				p.Fill(ui.Rect{X: rail.X, Y: rail.Y, W: filled, H: rail.H},
					ui.RGBA(255, 255, 255, 0.85), 2)
			}
			p.Fill(ui.Rect{
				X: rail.X + rail.W*frac - knob/2,
				Y: r.Y + (r.H-knob)/2,
				W: knob, H: knob,
			}, spikeText, knob/2)
		})
		slider.OnChange(func() { a.do.Volume(int(volume + 0.5)) })

		// A fixed width keeps the slider from shifting as the value grows.
		ui.Text(c, itoa(a.s.Volume)+"%").FontSize(m.bodySize).
			TextColor(spikeMuted).Width(38)
	})
}

// statusText is the evidence line: the player state, its detail and the
// drawable the native surface currently reports.
func statusText(c *ui.Context, a *app, m chromeMetrics) {
	status := a.s.Status
	if status == "" {
		status = "idle"
	}
	detail := a.s.Detail
	if detail == "" {
		detail = "—"
	}
	line := status + " · " + detail + " · drawable " +
		itoa(a.s.DrawableW) + "x" + itoa(a.s.DrawableH)

	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(statusTint(status))
		ui.Text(c, line).FontSize(m.bodySize).TextColor(spikeMuted).
			SingleLine().MaxWidth(m.statusMaxW).Tooltip(line)
	})
}

// clickCounter is the spike's evidence: pointer input on chrome over live video
// reached a control, and which one. The count is state, not decoration.
func clickCounter(c *ui.Context, a *app, m chromeMetrics) {
	last := a.s.LastAction
	if last == "" {
		last = "—"
	}

	ui.Row(c).Height(max(24, 30*m.scale)).PaddingX(max(8, 12*m.scale)).
		Radius(m.radius).Gap(6).Background(spikeFaint).
		Border(1, spikeHairline).AlignItems(ui.Center).Shrink(0).
		Label("Clicks " + itoa(a.s.Clicks) + ", last " + last).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(spikeAccent)
		ui.Text(c, "clicks "+itoa(a.s.Clicks)).Bold().
			FontSize(max(10, 12*m.scale)).TextColor(spikeText)
		ui.Text(c, "· "+last).FontSize(max(10, 12*m.scale)).
			TextColor(spikeMuted).SingleLine().
			MaxWidth(max(110, 160*m.scale)).Tooltip(last)
	})
}

// topChip is a quiet chrome control in the top bar.
func topChip(c *ui.Context, m chromeMetrics, label, hint string, fn func()) {
	ui.ButtonBase(c).Label(label).Tooltip(hint).
		Height(max(24, 30*m.scale)).PaddingX(m.padX).Radius(m.radius).
		Background(spikeChip).Border(1, spikeHairline).
		Cursor(ui.CursorPointer).Shrink(0).OnClick(fn).Children(func() {
		ui.Text(c, label).FontSize(max(11, 14*m.scale)).TextColor(spikeText)
	})
}

// transportButton is a player control: the accent one is the action the state
// asks for, the others are quiet chips.
func transportButton(c *ui.Context, m chromeMetrics, label string, primary bool, hint string, fn func()) {
	button := ui.ButtonBase(c).Label(label).Height(m.controlH).
		PaddingX(m.padX).Radius(m.radius).Cursor(ui.CursorPointer).
		Shrink(0).OnClick(fn)
	if hint != "" {
		button = button.Tooltip(hint)
	}

	color := spikeText
	if primary {
		button = button.Background(spikeAccent)
		// Charcoal on tomato measures about 5:1; white on #F82028 about 4.0:1,
		// below AA for this label size.
		color = spikeCharcoal
	} else {
		button = button.Background(spikeChip).Border(1, spikeHairline)
	}

	button.Children(func() {
		text := ui.Text(c, label).FontSize(m.buttonSize).TextColor(color)
		if primary {
			text.Bold()
		}
	})
}

// chromeHint is what remains when the chrome is hidden: a small reminder that H
// brings it back, placed so it never covers the middle of the frame.
func chromeHint(c *ui.Context, m chromeMetrics, a *app) {
	ui.Row(c).Absolute().Left(m.insetX).Bottom(m.insetY).Gap(8).
		Radius(m.radius).Background(ui.RGBA(0, 0, 0, 0.62)).
		Border(1, spikeHairline).Padding(6, 9).
		AlignItems(ui.Center).IgnoreInput(true).Children(func() {
		ui.Box(c).Size(18, 18).Radius(4).Center().
			Background(ui.RGBA(248, 32, 40, 0.18)).
			Border(1, ui.RGBA(248, 32, 40, 0.55)).Children(func() {
			ui.Text(c, "H").Bold().FontSize(10).TextColor(spikeText)
		})
		ui.Text(c, "shows controls · "+itoa(a.s.Clicks)+" clicks").
			FontSize(max(10, m.bodySize-1)).TextColor(spikeMuted)
	})
}

// pauseDim is #00000059 over the video area, under the bars.
func pauseDim(c *ui.Context) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Background(ui.RGBA(0, 0, 0, 0.349)).IgnoreInput(true)
}

// pauseBlock is the centred pause card: a drawn pause glyph, "Paused" and, in
// Watch, a hint that Resume continues.
func pauseBlock(c *ui.Context, m chromeMetrics, compact bool) {
	glyph, barW, barH := max(44, 64*m.scale), max(4, 6*m.scale), max(16, 24*m.scale)
	titleSize, pad := max(16, 24*m.scale), max(14, 24*m.scale)
	if compact {
		glyph, barW, barH = max(32, 48*m.scale), 4, max(12, 16*m.scale)
		titleSize, pad = max(13, 18*m.scale), max(10, 16*m.scale)
	}

	ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Center().
		IgnoreInput(true).Children(func() {
		ui.Column(c).Center().Gap(max(6, 10*m.scale)).Padding(pad).
			Radius(max(12, 18*m.scale)).Background(spikeCard).
			Border(1, spikeHairline).Children(func() {
			ui.Box(c).Size(glyph, glyph).Radius(glyph/2).Center().
				Background(ui.RGBA(0, 0, 0, 0.45)).
				Border(1, spikeHairline).Children(func() {
				ui.Row(c).Gap(max(4, 6*m.scale)).AlignItems(ui.Center).
					Children(func() {
						ui.Box(c).Size(barW, barH).Radius(2).
							Background(spikeText)
						ui.Box(c).Size(barW, barH).Radius(2).
							Background(spikeText)
					})
			})
			ui.Text(c, "Paused").Bold().FontSize(titleSize).
				TextColor(spikeText).SingleLine()
			if !compact {
				ui.Text(c, "Space or Resume continues").FontSize(m.bodySize).
					TextColor(spikeMuted).SingleLine()
			}
		})
	})
}

// drawerBackdrop dims the rest of the window without taking the pointer, so the
// spike's click evidence keeps reaching the chrome underneath.
func drawerBackdrop(c *ui.Context) {
	ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
		Background(ui.RGBA(0, 0, 0, 0.4)).IgnoreInput(true)
}

// drawerPanel is the channel drawer on the right edge: 28% of the width, at
// least 280 and at most the 420 of the frames.
func drawerPanel(c *ui.Context, a *app, m chromeMetrics) {
	ui.Column(c).Absolute().Right(0).Top(0).Bottom(0).Width(m.drawerW).
		Gap(m.gap).Background(spikePanel).Border(1, spikeHairline).
		Padding(m.drawerPad).Children(func() {
		drawerHeader(c, a, m)
		ui.Box(c).FillWidth().Height(1).Background(spikeHairline)
		ui.Column(c).Gap(max(6, 10*m.scale)).Children(func() {
			channelRow(c, a, m, "Fixture A", "synthetic HLS")
			channelRow(c, a, m, "Fixture B", "fixture MP4")
			channelRow(c, a, m, "Fixture C", "synthetic HLS")
			channelRow(c, a, m, "Fixture D", "fixture MP4")
		})
		ui.Box(c).Grow(1).Height(1)
		ui.Text(c, "Esc closes · H hides chrome").FontSize(m.bodySize).
			TextColor(spikeMuted).SingleLine()
	})
}

// drawerHeader names the drawer and offers the same close action as Escape.
func drawerHeader(c *ui.Context, a *app, m chromeMetrics) {
	ui.Row(c).FillWidth().Gap(m.gap).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, "Channels").Bold().FontSize(max(14, 20*m.scale)).
				TextColor(spikeText).SingleLine()
			ui.Text(c, "spike fixtures · not a catalog").FontSize(m.bodySize).
				TextColor(spikeMuted).SingleLine()
		})
		ui.ButtonBase(c).Label("Close drawer").Tooltip("Close (Esc)").
			Height(max(24, 30*m.scale)).PaddingX(max(8, 12*m.scale)).
			Radius(max(6, 8*m.scale)).Background(spikeFaint).
			Cursor(ui.CursorPointer).Shrink(0).OnClick(func() {
			a.do.Hit("drawer close")
			a.do.ToggleDrawer()
		}).Children(func() {
			ui.Text(c, "Close").FontSize(max(11, 13*m.scale)).
				TextColor(spikeText)
		})
	})
}

// channelRow is one fixture in the drawer. The last clicked channel keeps a
// tomato edge, which is the only channel state the spike has.
func channelRow(c *ui.Context, a *app, m chromeMetrics, name, kind string) {
	active := a.s.LastAction == "channel "+name

	row := ui.ButtonBase(c).Label(name).Padding(max(9, 14*m.scale)).
		Radius(max(8, 10*m.scale)).Gap(max(8, 12*m.scale)).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Shrink(0).
		OnClick(func() { a.do.Hit("channel " + name) })
	if active {
		row = row.Background(ui.RGBA(248, 32, 40, 0.14)).
			Border(1, ui.RGBA(248, 32, 40, 0.5))
	} else {
		row = row.Background(spikeFaint).Border(1, ui.RGBA(255, 255, 255, 0.08))
	}

	row.Children(func() {
		if active {
			ui.Box(c).Width(3).Height(max(24, 32*m.scale)).Radius(2).
				Background(spikeAccent)
		} else {
			ui.Box(c).Width(3).Height(max(24, 32*m.scale)).Radius(2).
				Background(ui.RGBA(255, 255, 255, 0.12))
		}
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Text(c, name).FontSize(max(13, 17*m.scale)).TextColor(spikeText).
				SingleLine()
			ui.Text(c, kind).FontSize(max(11, 13*m.scale)).
				TextColor(spikeMuted).SingleLine()
		})
	})
}

// pipTitle is the slim strip at the top of the floating panel: the window's
// name and the click evidence.
func pipTitle(c *ui.Context, a *app, m chromeMetrics) {
	ui.Row(c).Absolute().Left(8).Right(8).Top(8).Gap(6).
		Background(spikeChip).Radius(m.radius).Padding(5, 8).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, "mumu · PiP").Bold().FontSize(max(10, 12*m.scale)).
			TextColor(spikeText).SingleLine().Grow(1)
		ui.Text(c, "clicks "+itoa(a.s.Clicks)).FontSize(10).
			TextColor(spikeMuted).SingleLine()
	})
}

// pipBar is the slim control strip: Return, pause or resume, Close and the
// volume indicator. Close stops the stream because the spike has no separate
// close-panel action.
func pipBar(c *ui.Context, a *app, m chromeMetrics) {
	ui.Row(c).Absolute().Left(8).Right(8).Bottom(8).Gap(6).
		Background(spikePanel).Radius(m.radius).Padding(8).
		AlignItems(ui.Center).Wrap().Children(func() {
		transportButton(c, m, "Return", true, "Return to the window (P)", func() {
			a.do.Hit("return")
			a.do.MovePiP()
		})
		transportButton(c, m, pauseLabel(a), false, "Space", func() {
			a.do.Hit("pause")
			a.do.Pause()
		})
		transportButton(c, m, "Close", false, "Stop the stream", func() {
			a.do.Hit("stop")
			a.do.Stop()
		})
		muteIndicator(c, a, m)
	})
}

// muteIndicator is the temporary stand-in for a mute control: it reports the
// engine volume, and says "muted" when the volume is zero.
func muteIndicator(c *ui.Context, a *app, m chromeMetrics) {
	label := itoa(a.s.Volume) + "%"
	if a.s.Volume == 0 {
		label = "muted"
	}

	ui.Row(c).Height(m.controlH).PaddingX(max(8, 12*m.scale)).
		Radius(m.radius).Background(spikeChip).Border(1, spikeHairline).
		AlignItems(ui.Center).Shrink(0).Label("Volume " + label).
		Children(func() {
			ui.Text(c, label).FontSize(max(10, 12*m.scale)).
				TextColor(spikeMuted)
		})
}

// pauseLabel names the pause control for the state the player is in.
func pauseLabel(a *app) string {
	if a.s.Paused {
		return "Resume"
	}

	return "Pause"
}

// statusTint maps a player status to the dot's semantic color.
func statusTint(status string) ui.Color {
	switch status {
	case "playing", "started", "loaded":
		return spikeGood
	case "loading", "buffering":
		return spikeWarn
	case "error", "unavailable", "shutdown":
		return spikeError
	default:
		return spikeMuted
	}
}

// itoa avoids pulling strconv into every view expression.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}

	return string(digits[i:])
}
