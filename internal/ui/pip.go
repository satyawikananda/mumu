package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// pipIdleDelay is how long the floating chrome stays after the last
// activity while the picture plays.
const pipIdleDelay = 2 * time.Second

// PiPState is what the app publishes to the floating player.
type PiPState struct {
	// Channel names the playing channel.
	Channel string
	// Status is the player's short state text.
	Status string
	// Error is a credential-free failure message.
	Error string
	// Muted reflects the shared player's temporary PiP mute.
	Muted bool
	// Paused reflects the shared player's pause.
	Paused bool
	// Live marks a live stream format, not verified availability.
	Live bool
	// Quality is the active variant state.
	Quality WatchQuality
	// Recovery is the automatic reconnect state.
	Recovery WatchRecovery
}

// PiP is the native mini player. It owns no player: the same surface and
// session move into its window, and the view draws only chrome.
type PiP struct {
	state PiPState

	// chrome is the top and bottom rows, for the focus that keeps them up.
	chrome ui.Handle

	// Chrome idle state, as Watch's.
	chromeVisible   bool
	lastActivity    time.Time
	activityPending bool
	wasHovered      bool

	// OnReturn moves the video back to the main window.
	OnReturn func()
	// OnClose ends the mini player and stops playback.
	OnClose func()
	// OnToggleMute changes the temporary floating mute.
	OnToggleMute func()
	// OnTogglePause pauses or resumes the shared session.
	OnTogglePause func()
	// OnRetry reloads the active channel after a terminal failure.
	OnRetry func()
}

// NewPiP returns an empty mini player with its chrome showing.
func NewPiP() *PiP {
	return &PiP{chromeVisible: true}
}

// SetState shows state.
func (p *PiP) SetState(state PiPState) {
	previousPaused, previousMuted := p.state.Paused, p.state.Muted

	p.state = state
	if previousPaused != state.Paused || previousMuted != state.Muted {
		// Playback resumed or the temporary mute ended: give the chrome the
		// full idle period from now rather than an old deadline.
		p.activityPending = true
	}
	if p.pinned() {
		p.chromeVisible = true
	}
}

// State returns what the mini player shows, for the app and for tests.
func (p *PiP) State() PiPState { return p.state }

// ChromeVisible reports whether the floating chrome is showing.
func (p *PiP) ChromeVisible() bool { return p.chromeVisible }

// NoteActivity records input at at and reports whether hidden chrome must
// appear again.
func (p *PiP) NoteActivity(at time.Time) (changed bool) {
	if at.IsZero() {
		return false
	}

	p.lastActivity = at
	if !p.chromeVisible {
		p.chromeVisible = true

		return true
	}

	return false
}

// View draws the mini player on the MyGo main thread. The window is
// frameless and 16:9; the picture fills it and the chrome floats over it.
func (p *PiP) View(c *ui.Context, _ *ui.Theme) {
	c.Root().Background(ui.Transparent)
	p.shortcuts(c)
	width, height := c.Size()
	metrics := pipMetricsFor(c, width, height)
	p.trackIdle(c, metrics)

	vt := themeForContext(ThemeDark, c)
	ui.Themed(c, vt).Fill().Background(ui.Transparent).Children(func() {
		// Empty picture area: drag the window, and reveal chrome on entry.
		canvas := ui.Box(c).Absolute().Fill().Background(ui.Transparent).
			DragWindow().ObserveInput(func(ev ui.InputEvent) {
			switch ev.Kind {
			case ui.InputPointerUp, ui.InputPointerMove, ui.InputKeyDown:
				p.activityPending = true
			}
		})
		if hovered := canvas.Hovered(); hovered && !p.wasHovered {
			p.activityPending = true
			p.wasHovered = hovered
		} else {
			p.wasHovered = hovered
		}

		if p.detached() {
			// No frame to show: paint the stage rather than the desktop.
			ui.Box(c).Absolute().Fill().Background(watchBackdropColor).
				IgnoreInput(true).AccessHidden(true)
		}

		// The dim belongs under the chrome, as the approved frames show it;
		// the card is interactive and sits above in a centred band, so it
		// cannot take the pointer from the rows.
		p.stateDim(c)

		if metrics.closeOnly {
			p.stateCard(c, vt, metrics)
			p.closeControl(c, metrics)

			return
		}
		if p.chromeVisible {
			p.chromeLayer(c, vt, metrics)
		}
		p.stateCard(c, vt, metrics)
	})
}

