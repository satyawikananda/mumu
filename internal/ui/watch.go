package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// watchBackdropColor is the opaque stage behind a channel that shows no
// picture, such as the connecting screen or a terminal error.
var watchBackdropColor = ui.Hex("#07080B")

const (
	// watchIdleDelay is how long the transport chrome stays after the last
	// activity while a stream plays.
	watchIdleDelay = 3 * time.Second
	// watchDrawerBatch caps one page of the drawer grid, so a catalog of
	// thousands of rows costs twenty tiles.
	watchDrawerBatch = 20
	// watchDrawerColumns is the approved two-column drawer grid.
	watchDrawerColumns = 2
	// watchDrawerLoadAhead is how near the end the grid asks for the next
	// batch, in DIPs.
	watchDrawerLoadAhead float32 = 32
	// watchDrawerWidth is the approved drawer panel width.
	watchDrawerWidth float32 = 420
)

// Rect is a window-space box that the app converts into a native frame.
type Rect struct{ X, Y, W, H float32 }

// WatchKind is what the Watch page shows around the video.
type WatchKind int

const (
	// WatchEmpty reports no selected channel.
	WatchEmpty WatchKind = iota + 1
	// WatchResume offers the last channel without autoplay.
	WatchResume
	// WatchLoading reports a stream that is starting.
	WatchLoading
	// WatchBuffering reports playback waiting for data.
	WatchBuffering
	// WatchPlaying reports real playback.
	WatchPlaying
	// WatchStopped reports playback that ended.
	WatchStopped
	// WatchError reports a stream that failed.
	WatchError
	// WatchPaused reports a session the viewer paused.
	WatchPaused
)

// WatchChannel is one row of the channel drawer.
type WatchChannel struct {
	Key     string
	Name    string
	Meta    string
	Current bool
	// LogoURL identifies the channel artwork the drawer may already cache.
	LogoURL string
	// Live marks a live format, not verified availability. The drawer shows
	// its status line only when the app sets it.
	Live bool
	// Art is the published logo and tint, empty when the app has none; the
	// drawer then shows the monogram in the same box.
	Art Artwork
}

// WatchQuality is the real variant state of the active episode.
type WatchQuality struct {
	// Selected is the requested label, "auto" for the channel default.
	Selected string
	// Current is the label the player resolved, empty when unknown.
	Current string
	// Known reports that the stream declared its resolution.
	Known bool
	// Fallback reports that the requested label was unavailable.
	Fallback bool
}

// WatchQualityOption is one selectable quality label.
type WatchQualityOption struct {
	// Label is the raw stream label; "auto" means the channel default.
	Label    string
	Selected bool
}

// WatchRecovery is the automatic reconnect state of the active episode.
type WatchRecovery struct {
	Attempt     int
	MaxAttempts int
	Pending     bool
	Terminal    bool
	RetryIn     time.Duration
}

// WatchState is what the app publishes to the Watch page.
type WatchState struct {
	Kind WatchKind
	// Channel is the selected channel name.
	Channel string
	// Source attributes the channel to its playlist.
	Source string
	// Status is the player's short state text.
	Status string
	// Error is a credential-free failure message.
	Error string
	// Previous names the channel to return to after a failure. It is empty
	// when no earlier channel was playing, which hides the secondary action.
	Previous string
	// Live marks a live stream format, not verified availability.
	Live     bool
	Volume   int
	Muted    bool
	Favorite bool
	// PiPActive reports that the one surface is in the floating window.
	PiPActive bool
	// Paused reports a paused session; WatchPaused is the same fact.
	Paused bool
	// Quality is the active variant state.
	Quality WatchQuality
	// Qualities lists Auto plus the labels this channel declares.
	Qualities []WatchQualityOption
	// Recovery is the automatic reconnect state.
	Recovery WatchRecovery
	// Channels are the drawer rows, newest batch first.
	Channels []WatchChannel
}

// Watch is the native Watch page. It draws status and controls over a native
// video surface the app places across the whole window.
type Watch struct {
	state WatchState

	// video is the box the app positions the native surface over.
	video ui.Handle
	// chrome is the transport layer, for the focus that keeps it visible.
	chrome ui.Handle
	frame  Rect
	// reveal is the chevron's window box while the chrome is hidden. Input
	// is matched against it, because revealing the chrome on the press would
	// remove the chevron before its own release could click it.
	reveal Rect

	// volume mirrors the shared player for the slider.
	volume float64

	// drawerOpen keeps the drawer independent of the page's idle chrome.
	drawerOpen   bool
	drawerQuery  string
	drawerFocus  string
	drawerLimit  int
	drawerScroll ui.ScrollState

	// qualityOpen anchors the quality popover.
	qualityOpen bool

	// Chrome idle state. lastActivity is refreshed by input and by the app;
	// trackIdle restates the deadline on each frame that shows the chrome.
	chromeVisible   bool
	lastActivity    time.Time
	activityPending bool
	// pointer is the last pointer position read from the stage, so a move
	// can be told from a frame drawn for another reason.
	pointerX, pointerY float32
	pointerKnown       bool
	// seenVideo reports that this Watch session really presented frames,
	// which tells an initial connect from a switch.
	seenVideo bool

	// mark is the app's decoded brand bitmap for the connecting screen.
	mark *ui.Bitmap
	// hintsFor supplies the shell's scoped shortcut hints.
	hintsFor func(limit int) []Hint

	// OnFrame reports the video box in window points whenever it changes.
	OnFrame func(Rect)
	// OnResume starts the last channel after an explicit click.
	OnResume func()
	// OnRetry reloads the active channel.
	OnRetry func()
	// OnBack returns to the route the viewer came from.
	OnBack func()
	// OnPreviousChannel returns to the channel that played before a failed
	// switch.
	OnPreviousChannel func()
	// OnCancel cancels the initial connecting attempt.
	OnCancel func()
	// OnBrowse sends the viewer to Discover.
	OnBrowse func()
	// OnVolume changes the player volume.
	OnVolume func(volume int)
	// OnToggleMute changes the mute state.
	OnToggleMute func()
	// OnTogglePause pauses or resumes the shared session.
	OnTogglePause func()
	// OnSelectQuality switches to a real variant label.
	OnSelectQuality func(label string)
	// OnFullscreen toggles native fullscreen.
	OnFullscreen func()
	// OnToggleFavorite adds or removes the active channel from favorites.
	OnToggleFavorite func()
	// OnSelectChannel switches to the drawer row with this key.
	OnSelectChannel func(key string)
	// OnEnterPiP moves the video into the floating window.
	OnEnterPiP func()
	// OnReturn moves the video back from the floating window.
	OnReturn func()
	// OnOpenControlCenter opens the shell overlay over the picture.
	OnOpenControlCenter func()
	// OnSettings opens Settings; the shell wires it by default.
	OnSettings func()
}

// NewWatch returns an idle Watch page with its chrome showing.
func NewWatch() *Watch {
	w := &Watch{
		state:         WatchState{Kind: WatchEmpty},
		chromeVisible: true,
		drawerLimit:   watchDrawerBatch,
	}

	return w
}

// SetMark gives the connecting screen the app's decoded brand bitmap.
func (w *Watch) SetMark(mark *ui.Bitmap) { w.mark = mark }

// SetHints supplies the shell's scoped shortcut hints for the hint bar.
func (w *Watch) SetHints(fn func(limit int) []Hint) { w.hintsFor = fn }

// SetState shows state; an empty volume keeps the current one.
func (w *Watch) SetState(state WatchState) {
	previousKind := w.state.Kind
	previousChannel := w.state.Channel
	previousChannels := len(w.state.Channels)

	w.state = state
	if state.Volume != 0 || !state.Muted {
		w.volume = float64(state.Volume)
	}
	if previousChannel != state.Channel {
		w.drawerFocus = ""
		if w.drawerOpen {
			w.ensureDrawerFocus(state.Channels)
		}
	}
	switch state.Kind {
	case WatchEmpty, WatchResume, WatchStopped:
		w.seenVideo = false
	}
	if w.drawerOpen && previousChannels == 0 && len(state.Channels) > 0 {
		w.ensureDrawerFocus(state.Channels)
	}
	switch w.kind() {
	case WatchPlaying, WatchBuffering, WatchPaused:
		w.seenVideo = true
	}
	if previousKind != state.Kind && !w.pinned() {
		// Playback just started or stopped; give the chrome a full idle
		// period rather than an old deadline.
		w.activityPending = true
	}
}

// State returns what the page shows, for the app and for tests.
func (w *Watch) State() WatchState { return w.state }

// kind normalises the published state: a paused player is a paused page.
func (w *Watch) kind() WatchKind {
	if w.state.Kind == WatchPlaying && w.state.Paused {
		return WatchPaused
	}

	return w.state.Kind
}

// Paused reports whether the session is paused.
func (w *Watch) Paused() bool { return w.kind() == WatchPaused }

// DrawerOpen reports whether the channel drawer is open.
func (w *Watch) DrawerOpen() bool { return w.drawerOpen }

// ToggleDrawer opens or closes the channel drawer.
func (w *Watch) ToggleDrawer() {
	if !w.drawerOpen {
		w.openDrawer()
		return
	}
	w.CloseDrawer()
}

// openDrawer shows the drawer with a fresh first page.
func (w *Watch) openDrawer() {
	w.drawerOpen = true
	w.drawerQuery = ""
	w.resetDrawerPage()
	w.ensureDrawerFocus(w.state.Channels)
	w.activityPending = true
}

// CloseDrawer closes the drawer and keeps the page's video untouched.
func (w *Watch) CloseDrawer() {
	if !w.drawerOpen {
		return
	}
	w.drawerOpen = false
	w.drawerFocus = ""
	w.drawerLimit = watchDrawerBatch
	w.activityPending = true
}

// resetDrawerPage starts the drawer grid at its first batch and at the top,
// so a new query or a reopened drawer does not keep the old page or offset.
func (w *Watch) resetDrawerPage() {
	w.drawerLimit = watchDrawerBatch
	w.drawerScroll = ui.ScrollState{}
	w.drawerFocus = ""
}

// QualityOpen reports whether the quality menu is showing.
func (w *Watch) QualityOpen() bool { return w.qualityOpen }

// CloseQuality closes the quality menu.
func (w *Watch) CloseQuality() { w.qualityOpen = false }

// ChromeVisible reports whether the transport chrome is showing.
func (w *Watch) ChromeVisible() bool { return w.chromeVisible }

// NoteActivity records input at at and reports whether hidden chrome must
// appear again. The app calls it for presses, keys and scroll; the page also
// calls it for input it sees itself.
func (w *Watch) NoteActivity(at time.Time) (changed bool) {
	if at.IsZero() {
		return false
	}

	w.lastActivity = at
	if !w.chromeVisible {
		w.chromeVisible = true

		return true
	}

	return false
}

// NotePointerMove treats pointer movement as activity: moving the pointer
// shows the chrome at once, and while it shows the move keeps its idle
// deadline moving. The method stays exported because the app's input monitor
// calls it.
func (w *Watch) NotePointerMove(at time.Time) {
	w.NoteActivity(at)
}

// IsInitialLoading reports a first connect with no picture behind it. Escape
// cancels that attempt instead of opening Control Center.
func (w *Watch) IsInitialLoading() bool {
	return w.kind() == WatchLoading && !w.seenVideo
}

// CanRetry reports whether Retry has a real attempt to repeat.
func (w *Watch) CanRetry() bool {
	switch w.kind() {
	case WatchLoading, WatchError:
		return true
	default:
		return false
	}
}

// Retry dispatches the retry intent.
func (w *Watch) Retry() {
	if w.CanRetry() && w.OnRetry != nil {
		w.OnRetry()
	}
}

// Cancel stops an initial connecting attempt.
func (w *Watch) Cancel() {
	if w.OnCancel != nil {
		w.OnCancel()
	}
}

// CanPause reports whether there is a session to pause or resume. A connect
// that has not produced a session yet has none.
func (w *Watch) CanPause() bool {
	switch w.kind() {
	case WatchPlaying, WatchBuffering, WatchPaused:
		return true
	default:
		return false
	}
}

// TogglePause dispatches the shared pause intent.
func (w *Watch) TogglePause() {
	if !w.CanPause() || w.OnTogglePause == nil {
		return
	}
	w.OnTogglePause()
}

// View draws the page on the MyGo main thread.
func (w *Watch) View(c *ui.Context, theme *ui.Theme) {
	c.Root().Background(ui.Transparent)
	w.trackIdle(c)

	stage := ui.Box(c).Fill().Background(ui.Transparent).
		ObserveInput(func(ev ui.InputEvent) {
			switch ev.Kind {
			case ui.InputPointerUp:
				// A release on the idle chevron opens Control Center; any
				// other release is ordinary activity.
				if !w.chromeVisible && pointIn(w.reveal, ev.X, ev.Y) {
					w.activityPending = true
					if w.OnOpenControlCenter != nil {
						w.OnOpenControlCenter()
					}

					return
				}
				w.activityPending = true
			case ui.InputKeyDown, ui.InputScroll:
				w.activityPending = true
			}
		})
	// Reading the position has MyGo build a frame on every pointer move; a
	// changed position, or the pointer arriving over the picture, is a move,
	// and a move reveals the chrome at once.
	if x, y, over := stage.PointerPosition(); over {
		if !w.pointerKnown || x != w.pointerX || y != w.pointerY {
			w.NotePointerMove(c.Now())
		}
		w.pointerX, w.pointerY, w.pointerKnown = x, y, true
	} else {
		w.pointerKnown = false
	}
	if !w.chromeVisible {
		// The idle picture hides the pointer; the frame that reveals the
		// chrome drops this, and MyGo recomputes the cursor from the build.
		stage.Cursor(ui.CursorNone)
	}

	stage.Children(func() {
		ui.Box(c).Fill().Bind(&w.video)
		w.stage(c, theme)
	})
	w.reportFrame(c)
}

// trackIdle hides the chrome after the idle delay while a stream plays. The
// deadline is restated with c.After on each frame that shows the chrome, so a
// frame running a transition cannot drop the window's timer; nothing polls.
func (w *Watch) trackIdle(c *ui.Context) {
	now := c.Now()
	if w.lastActivity.IsZero() {
		w.lastActivity = now
	}
	if w.activityPending {
		w.activityPending = false
		w.NoteActivity(now)
	}

	if !w.canHideChrome(c) {
		w.chromeVisible = true

		return
	}

	remaining := watchIdleDelay - now.Sub(w.lastActivity)
	if remaining <= 0 {
		w.chromeVisible = false

		return
	}
	if w.chromeVisible {
		c.After(remaining)
	}
}

// canHideChrome reports the one state that idles: a playing stream with no
// drawer, no menu, no floating player and no focus in the chrome.
func (w *Watch) canHideChrome(c *ui.Context) bool {
	if w.kind() != WatchPlaying || w.state.PiPActive {
		return false
	}
	if w.drawerOpen || w.qualityOpen {
		return false
	}

	return !w.chrome.FocusWithin(c)
}

// pinned reports states that keep chrome up regardless of the idle timer.
func (w *Watch) pinned() bool {
	if w.kind() != WatchPlaying || w.state.PiPActive {
		return true
	}

	return w.drawerOpen || w.qualityOpen
}

// stage draws the screen for the current kind.
func (w *Watch) stage(c *ui.Context, theme *ui.Theme) {
	switch w.kind() {
	case WatchLoading:
		w.connecting(c, theme)
	case WatchError:
		w.errorScreen(c, theme)
	case WatchEmpty, WatchResume, WatchStopped:
		w.welcomeScreen(c, theme)
	default:
		w.playScreen(c, theme)
	}

	if w.drawerOpen {
		w.drawer(c, theme)
	}
}

// playScreen draws the picture states: playing, paused, buffering and the
// placeholder while the one surface is in the floating window.
func (w *Watch) playScreen(c *ui.Context, _ *ui.Theme) {
	metrics := watchMetricsFor(c)
	vt := themeForContext(ThemeDark, c)

	if w.state.PiPActive {
		w.backdrop(c, vt)
		w.pipPlaceholder(c, vt)
	}

	if w.chromeVisible {
		w.chromeLayer(c, vt, metrics)
	} else {
		w.revealChevron(c, vt)
	}

	// The paused and buffering cards sit above the chrome, in a centred
	// band: a full-window container here would take the pointer from the
	// transport controls below it.
	switch w.kind() {
	case WatchPaused:
		w.pauseOverlay(c, vt, metrics)
	case WatchBuffering:
		w.bufferOverlay(c, vt, metrics)
	}
}