// shortcuts registers the floating window's own keys. This window has no
// shell shortcut registry, so each action is declared on the window itself
// and runs whatever the focus, the chrome state or the playback state is.
// Esc closes and stops, P returns, Space pauses, M mutes, and R retries a
// terminal failure.
func (p *PiP) shortcuts(c *ui.Context) {
	c.OnShortcut(0, ui.KeyEscape, p.close)
	c.OnShortcut(0, ui.KeyP, p.returnToWindow)
	c.OnShortcut(0, ui.KeySpace, p.togglePause)
	c.OnShortcut(0, ui.KeyM, func() {
		if p.OnToggleMute != nil {
			p.OnToggleMute()
		}
	})
	c.OnShortcut(0, ui.KeyR, p.retry)
}

// CanRetry reports whether a failed attempt is waiting for the viewer: a
// playback error, or recovery that used its last attempt.
func (p *PiP) CanRetry() bool {
	return p.state.Error != "" || p.state.Recovery.Terminal
}

// close ends the floating player and the shared playback.
func (p *PiP) close() {
	if p.OnClose != nil {
		p.OnClose()
	}
}

// returnToWindow hands the session back to the main window.
func (p *PiP) returnToWindow() {
	if p.OnReturn != nil {
		p.OnReturn()
	}
}

// retry repeats the last attempt after a terminal failure.
func (p *PiP) retry() {
	if p.CanRetry() && p.OnRetry != nil {
		p.OnRetry()
	}
}

// detached reports that the surface has no picture, so the stage must paint.
func (p *PiP) detached() bool { return p.CanRetry() }

// pinned reports states that keep the chrome up regardless of the idle timer.
func (p *PiP) pinned() bool {
	return p.state.Paused || p.state.Muted || p.state.Error != "" ||
		p.state.Recovery.Pending || p.state.Recovery.Terminal
}

// trackIdle hides the chrome after the idle delay while the picture plays.
func (p *PiP) trackIdle(c *ui.Context, metrics pipMetrics) {
	now := c.Now()
	if p.lastActivity.IsZero() {
		p.lastActivity = now
	}
	if p.activityPending {
		p.activityPending = false
		p.NoteActivity(now)
	}

	if metrics.closeOnly || p.pinned() || p.chrome.FocusWithin(c) {
		p.chromeVisible = true

		return
	}

	remaining := pipIdleDelay - now.Sub(p.lastActivity)
	if remaining <= 0 {
		p.chromeVisible = false

		return
	}
	if p.chromeVisible {
		// Restated every frame: a frame that also runs a transition replaces
		// the window's timer.
		c.After(remaining)
	}
}

// chromeLayer draws the top row, bottom row and their scrims.
func (p *PiP) chromeLayer(c *ui.Context, vt *ui.Theme, metrics pipMetrics) {
	width, _ := c.Size()
	name := strings.TrimSpace(p.state.Channel)
	if name == "" {
		name = "mumu"
	}

	ui.Themed(c, vt).Absolute().Fill().Background(ui.Transparent).
		Bind(&p.chrome).Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(metrics.topScrim).
			IgnoreInput(true).AccessHidden(true).
			LinearGradient(ui.LinearGradient{
				Angle: 180,
				Stops: []ui.GradientStop{
					{Color: ui.RGBA(0, 0, 0, 0.80), At: 0},
					{Color: ui.RGBA(0, 0, 0, 0.36), At: 0.55},
					{Color: ui.Transparent, At: 1},
				},
			})

		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(metrics.bottomScrim).
			IgnoreInput(true).AccessHidden(true).
			LinearGradient(ui.LinearGradient{
				Angle: 180,
				Stops: []ui.GradientStop{
					{Color: ui.Transparent, At: 0},
					{Color: ui.RGBA(0, 0, 0, 0.72), At: 0.6},
					{Color: ui.RGBA(0, 0, 0, 0.88), At: 1},
				},
			})

		ui.Row(c).Absolute().Left(0).Right(0).Top(0).Height(metrics.rowHeight).
			Padding(10, metrics.pad).AlignItems(ui.Center).Gap(8).
			DragWindow().Children(func() {
			p.statePill(c, vt, metrics)
			ui.Text(c, name).Font(bodyFont()).FontSize(metrics.nameSize).
				FontWeight(600).TextColor(videoInk).SingleLine().MaxWidth(width * 0.5)
			ui.Text(c, watchQualityText(p.state.Quality)).Font(bodyFont()).
				FontSize(textSize(vt, 11)).FontWeight(500).TextColor(videoFaint).
				SingleLine()
			ui.Spacer(c)
			p.closeControl(c, metrics)
		})

		ui.Row(c).Absolute().Left(0).Right(0).Bottom(0).Height(metrics.rowHeight).
			Padding(0, metrics.pad, max(float32(6), metrics.pad-4), metrics.pad).
			AlignItems(ui.Center).Gap(metrics.gap).Children(func() {
			p.playControl(c, metrics)
			p.muteControl(c, vt, metrics)
			ui.Spacer(c)
			p.returnControl(c, metrics)
		})
	})
}