// welcomeScreen draws the states without a picture: empty, resume and
// stopped. It keeps a way back and Settings reachable.
func (w *Watch) welcomeScreen(c *ui.Context, _ *ui.Theme) {
	metrics := watchMetricsFor(c)
	vt := themeForContext(ThemeDark, c)

	w.backdrop(c, vt)
	ui.Themed(c, vt).Absolute().Fill().Background(ui.Transparent).Children(func() {
		w.statusForKind(c)
	})
	// The bar is built last so its controls stay above the centered status.
	w.topBar(c, vt, metrics, false)
}

// statusForKind draws the shared status view for the idle states.
func (w *Watch) statusForKind(c *ui.Context) {
	switch w.state.Kind {
	case WatchResume:
		name := strings.TrimSpace(w.state.Channel)
		Status{
			Kind:    StatusEmpty,
			Title:   "Resume " + name,
			Message: "Your last channel. Playback starts only when you choose Resume.",
			Action:  "Resume",
			OnAction: func() {
				if w.OnResume != nil {
					w.OnResume()
				}
			},
		}.View(c)
	case WatchStopped:
		Status{
			Kind:     StatusEmpty,
			Title:    "Playback stopped",
			Message:  "Choose another channel to keep watching.",
			Action:   "Browse Discover",
			OnAction: w.browse,
		}.View(c)
	default:
		Status{
			Kind:     StatusEmpty,
			Title:    "Nothing playing",
			Message:  "Choose a channel in Discover to start watching.",
			Action:   "Browse Discover",
			OnAction: w.browse,
		}.View(c)
	}
}

// pauseOverlay dims the frozen frame and offers the resume target.
func (w *Watch) pauseOverlay(c *ui.Context, vt *ui.Theme, metrics watchMetrics) {
	ui.Box(c).Absolute().Fill().Background(videoDim).IgnoreInput(true).AccessHidden(true)

	circle := min(float32(76), metrics.height*0.12)
	if circle < 52 {
		circle = 52
	}

	ui.Themed(c, vt).Absolute().Left(0).Right(0).Top(metrics.height*0.36).
		Background(ui.Transparent).AlignItems(ui.Center).Gap(16).
		Padding(0, metrics.pad).Children(func() {
		resume := ui.ButtonBase(c).Size(circle, circle).Radius(circle / 2).
			Background(videoInk).Center().Label("Resume").
			Tooltip("Resume the live stream").FocusRingOffset(2)
		resume.Children(func() {
			ui.Icon(c, icons().play).Size(circle*0.36, circle*0.36).
				TextColor(ui.Hex("#0A0A0C")).AccessHidden(true)
		})
		pose := ui.Pose{}
		if resume.Hovered() {
			pose = ui.Pose{Scale: 1.05}
		}
		resume.AnimatePose(pose, hoverMotion)
		if resume.Clicked() {
			w.TogglePause()
		}

		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "Paused").Font(displayFont()).
				FontSize(textSize(vt, 22)).FontWeight(700).
				TextColor(videoInk).Role(ui.RoleStatus)
			ui.Text(c, "Frame frozen · audio stopped.").Font(bodyFont()).
				FontSize(textSize(vt, 13)).TextColor(videoMuted).
				TextAlign(ui.Center)
		})

		ui.Row(c).MaxWidth(520).Padding(11, 15).Gap(9).Radius(10).
			AlignItems(ui.Center).Background(videoRaised).Children(func() {
			ui.Icon(c, icons().rotateLeft).Size(14, 14).
				TextColor(videoMuted).AccessHidden(true)
			ui.Text(c, "Resume reconnects to live.").Font(bodyFont()).
				FontSize(textSize(vt, 12)).TextColor(videoMuted)
		})
	})
}

// bufferOverlay keeps the last frame and reports the real reconnect attempt.
func (w *Watch) bufferOverlay(c *ui.Context, vt *ui.Theme, metrics watchMetrics) {
	ui.Box(c).Absolute().Fill().Background(videoDim).IgnoreInput(true).AccessHidden(true)

	recovery := w.state.Recovery
	title := "Buffering"
	message := "Waiting for the stream to continue."
	if w.state.Live {
		message = "Signal dropped. Reconnecting to live."
	}
	if recovery.Terminal {
		title = "Signal lost"
		message = "Automatic retries stopped."
	}

	ui.Themed(c, vt).Absolute().Left(0).Right(0).Top(metrics.height*0.32).
		Background(ui.Transparent).AlignItems(ui.Center).Gap(14).
		Padding(0, metrics.pad).Children(func() {
		ui.Spinner(c).Size(40, 40).Label(title)
		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, title).Font(displayFont()).FontSize(textSize(vt, 22)).
				FontWeight(700).TextColor(videoInk).Role(ui.RoleStatus)
			ui.Text(c, message).Font(bodyFont()).FontSize(textSize(vt, 13)).
				TextColor(videoMuted).TextAlign(ui.Center)
		})
		if line := w.recoveryLine(); line != "" {
			ui.Text(c, line).Font(bodyFont()).FontSize(textSize(vt, 11)).
				FontWeight(500).TextColor(videoFaint).TextAlign(ui.Center)
		}
		if recovery.Terminal {
			ui.PrimaryButton(c, "Retry").OnClick(w.retry)
		}
	})
}

// recoveryLine names the attempt and countdown while one is pending. A
// terminal recovery never claims to be retrying.
func (w *Watch) recoveryLine() string {
	recovery := w.state.Recovery
	parts := make([]string, 0, 2)
	if recovery.Attempt > 0 {
		if recovery.MaxAttempts > 0 {
			parts = append(parts, fmt.Sprintf("Attempt %d of %d", recovery.Attempt, recovery.MaxAttempts))
		} else {
			parts = append(parts, fmt.Sprintf("Attempt %d", recovery.Attempt))
		}
	}
	switch {
	case recovery.Pending && recovery.RetryIn > 0:
		parts = append(parts, fmt.Sprintf("retrying in %ds", int(math.Ceil(recovery.RetryIn.Seconds()))))
	case recovery.Pending:
		parts = append(parts, "retrying")
	case recovery.Terminal && recovery.Attempt > 0:
		parts = append(parts, "retries stopped")
	case recovery.Terminal:
		parts = append(parts, "automatic retries stopped")
	}

	return strings.Join(parts, " · ")
}

// pipPlaceholder replaces the picture while the one surface floats.
func (w *Watch) pipPlaceholder(c *ui.Context, vt *ui.Theme) {
	ui.Themed(c, vt).Absolute().Fill().Background(ui.Transparent).Center().Gap(14).
		Padding(48).Children(func() {
		ui.Icon(c, icons().pip).Size(32, 32).TextColor(videoMuted).AccessHidden(true)
		ui.Text(c, "Playing in Picture-in-Picture").Font(displayFont()).
			FontSize(textSize(vt, 22)).FontWeight(700).TextColor(videoInk).
			TextAlign(ui.Center).Role(ui.RoleStatus)
		ui.Text(c, "The picture is in the floating window. Return it here for the full player.").
			Font(bodyFont()).FontSize(textSize(vt, 13)).TextColor(videoMuted).
			TextAlign(ui.Center)
		if w.OnReturn == nil {
			ui.PrimaryButton(c, "Return to window").Disabled(true)
			return
		}
		ui.PrimaryButton(c, "Return to window").OnClick(w.OnReturn)
	})
}

// connecting draws the full-window connecting screen with the real clock.
func (w *Watch) connecting(c *ui.Context, _ *ui.Theme) {
	metrics := watchMetricsFor(c)
	vt := themeForContext(ThemeDark, c)
	now := c.Now()
	clock := now.Format("15:04")

	w.backdrop(c, vt)

	ui.Themed(c, vt).Absolute().Fill().Background(ui.Transparent).Children(func() {
		ui.Row(c).Absolute().Left(metrics.pad).Right(metrics.pad).Top(metrics.top).
			AlignItems(ui.Center).Gap(12).Children(func() {
			BrandLockup(c, w.mark)
			ui.Spacer(c)
			ui.Text(c, clock).Font(bodyFont()).FontSize(textSize(vt, 15)).
				FontWeight(600).TextColor(videoMuted).SingleLine()
		})

		ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Center().
			Gap(22).Padding(metrics.pad * 2).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				tile := min(float32(54), metrics.height*0.08)
				if tile < 44 {
					tile = 44
				}
				ui.Box(c).Size(tile, tile).Radius(12).Center().
					Background(paletteFor(true).surface2).Children(func() {
					ui.Text(c, artworkMonogram(w.channelArt(), w.channelName())).
						Font(bodyFont()).FontSize(textSize(vt, 18)).FontWeight(700).
						TextColor(videoInk).SingleLine()
				})

				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, w.channelName()).Font(displayFont()).
						FontSize(textSize(vt, 24)).FontWeight(600).
						TextColor(videoInk).SingleLine().MaxWidth(420).
						Role(ui.RoleHeading).Level(1)
					if sub := w.connectingDetail(); sub != "" {
						ui.Text(c, sub).Font(bodyFont()).FontSize(textSize(vt, 13)).
							TextColor(videoMuted).SingleLine()
					}
				})
			})

			ui.Spinner(c).Size(34, 34).Label("Connecting")
			ui.Text(c, "Connecting to stream…").Font(bodyFont()).
				FontSize(textSize(vt, 15)).FontWeight(500).TextColor(videoMuted).
				Role(ui.RoleStatus)

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if w.IsInitialLoading() {
					ui.PrimaryButton(c, "Cancel").OnClick(w.Cancel)
				}
				ui.Button(c, "Other channels").OnClick(w.ToggleDrawer)
				ui.Button(c, "Try again").OnClick(w.retry).
					Disabled(!w.CanRetry())
			})
		})

		ui.Box(c).Absolute().Left(0).Right(0).Bottom(24).Children(func() {
			HintBar{Hints: w.connectingHints()}.View(c)
		})
	})

	// Keep the clock honest while the connection takes a while. Restated
	// every frame, as the idle deadline is, so another frame's timer cannot
	// drop it.
	c.After(now.Truncate(time.Minute).Add(time.Minute).Sub(now))
}

// connectingHints returns the connecting screen's own actions.
func (w *Watch) connectingHints() []Hint {
	hints := make([]Hint, 0, 3)
	if w.IsInitialLoading() {
		hints = append(hints, Hint{Key: "Esc", Action: "Cancel"})
	}
	hints = append(hints,
		Hint{Key: "C", Action: "Other channels"},
		Hint{Key: "R", Action: "Try again"},
	)

	return hints
}

// connectingDetail attributes the stream without inventing metadata.
func (w *Watch) connectingDetail() string {
	return joinMeta(w.state.Source, w.qualityLine())
}

// errorScreen draws a terminal failure, never a stale frame claim.
func (w *Watch) errorScreen(c *ui.Context, _ *ui.Theme) {
	metrics := watchMetricsFor(c)
	vt := themeForContext(ThemeDark, c)

	w.backdrop(c, vt)

	message := strings.TrimSpace(w.state.Error)
	if message == "" {
		message = "The stream did not start. Try again or pick another channel."
	}

	ui.Themed(c, vt).Absolute().Fill().Background(ui.Transparent).Center().Gap(16).
		Padding(metrics.pad).Children(func() {
		circle := min(float32(76), metrics.height*0.12)
		if circle < 52 {
			circle = 52
		}
		ui.Box(c).Size(circle, circle).Radius(circle / 2).Center().
			Background(videoRaised).Children(func() {
			ui.Icon(c, icons().alert).Size(26, 26).
				TextColor(videoMuted).AccessHidden(true)
		})

		ui.Column(c).AlignItems(ui.Center).Gap(6).MaxWidth(560).Children(func() {
			ui.Text(c, "Stream unavailable").Font(displayFont()).
				FontSize(textSize(vt, 24)).FontWeight(700).TextColor(videoInk).
				TextAlign(ui.Center).Role(ui.RoleHeading).Level(1)
			ui.Text(c, message).Font(bodyFont()).FontSize(textSize(vt, 13)).
				TextColor(videoMuted).TextAlign(ui.Center).MaxLines(3)
		})

		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.PrimaryButton(c, "Retry").OnClick(w.retry)
			ui.Button(c, "Pick another channel").OnClick(w.pickAnother)
			if label := w.previousLabel(); label != "" {
				ui.Button(c, label).OnClick(w.previousChannel)
			}
		})

		if line := w.recoveryLine(); line != "" {
			ui.Text(c, line).Font(bodyFont()).FontSize(textSize(vt, 11)).
				FontWeight(500).TextColor(videoFaint).TextAlign(ui.Center)
		}
	})

	// The bar is built last so its controls stay above the centered panel.
	w.topBar(c, vt, metrics, true)
}

// previousLabel names the channel a failed switch can return to.
func (w *Watch) previousLabel() string {
	name := strings.TrimSpace(w.state.Previous)
	if name == "" {
		return ""
	}

	return "Back to " + name
}

// previousChannel dispatches the return-to-previous intent.
func (w *Watch) previousChannel() {
	if w.state.Previous == "" || w.OnPreviousChannel == nil {
		return
	}
	w.OnPreviousChannel()
}

// pickAnother opens the drawer when it has rows, else Discover.
func (w *Watch) pickAnother() {
	if w.HasChannels() {
		w.openDrawer()

		return
	}
	w.browse()
}

// chromeLayer draws the scrims and the transport controls over the picture.
func (w *Watch) chromeLayer(c *ui.Context, vt *ui.Theme, metrics watchMetrics) {
	ui.Themed(c, vt).Absolute().Fill().Background(ui.Transparent).
		Bind(&w.chrome).Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(metrics.topScrim).
			IgnoreInput(true).AccessHidden(true).
			LinearGradient(ui.LinearGradient{
				Angle: 180,
				Stops: []ui.GradientStop{
					{Color: ui.RGBA(0, 0, 0, 0.85), At: 0},
					{Color: ui.RGBA(0, 0, 0, 0.40), At: 0.45},
					{Color: ui.Transparent, At: 1},
				},
			})

		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(metrics.bottomScrim).
			IgnoreInput(true).AccessHidden(true).
			LinearGradient(ui.LinearGradient{
				Angle: 180,
				Stops: []ui.GradientStop{
					{Color: ui.Transparent, At: 0},
					{Color: ui.RGBA(0, 0, 0, 0.70), At: 0.35},
					{Color: ui.RGBA(0, 0, 0, 0.95), At: 1},
				},
			})

		w.topBar(c, vt, metrics, true)
		w.bottomChrome(c, vt, metrics)
	})
}

// chromeInsets names where the chrome stops. An open drawer takes the right
// side of the picture for itself, so the bar, live line and controls reflow
// into the remaining width instead of running under the panel.
func (w *Watch) chromeInsets(c *ui.Context, metrics watchMetrics) (left, right float32) {
	left, right = metrics.pad, metrics.pad
	if !w.drawerOpen {
		return left, right
	}
	width, _ := c.Size()
	panel := min(watchDrawerWidth, width*0.9)
	if width-panel-2*metrics.pad >= 420 {
		right += panel
	}

	return left, right
}