// statePill names live, paused or a real failure state.
func (p *PiP) statePill(c *ui.Context, _ *ui.Theme, metrics pipMetrics) {
	text := "LIVE"
	background := paletteFor(true).brandFill
	foreground := videoInk
	dot := videoInk
	switch {
	case p.state.Paused:
		text, background, foreground, dot = "PAUSED", videoRaised, videoMuted, videoFaint
	case p.state.Error != "" || p.state.Recovery.Terminal:
		text, background, foreground, dot = "OFFLINE", videoRaised, videoMuted, paletteFor(true).brand
	case p.state.Recovery.Pending:
		text, background, foreground, dot = "BUFFERING", videoRaised, videoMuted, videoFaint
	case !p.state.Live:
		text, background, foreground, dot = "ON DEMAND", videoRaised, videoMuted, videoFaint
	}

	pill := ui.Row(c).Padding(3, 7).Radius(4).Gap(4).
		AlignItems(ui.Center).Background(background)
	pill.Children(func() {
		ui.Box(c).Size(4, 4).Radius(2).Background(dot).AccessHidden(true)
		ui.Text(c, text).Font(bodyFont()).FontSize(metrics.pillSize).
			FontWeight(700).LetterSpacing(0.6).TextColor(foreground).SingleLine()
	})
}

// playControl pauses or resumes the shared session.
func (p *PiP) playControl(c *ui.Context, metrics pipMetrics) {
	label, icon := "Pause", icons().pause
	if p.state.Paused {
		label, icon = "Play", icons().play
	}
	mediaIconButton(c, mediaAction{
		Key: "pip-play", Label: label, Icon: icon, Size: metrics.icon,
		OnClick: p.togglePause,
	})
}

// muteControl toggles the floating window's temporary mute and spells it out.
func (p *PiP) muteControl(c *ui.Context, vt *ui.Theme, metrics pipMetrics) {
	label, icon := "Mute", icons().volume
	if p.state.Muted {
		label, icon = "Unmute", icons().mute
	}
	mediaIconButton(c, mediaAction{
		Key: "pip-mute", Label: label, Icon: icon, Size: metrics.icon,
		OnClick: p.OnToggleMute,
	})
	if p.state.Muted {
		ui.Text(c, "Muted").Font(bodyFont()).FontSize(textSize(vt, 11)).
			FontWeight(600).TextColor(videoMuted).SingleLine()
	}
}

// returnControl hands the session back to the main window.
func (p *PiP) returnControl(c *ui.Context, metrics pipMetrics) {
	mediaIconButton(c, mediaAction{
		Key: "pip-return", Label: "Return to window", Icon: icons().monitor,
		Size: metrics.icon, OnClick: p.OnReturn,
	})
}

// closeControl ends the floating player.
func (p *PiP) closeControl(c *ui.Context, metrics pipMetrics) {
	closeButton := func() {
		size := metrics.icon
		if metrics.closeOnly {
			size = 14
		}
		mediaIconButton(c, mediaAction{
			Key: "pip-close", Label: "Close Picture in Picture",
			Icon: icons().close, Size: size, OnClick: p.OnClose,
		})
	}

	if metrics.closeOnly {
		// No strip to carry the target: keep a quiet disc so the white glyph
		// stays legible over any picture.
		ui.Row(c).Absolute().Right(metrics.pad).Top(metrics.pad).Children(func() {
			ui.Box(c).Size(32, 32).Radius(16).
				Background(ui.RGBA(0, 0, 0, 0.45)).Center().Children(closeButton)
		})

		return
	}
	closeButton()
}

// stateDim dims the picture for pause and failure without taking input.
func (p *PiP) stateDim(c *ui.Context) {
	switch {
	case p.state.Paused:
		ui.Box(c).Absolute().Fill().Background(videoDim).IgnoreInput(true).AccessHidden(true)
	case p.CanRetry():
		ui.Box(c).Absolute().Fill().Background(ui.RGBA(5, 6, 10, 0.62)).
			IgnoreInput(true).AccessHidden(true)
	}
}