// topBar draws back, identity, the honest state pill and the way out.
func (w *Watch) topBar(c *ui.Context, vt *ui.Theme, metrics watchMetrics, full bool) {
	_, right := w.chromeInsets(c, metrics)
	ui.Row(c).Absolute().Left(metrics.pad).Right(right).Top(metrics.top).
		AlignItems(ui.Center).Gap(metrics.gap).Children(func() {
		mediaIconButton(c, mediaAction{
			Key: "watch-back", Label: "Back", Icon: icons().arrowLeft,
			Size: metrics.icon, OnClick: w.OnBack,
		})

		ui.Text(c, w.channelName()).Font(displayFont()).FontSize(metrics.title).
			FontWeight(600).TextColor(videoInk).SingleLine().MaxWidth(metrics.titleMax).
			Role(ui.RoleHeading).Level(1)

		if full && w.state.Channel != "" {
			w.statePill(c, vt)
			w.qualityTag(c, vt)
		}

		ui.Spacer(c)

		mediaIconButton(c, mediaAction{
			Key: "watch-control-center", Label: "Control Center",
			Icon: icons().panelLeft, Size: metrics.icon - 2,
			OnClick: w.OnOpenControlCenter,
		})
		mediaIconButton(c, mediaAction{
			Key: "watch-settings", Label: "Settings",
			Icon: icons().settings, Size: metrics.icon - 2, OnClick: w.OnSettings,
		})
	})
}

// statePill names the real playback state, never a verification claim.
func (w *Watch) statePill(c *ui.Context, vt *ui.Theme) {
	text := "LIVE"
	background := paletteFor(true).brandFill
	foreground := videoInk
	dot := videoInk
	switch w.kind() {
	case WatchPaused:
		text, background, foreground = "PAUSED", videoRaised, videoMuted
		dot = videoFaint
	case WatchBuffering:
		text, background, foreground = "BUFFERING", videoRaised, videoMuted
		dot = videoFaint
	case WatchError:
		text, background, foreground = "OFFLINE", videoRaised, videoMuted
		dot = paletteFor(true).brand
	default:
		if !w.state.Live {
			text, background, foreground = "ON DEMAND", videoRaised, videoMuted
			dot = videoFaint
		}
	}

	pill := ui.Row(c).Padding(3, 8).Radius(4).Gap(5).
		AlignItems(ui.Center).Background(background)
	pill.Children(func() {
		ui.Box(c).Size(5, 5).Radius(2.5).Background(dot).AccessHidden(true)
		ui.Text(c, text).Font(bodyFont()).FontSize(textSize(vt, 10)).
			FontWeight(700).LetterSpacing(0.7).TextColor(foreground).SingleLine()
	})
}

// qualityTag names the resolved variant, with the fallback marked.
func (w *Watch) qualityTag(c *ui.Context, vt *ui.Theme) {
	base := w.qualityText()

	ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
		ui.Text(c, base).Font(bodyFont()).FontSize(textSize(vt, 12)).
			FontWeight(500).TextColor(videoMuted).SingleLine()
		if w.state.Quality.Fallback {
			ui.Text(c, "auto").Font(bodyFont()).FontSize(textSize(vt, 10)).
				FontWeight(600).TextColor(videoFaint).SingleLine()
		}
	})
}

// qualityText names the actual variant without inventing one.
func (w *Watch) qualityText() string { return watchQualityText(w.state.Quality) }

// qualityLine names the variant for a meta line, empty when only Auto is
// known: "Auto" belongs in the tag, not in a list of stream facts.
func (w *Watch) qualityLine() string {
	quality := w.state.Quality
	if !quality.Known && strings.TrimSpace(quality.Current) == "" {
		return ""
	}

	return watchQualityText(quality)
}

// watchQualityText names a real variant label for the picture chrome.
func watchQualityText(quality WatchQuality) string {
	if quality.Known && strings.TrimSpace(quality.Current) != "" {
		return strings.TrimSpace(quality.Current)
	}
	if selected := strings.TrimSpace(quality.Selected); selected != "" &&
		!strings.EqualFold(selected, "auto") {
		return selected
	}

	return "Auto"
}

// bottomChrome draws the meta block, live line, controls and hint bar.
func (w *Watch) bottomChrome(c *ui.Context, vt *ui.Theme, metrics watchMetrics) {
	_, right := w.chromeInsets(c, metrics)
	ui.Column(c).Absolute().Left(metrics.pad).Right(right).
		Bottom(metrics.pad).Gap(metrics.gap).Children(func() {
		w.metaBlock(c, vt, metrics)
		w.liveLine(c, vt, metrics)
		w.controls(c, vt, metrics)
		if w.hintsFor != nil {
			width, height := c.Size()
			if hints := w.hintsFor(layoutFor(width, height).HintCount); len(hints) > 0 {
				HintBar{Hints: hints}.View(c)
			}
		}
	})
}

// metaBlock names the channel and its source above the live line.
func (w *Watch) metaBlock(c *ui.Context, _ *ui.Theme, metrics watchMetrics) {
	line := joinMeta(w.state.Source, w.qualityLine())

	ui.Column(c).Gap(4).Children(func() {
		if line != "" {
			ui.Text(c, line).Font(bodyFont()).FontSize(metrics.metaLine).
				TextColor(videoMuted).SingleLine()
		}
		title := w.channelName()
		if w.state.Live {
			title += " · Live broadcast"
		}
		ui.Text(c, title).Font(displayFont()).FontSize(metrics.metaTitle).
			FontWeight(600).TextColor(videoInk).SingleLine().
			Ellipsis("…")
	})
}

// liveLine is the non-interactive live status: a full-width brand line with
// an edge label, never a playhead or an elapsed time.
func (w *Watch) liveLine(c *ui.Context, vt *ui.Theme, _ watchMetrics) {
	line := paletteFor(true).brand
	label := "LIVE"
	labelColor := videoMuted
	dotColor := paletteFor(true).brand
	accessible := "Live stream, playing at the live edge."

	switch w.kind() {
	case WatchPaused:
		line = videoLine
		label = "PAUSED"
		labelColor = videoFaint
		dotColor = videoFaint
		accessible = "Live stream paused. The frame is frozen and audio is stopped."
	case WatchBuffering:
		line = videoLine
		label = "BUFFERING"
		labelColor = videoFaint
		dotColor = videoFaint
		accessible = "Live stream buffering. Reconnecting to the live edge."
	case WatchError:
		line = videoLine
		label = "OFFLINE"
		labelColor = videoFaint
		dotColor = paletteFor(true).brand
		accessible = "Stream offline."
	}
	if w.kind() == WatchPlaying && !w.state.Live {
		label = "ON DEMAND"
		accessible = "Playing on demand."
	}

	ui.Column(c).FillWidth().Gap(2).IgnoreInput(true).
		Role(ui.RoleStatus).Label(accessible).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.End).Gap(5).AlignItems(ui.Center).
			AccessHidden(true).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(dotColor)
			ui.Text(c, label).Font(bodyFont()).FontSize(textSize(vt, 11)).
				FontWeight(700).TextColor(labelColor).SingleLine()
		})
		ui.Box(c).FillWidth().Height(4).Radius(2).Background(line).AccessHidden(true)
	})
}

// controls draws the transport row over the picture.
func (w *Watch) controls(c *ui.Context, vt *ui.Theme, metrics watchMetrics) {
	ui.Row(c).FillWidth().Height(metrics.control).AlignItems(ui.Center).
		Gap(metrics.gap).Children(func() {
		w.playControl(c, metrics)
		w.muteControl(c, metrics)

		slider := ui.Slider(c, &w.volume, 0, 100).Width(metrics.volumeWidth).
			Label("Volume")
		if slider.Changed() && w.OnVolume != nil {
			w.OnVolume(int(w.volume))
		}

		ui.Spacer(c)

		w.favoriteControl(c, metrics)
		w.channelsControl(c, metrics)
		w.qualityControl(c, vt, metrics)
		w.pipControl(c, metrics)
		w.fullscreenControl(c, metrics)
	})
}

// playControl pauses or resumes the shared session.
func (w *Watch) playControl(c *ui.Context, metrics watchMetrics) {
	label, icon := "Pause", icons().pause
	if w.Paused() {
		label, icon = "Play", icons().play
	}
	mediaIconButton(c, mediaAction{
		Key: "watch-play", Label: label, Icon: icon, Size: metrics.icon + 2,
		Disabled: !w.CanPause(), OnClick: w.TogglePause,
	})
}

// muteControl toggles the shared audio intent.
func (w *Watch) muteControl(c *ui.Context, metrics watchMetrics) {
	label, icon := "Mute", icons().volume
	if w.state.Muted {
		label, icon = "Unmute", icons().mute
	}
	mediaIconButton(c, mediaAction{
		Key: "watch-mute", Label: label, Icon: icon, Size: metrics.icon,
		Disabled: !w.CanControl(), OnClick: w.ToggleMute,
	})
}

// favoriteControl is the favorite toggle the acceptance criteria require.
func (w *Watch) favoriteControl(c *ui.Context, metrics watchMetrics) {
	label := "Add to Favorites"
	if w.state.Favorite {
		label = "Remove from Favorites"
	}
	mediaIconButton(c, mediaAction{
		Key: "watch-favorite", Label: label, Icon: icons().heart,
		Size: metrics.icon, Active: w.state.Favorite, ActiveColor: paletteFor(true).brand,
		Disabled: w.state.Channel == "", OnClick: w.ToggleFavorite,
	})
}

// channelsControl opens the drawer and shows when it is open.
func (w *Watch) channelsControl(c *ui.Context, metrics watchMetrics) {
	mediaIconButton(c, mediaAction{
		Key: "watch-channels", Label: "Channels", Icon: icons().listVideo,
		Size: metrics.icon, Active: w.drawerOpen,
		Disabled: !w.HasChannels(), OnClick: w.ToggleDrawer,
	})
}

// qualityControl anchors the real variant menu.
func (w *Watch) qualityControl(c *ui.Context, _ *ui.Theme, metrics watchMetrics) {
	button := mediaIconButton(c, mediaAction{
		Key: "watch-quality", Label: "Quality", Icon: icons().sliders,
		Size: metrics.icon, Active: w.qualityOpen,
		Tooltip:  "Quality · " + w.qualityText(),
		Disabled: w.state.Channel == "",
		OnClick:  func() { w.qualityOpen = !w.qualityOpen },
	})

	ui.Popover(c, button, &w.qualityOpen, func() { w.qualityMenu(c) })
}

// qualityMenu lists Auto plus the labels the channel declared.
func (w *Watch) qualityMenu(c *ui.Context) {
	t := c.Theme()
	options := w.state.Qualities
	if len(options) == 0 {
		options = []WatchQualityOption{{Label: "auto", Selected: true}}
	}

	ui.Column(c).Width(216).Gap(2).Children(func() {
		ui.Text(c, "QUALITY").Font(bodyFont()).FontSize(textSize(t, 10)).
			FontWeight(700).LetterSpacing(0.8).TextColor(t.TextMuted).
			Padding(2, 8, 6, 8)

		for _, option := range options {
			label := option.Label
			if label == "" {
				continue
			}
			name := qualityOptionLabel(label)
			item := ui.ButtonBase(c).FillWidth().Height(32).Padding(4, 8).
				Radius(8).Gap(8).AlignItems(ui.Center).FocusRingOffset(-2).
				Role(ui.RoleOption).Checked(option.Selected).Label(name)
			if item.Hovered() {
				item = item.Background(t.SurfaceHover)
			}
			item.Children(func() {
				ui.Text(c, name).Font(bodyFont()).FontSize(textSize(t, 13)).
					TextColor(t.Text).Grow(1).SingleLine()
				if option.Selected {
					ui.Icon(c, icons().check).Size(14, 14).
						TextColor(t.Accent).AccessHidden(true)
				}
			})
			if item.Clicked() {
				w.selectQuality(label)
			}
		}

		if !w.state.Quality.Known {
			ui.Text(c, "This stream doesn't report its resolution.").
				Font(bodyFont()).FontSize(textSize(t, 11)).
				TextColor(t.TextMuted).Padding(6, 8, 2, 8).MaxLines(2)
		}
		if w.state.Quality.Fallback {
			ui.Text(c, "The requested quality isn't available; playing the stream's default.").
				Font(bodyFont()).FontSize(textSize(t, 11)).
				TextColor(t.TextMuted).Padding(4, 8, 2, 8).MaxLines(3)
		}
	})
}

// selectQuality dispatches one real variant label and closes the menu.
func (w *Watch) selectQuality(label string) {
	w.qualityOpen = false
	w.activityPending = true
	if w.OnSelectQuality != nil {
		w.OnSelectQuality(label)
	}
}

// pipControl enters the floating player or brings the session back.
func (w *Watch) pipControl(c *ui.Context, metrics watchMetrics) {
	label, tip := "Picture in Picture", "Picture in Picture"
	if w.state.PiPActive {
		label, tip = "Return to window", "Return the stream to this window"
	}
	mediaIconButton(c, mediaAction{
		Key: "watch-pip", Label: label, Tooltip: tip, Icon: icons().pip,
		Size: metrics.icon, Active: w.state.PiPActive,
		Disabled: !w.canEnterPiP(), OnClick: w.TogglePiP,
	})
}

// fullscreenControl toggles native fullscreen.
func (w *Watch) fullscreenControl(c *ui.Context, metrics watchMetrics) {
	mediaIconButton(c, mediaAction{
		Key: "watch-fullscreen", Label: "Fullscreen", Icon: icons().fullscreen,
		Size: metrics.icon, Disabled: !w.CanControl(), OnClick: w.ToggleFullscreen,
	})
}

// revealChevron is the quiet way back to the controls while they are hidden.
func (w *Watch) revealChevron(c *ui.Context, vt *ui.Theme) {
	width, height := c.Size()
	const chevronWidth, chevronHeight float32 = 52, 24
	w.reveal = Rect{
		X: width/2 - chevronWidth/2,
		Y: height - 12 - chevronHeight,
		W: chevronWidth,
		H: chevronHeight,
	}

	ui.Themed(c, vt).Absolute().Left(0).Right(0).Bottom(12).
		Background(ui.Transparent).AlignItems(ui.Center).Children(func() {
		button := ui.ButtonBase(c).Width(chevronWidth).Height(chevronHeight).
			Radius(12).Background(videoRaised).Center().
			Label("Open Control Center").Tooltip("Open Control Center").
			FocusRingOffset(-2).
			Transition(ui.ElementTransition{
				Duration: hoverMotion.Duration,
				Enter:    &ui.Motion{Y: 6, Opacity: 0},
				Exit:     &ui.Motion{Y: 6, Opacity: 0},
			})
		if button.Hovered() {
			button = button.Background(ui.Hex("#FFFFFF").Alpha(0.24))
		}
		button.Children(func() {
			ui.Icon(c, icons().chevron).Size(14, 14).TextColor(videoInk).
				Transform(ui.Rotation(180)).AccessHidden(true)
		})
		// A pointer click is handled by the stage's input, before the frame
		// that reveals the chrome. This covers assistive technology.
		if button.Clicked() && w.OnOpenControlCenter != nil {
			w.OnOpenControlCenter()
		}
	})
}

// pointIn reports whether a window point lies in box.
func pointIn(box Rect, x, y float32) bool {
	return x >= box.X && x <= box.X+box.W && y >= box.Y && y <= box.Y+box.H
}

// backdrop paints the opaque stage used whenever the app detached the
// surface, so no state claims a picture it does not have.
func (w *Watch) backdrop(c *ui.Context, vt *ui.Theme) {
	ui.Box(c).Absolute().Fill().Background(watchBackdropColor).
		IgnoreInput(true).AccessHidden(true).Children(func() {
		ui.Box(c).Absolute().Fill().RadialGradient(ui.RadialGradient{
			Center: ui.Point{X: 0.32, Y: 0.18},
			Radius: 900,
			Stops: []ui.GradientStop{
				{Color: paletteFor(true).surface2.Alpha(0.85), At: 0},
				{Color: ui.Transparent, At: 1},
			},
		})
		if name := strings.TrimSpace(w.state.Channel); name != "" {
			ui.Text(c, strings.ToLower(name)).Font(displayFont()).
				FontSize(textSize(vt, 150)).FontWeight(700).
				TextColor(videoInk.Alpha(0.05)).SingleLine().Ellipsis("").
				Absolute().Left(24).Top(150).AccessHidden(true)
		}
	})
}