// stateCard draws the interactive pause or failure card above the chrome.
func (p *PiP) stateCard(c *ui.Context, vt *ui.Theme, metrics pipMetrics) {
	switch {
	case p.state.Paused:
		circle := min(float32(52), metrics.rowHeight*0.9)
		if circle < 40 {
			circle = 40
		}
		ui.Row(c).Absolute().Left(0).Right(0).Top(metrics.height * 0.30).
			Center().Children(func() {
			resume := ui.ButtonBase(c).Size(circle, circle).Radius(circle / 2).
				Background(videoInk).Center().Label("Resume").
				Tooltip("Resume the live stream").FocusRingOffset(2)
			resume.Children(func() {
				ui.Icon(c, icons().play).Size(circle*0.38, circle*0.38).
					TextColor(ui.Hex("#0A0A0C")).AccessHidden(true)
			})
			pose := ui.Pose{}
			if resume.Hovered() {
				pose = ui.Pose{Scale: 1.06}
			}
			resume.AnimatePose(pose, hoverMotion)
			if resume.Clicked() {
				p.togglePause()
			}
		})
	case p.state.Recovery.Pending || p.state.Recovery.Terminal || p.state.Error != "":
		p.failureOverlay(c, vt, metrics)
	}
}

// failureOverlay names a lost signal or a terminal failure, never a
// countdown that is no longer running.
func (p *PiP) failureOverlay(c *ui.Context, vt *ui.Theme, metrics pipMetrics) {
	title := "Signal lost"
	meta := p.recoveryStatus()
	if p.state.Error != "" {
		title = "Stream unavailable"
		meta = p.state.Error
	}
	showRetry := p.state.Error != "" || p.state.Recovery.Terminal

	ui.Themed(c, vt).Absolute().Left(0).Right(0).Top(metrics.height*0.24).
		Background(ui.Transparent).AlignItems(ui.Center).
		Gap(8).Padding(0, metrics.pad).Children(func() {
		ui.Icon(c, icons().offline).Size(22, 22).TextColor(videoMuted).
			AccessHidden(true)
		ui.Text(c, title).Font(bodyFont()).FontSize(textSize(vt, 13)).
			FontWeight(600).TextColor(videoInk).SingleLine().
			Role(ui.RoleStatus)
		if meta != "" {
			ui.Text(c, meta).Font(bodyFont()).FontSize(textSize(vt, 11)).
				FontWeight(500).TextColor(videoFaint).SingleLine().
				MaxWidth(max(float32(160), metrics.width-48))
		}
		if showRetry && p.OnRetry != nil {
			ui.Button(c, "Try again").OnClick(p.OnRetry)
		}
	})
}

// recoveryStatus is the honest countdown or its terminal sentence.
func (p *PiP) recoveryStatus() string {
	recovery := p.state.Recovery
	switch {
	case recovery.Pending && recovery.RetryIn > 0:
		return fmt.Sprintf("Retrying in %ds", int(math.Ceil(recovery.RetryIn.Seconds())))
	case recovery.Pending:
		return "Retrying"
	case recovery.Terminal:
		return "Automatic retries stopped."
	default:
		return strings.TrimSpace(p.state.Status)
	}
}

// togglePause dispatches the shared pause intent.
func (p *PiP) togglePause() {
	if p.OnTogglePause != nil {
		p.OnTogglePause()
	}
}

// pipMetrics is the responsive policy of the floating chrome.
type pipMetrics struct {
	width       float32
	height      float32
	closeOnly   bool
	icon        float32
	pad         float32
	gap         float32
	nameSize    float32
	pillSize    float32
	rowHeight   float32
	topScrim    float32
	bottomScrim float32
}

// pipMetricsFor picks one of the three approved sizes from the window width.
func pipMetricsFor(c *ui.Context, width, height float32) pipMetrics {
	vt := themeForContext(ThemeDark, c)
	metrics := pipMetrics{
		width:       width,
		height:      height,
		icon:        19,
		pad:         14,
		gap:         18,
		nameSize:    textSize(vt, 13),
		pillSize:    textSize(vt, 9),
		rowHeight:   54,
		topScrim:    120,
		bottomScrim: 54,
	}
	switch {
	case width >= 560:
	case width >= 440:
		metrics.icon = 16
		metrics.pad = 12
		metrics.gap = 16
		metrics.rowHeight = 46
		metrics.topScrim = 100
		metrics.bottomScrim = 46
	case width >= 360:
		metrics.icon = 14
		metrics.pad = 10
		metrics.gap = 14
		metrics.nameSize = textSize(vt, 12)
		metrics.rowHeight = 40
		metrics.topScrim = 88
		metrics.bottomScrim = 40
	default:
		metrics.closeOnly = true
		metrics.icon = 14
		metrics.pad = 8
	}
	if height < metrics.rowHeight*2 {
		// A window shorter than its two rows keeps the close target alone.
		metrics.closeOnly = true
	}

	return metrics
}