// drawer draws the channel drawer over the picture without touching video.
func (w *Watch) drawer(c *ui.Context, _ *ui.Theme) {
	vt := themeForContext(ThemeDark, c)
	width, _ := c.Size()
	panelWidth := min(watchDrawerWidth, width*0.9)
	metrics := watchMetricsFor(c)

	ui.Row(c).Absolute().Fill().Children(func() {
		ui.Box(c).Grow(1).FillHeight().Background(ui.RGBA(0, 0, 0, 0.42)).
			IgnoreInput(true).AccessHidden(true)

		ui.Themed(c, vt).Width(panelWidth).FillHeight().Background(videoPanel).
			BorderWidth(1, 0, 0, 0).Border(1, videoDivider).
			Padding(24, 22).Gap(16).Role(ui.RoleGroup).
			Label("Channel drawer").Children(func() {
			w.drawerHead(c, vt)
			w.drawerSearch(c, vt)
			channels := w.visibleChannels()
			w.drawerKeys(c, channels)
			w.drawerGrid(c, vt, metrics, channels)
			w.drawerFooter(c, vt)
		})
	})
}

// drawerHead names the drawer and the source it lists.
func (w *Watch) drawerHead(c *ui.Context, vt *ui.Theme) {
	title := strings.TrimSpace(w.state.Source)
	if title == "" {
		title = "Channels"
	}

	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(10).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "CHANNEL").Font(bodyFont()).FontSize(textSize(vt, 11)).
				FontWeight(700).LetterSpacing(1).TextColor(videoFaint).SingleLine()
			ui.Text(c, title).Font(displayFont()).FontSize(textSize(vt, 20)).
				FontWeight(700).TextColor(videoInk).SingleLine().Ellipsis("…").
				Role(ui.RoleHeading).Level(2)
		})
		mediaIconButton(c, mediaAction{
			Key: "watch-drawer-close", Label: "Close channel drawer",
			Icon: icons().close, Size: 18, OnClick: w.CloseDrawer,
		})
	})
}

// drawerSearch filters by the real published row labels.
func (w *Watch) drawerSearch(c *ui.Context, vt *ui.Theme) {
	field := ui.SearchField(c, &w.drawerQuery).Label("Search channels")
	if field.Changed() {
		w.resetDrawerPage()
	}

	ui.Text(c, w.drawerCount()).Font(bodyFont()).FontSize(textSize(vt, 11)).
		TextColor(videoFaint).SingleLine()
}

// drawerCount is the real size of the result set.
func (w *Watch) drawerCount() string {
	total := len(w.state.Channels)
	if w.drawerQuery == "" {
		return channelCount(total)
	}

	matches := len(w.drawerMatches())
	if matches == 1 {
		return "1 match"
	}

	return fmt.Sprintf("%d matches", matches)
}

// visibleChannels is what the drawer grid shows: the published catalog, or
// its matches while a query is active. Search reuses the same tiles, batches
// and scroll, only over the filtered rows.
func (w *Watch) visibleChannels() []WatchChannel {
	if strings.TrimSpace(w.drawerQuery) == "" {
		return w.state.Channels
	}

	return w.drawerMatches()
}

// drawerMatches filters the drawer rows by name and meta, case-insensitively.
func (w *Watch) drawerMatches() []WatchChannel {
	query := strings.ToLower(strings.TrimSpace(w.drawerQuery))
	if query == "" {
		return w.state.Channels
	}

	matches := make([]WatchChannel, 0, len(w.state.Channels))
	for _, channel := range w.state.Channels {
		if strings.Contains(strings.ToLower(channel.Name), query) ||
			strings.Contains(strings.ToLower(channel.Meta), query) {
			matches = append(matches, channel)
		}
	}

	return matches
}

// drawerGrid draws the approved two-column grid over the visible rows,
// twenty at a time. Search results use the same tiles and the same paging.
func (w *Watch) drawerGrid(
	c *ui.Context,
	vt *ui.Theme,
	metrics watchMetrics,
	channels []WatchChannel,
) {
	if len(channels) == 0 {
		empty := "No channels in this drawer yet."
		if strings.TrimSpace(w.drawerQuery) != "" {
			empty = fmt.Sprintf("No channels match %q.", strings.TrimSpace(w.drawerQuery))
		}
		ui.Text(c, empty).Font(bodyFont()).
			FontSize(textSize(vt, 13)).TextColor(videoMuted).MaxLines(2)
		w.ensureDrawerFocus(channels)

		return
	}

	limit := min(w.drawerLimit, len(channels))
	if limit <= 0 {
		limit = watchDrawerBatch
	}

	// The rows are already local; nearing the end lays out the next batch.
	// One batch per frame, because the grown content moves the end away from
	// the offset, and never before a scroll range exists.
	nearBottom := w.drawerScroll.MaxY > 0 &&
		w.drawerScroll.Y >= w.drawerScroll.MaxY-watchDrawerLoadAhead
	if nearBottom && len(channels) > limit {
		w.drawerLimit = min(w.drawerLimit+watchDrawerBatch, len(channels))
		limit = min(w.drawerLimit, len(channels))
		c.Invalidate()
	}

	ui.Scroll(c).Grow(1).FillWidth().TrackScroll(&w.drawerScroll).Children(func() {
		ui.Column(c).Gap(12).Children(func() {
			ui.Grid(c).Columns(watchDrawerColumns).Gap(12).Children(func() {
				for _, channel := range channels[:limit] {
					w.drawerTile(c, vt, metrics, channel)
				}
			})
		})
	})

	if nearBottom && len(channels) > limit {
		drawerLoading(c, vt)
	}
}

// drawerLoading marks the batch being laid out in place of a button. It
// floats above the footer so it is visible at the point that asked for it,
// takes no pointer and no room, and goes away with the next layout.
func drawerLoading(c *ui.Context, vt *ui.Theme) {
	ui.Row(c).Absolute().Left(22).Right(22).Bottom(56).Height(30).
		AlignItems(ui.Center).Justify(ui.Center).Gap(8).Radius(9).
		Background(ui.Hex("#08090B").Alpha(0.92)).Border(1, videoDivider).
		IgnoreInput(true).Children(func() {
		ui.Spinner(c).Size(14, 14).Label("Loading more channels")
		ui.Text(c, "Loading more channels…").Font(bodyFont()).
			FontSize(textSize(vt, 12)).TextColor(videoFaint).SingleLine()
	})
}

// drawerTile draws one switchable channel in the drawer grid.
func (w *Watch) drawerTile(c *ui.Context, _ *ui.Theme, metrics watchMetrics, channel WatchChannel) {
	height := min(float32(150), metrics.height*0.18)
	if height < 104 {
		height = 104
	}

	ChannelTile{
		Key:        channel.Key,
		Name:       channel.Name,
		Meta:       channel.Meta,
		Live:       channel.Live,
		Drawer:     true,
		Height:     height,
		Art:        channel.Art,
		Selected:   channel.Current,
		FocusField: &w.drawerFocus,
		FocusValue: drawerFocusValue(channel.Key),
		OnActivate: func() { w.selectChannelKey(channel.Key) },
	}.View(c)
}

// drawerFooter repeats the drawer keys where the design puts them.
func (w *Watch) drawerFooter(c *ui.Context, vt *ui.Theme) {
	ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
		drawerHint(c, vt, "↑ ↓", "select")
		drawerHint(c, vt, "⏎", "watch")
		drawerHint(c, vt, "Esc", "close")
	})
}

// drawerHint draws one keycap and its action.
func drawerHint(c *ui.Context, vt *ui.Theme, key, action string) {
	ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
		Keycap{Label: key}.View(c)
		ui.Text(c, action).Font(bodyFont()).FontSize(textSize(vt, 11)).
			TextColor(videoFaint).SingleLine()
	})
}

// drawerKeys moves the grid cursor. It works on the visible rows, browsed or
// filtered, and leaves the keys to the search field while it takes text.
func (w *Watch) drawerKeys(c *ui.Context, channels []WatchChannel) {
	if c.FocusTakesText() || !w.DrawerOpen() {
		return
	}

	limit := min(w.drawerLimit, len(channels))
	if limit <= 0 {
		return
	}

	move := func(delta int) {
		index := w.drawerIndex(channels[:limit])
		next := index + delta
		if index < 0 {
			// No cursor in the current rows: step in from the near end.
			if delta > 0 {
				next = 0
			} else {
				next = limit - 1
			}
		}
		if next < 0 {
			next = 0
		}
		if next >= limit {
			next = limit - 1
		}
		w.drawerFocus = drawerFocusValue(channels[next].Key)
	}

	switch {
	case c.Shortcut(0, ui.KeyDown):
		move(watchDrawerColumns)
	case c.Shortcut(0, ui.KeyUp):
		move(-watchDrawerColumns)
	case c.Shortcut(0, ui.KeyRight):
		move(1)
	case c.Shortcut(0, ui.KeyLeft):
		move(-1)
	}
}

// drawerIndex is the cursor's place in the visible batch.
func (w *Watch) drawerIndex(channels []WatchChannel) int {
	for i, channel := range channels {
		if drawerFocusValue(channel.Key) == w.drawerFocus {
			return i
		}
	}

	return -1
}

// ensureDrawerFocus starts the cursor on the current channel, else the first.
func (w *Watch) ensureDrawerFocus(channels []WatchChannel) {
	if len(channels) == 0 {
		w.drawerFocus = ""

		return
	}
	for _, channel := range channels {
		if channel.Current {
			w.drawerFocus = drawerFocusValue(channel.Key)

			return
		}
	}
	w.drawerFocus = drawerFocusValue(channels[0].Key)
}

// drawerFocusValue scopes a drawer tile's focus identity.
func drawerFocusValue(key string) string {
	if key == "" {
		return ""
	}

	return "drawer:" + key
}

// qualityOptionLabel names the raw label for people.
func qualityOptionLabel(label string) string {
	if strings.EqualFold(strings.TrimSpace(label), "auto") {
		return "Auto"
	}

	return strings.TrimSpace(label)
}

// mediaAction is one icon action drawn over video.
type mediaAction struct {
	Key         string
	Label       string
	Tooltip     string
	Icon        *ui.SVG
	Size        float32
	Active      bool
	ActiveColor ui.Color
	Disabled    bool
	OnClick     func()
}

// mediaIconButton draws a quiet icon button that keeps a visible label for
// assistive technology and a tooltip for the pointer.
func mediaIconButton(c *ui.Context, a mediaAction) ui.Element {
	if a.Size <= 0 {
		a.Size = 20
	}
	if a.Key != "" {
		c = c.Key(a.Key)
	}
	box := a.Size + 16
	tooltip := a.Tooltip
	if tooltip == "" {
		tooltip = a.Label
	}

	button := ui.ButtonBase(c).Size(box, box).Radius(8).Center().
		FocusRingOffset(-2).Label(a.Label).Tooltip(tooltip).
		AccessSelected(a.Active).Disabled(a.Disabled).DimDisabled(false)
	if button.Hovered() && !a.Disabled {
		button = button.Background(videoHover)
	}
	button.Children(func() {
		color := videoInk
		switch {
		case a.Disabled:
			color = videoFaint
		case a.Active && a.ActiveColor.A > 0:
			color = a.ActiveColor
		case a.Active:
			color = videoInk
		}
		ui.Icon(c, a.Icon).Size(a.Size, a.Size).TextColor(color).AccessHidden(true)
	})
	button.Transition(ui.ElementTransition{Colors: true, Duration: hoverMotion.Duration})
	if button.Clicked() && !a.Disabled && a.OnClick != nil {
		a.OnClick()
	}

	return button
}

// watchMetrics is the responsive policy of the video chrome.
type watchMetrics struct {
	pad         float32
	top         float32
	gap         float32
	icon        float32
	title       float32
	titleMax    float32
	metaLine    float32
	metaTitle   float32
	control     float32
	volumeWidth float32
	topScrim    float32
	bottomScrim float32
	height      float32
}

// watchMetricsFor scales the chrome from the two reference dimensions.
func watchMetricsFor(c *ui.Context) watchMetrics {
	width, height := c.Size()
	layout := layoutFor(width, height)
	pad := min(layout.Padding, 48)
	metrics := watchMetrics{
		pad:         pad,
		top:         max(float32(24), height*0.038),
		gap:         16,
		icon:        22,
		title:       22,
		titleMax:    max(240, width*0.42),
		metaLine:    textSize(c.Theme(), 13),
		metaTitle:   textSize(c.Theme(), 20),
		control:     40,
		volumeWidth: 96,
		topScrim:    200,
		bottomScrim: 436,
		height:      height,
	}
	switch layout.Breakpoint {
	case BreakpointWide:
		metrics.gap = 18
	case BreakpointMedium:
		metrics.gap = 16
		metrics.volumeWidth = 88
	default:
		metrics.pad = 28
		metrics.top = 22
		metrics.icon = 20
		metrics.title = 18
		metrics.titleMax = max(180, width*0.40)
		metrics.metaLine = textSize(c.Theme(), 12)
		metrics.metaTitle = textSize(c.Theme(), 17)
		metrics.control = 36
		metrics.volumeWidth = 72
		metrics.topScrim = 150
		metrics.bottomScrim = 330
	}

	return metrics
}

// channelArt is the published drawer art of the active channel, when known.
func (w *Watch) channelArt() Artwork {
	for _, channel := range w.state.Channels {
		if channel.Name == w.state.Channel {
			return channel.Art
		}
	}

	return Artwork{}
}

// channelName names the page without inventing a channel.
func (w *Watch) channelName() string {
	if name := strings.TrimSpace(w.state.Channel); name != "" {
		return name
	}

	return "Watch"
}

// ResetFrame forgets the last reported video box, so a window that was
// destroyed and recreated reports its box again instead of keeping a stale
// one.
func (w *Watch) ResetFrame() { w.frame = Rect{} }

// reportFrame tells the app where the video box is.
func (w *Watch) reportFrame(c *ui.Context) {
	if w.OnFrame == nil {
		return
	}

	box := w.video.Bounds(c)
	frame := Rect{X: box.X, Y: box.Y, W: box.W, H: box.H}
	if frame == w.frame {
		return
	}
	w.frame = frame

	if frame.W <= 0 || frame.H <= 0 {
		return
	}
	w.OnFrame(frame)
}

// selectChannelKey switches to the drawer row with this key.
func (w *Watch) selectChannelKey(key string) {
	if key == "" || w.OnSelectChannel == nil {
		return
	}

	w.activityPending = true
	w.OnSelectChannel(key)
}

// ToggleFavorite dispatches the favorite intent.
func (w *Watch) ToggleFavorite() {
	if w.state.Channel != "" && w.OnToggleFavorite != nil {
		w.OnToggleFavorite()
	}
}

// browse dispatches the Discover intent.
func (w *Watch) browse() {
	if w.OnBrowse != nil {
		w.OnBrowse()
	}
}

// retry dispatches the retry intent.
func (w *Watch) retry() {
	if w.OnRetry != nil {
		w.OnRetry()
	}
}

// CanControl reports whether playback actions have a real active session.
func (w *Watch) CanControl() bool {
	switch w.kind() {
	case WatchLoading, WatchBuffering, WatchPlaying, WatchPaused:
		return true
	default:
		return false
	}
}

// canEnterPiP reports whether the mini player is meaningful now: there must
// be a session with a picture to move.
func (w *Watch) canEnterPiP() bool {
	return w.state.PiPActive || w.CanPause()
}

// HasChannels reports whether the channel drawer has rows.
func (w *Watch) HasChannels() bool { return len(w.state.Channels) > 0 }

// ToggleMute dispatches the mute intent.
func (w *Watch) ToggleMute() {
	if w.OnToggleMute != nil {
		w.OnToggleMute()
	}
}

// AdjustVolume changes the volume by delta, clamped to 0..100.
func (w *Watch) AdjustVolume(delta int) {
	if !w.CanControl() || w.OnVolume == nil {
		return
	}
	volume := int(w.volume) + delta
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	w.volume = float64(volume)
	w.OnVolume(volume)
}

// ToggleFullscreen dispatches the native fullscreen intent.
func (w *Watch) ToggleFullscreen() {
	if w.OnFullscreen != nil {
		w.OnFullscreen()
	}
}

// TogglePiP enters PiP or returns to the main window, one session only.
func (w *Watch) TogglePiP() {
	if w.state.PiPActive {
		if w.OnReturn != nil {
			w.OnReturn()
		}

		return
	}
	if w.CanControl() && w.OnEnterPiP != nil {
		w.OnEnterPiP()
	}
}
