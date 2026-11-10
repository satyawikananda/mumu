package ui

import (
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// watchVideoColor is the letterbox behind the native video surface.
var watchVideoColor = ui.Hex("#09090B")

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
)

// WatchChannel is one row of the channel drawer.
type WatchChannel struct {
	Key     string
	Name    string
	Meta    string
	Current bool
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
	Channels  []WatchChannel
}

// Watch is the native Watch page. It draws status and controls around a
// native video surface that the app positions over its video box.
type Watch struct {
	state      WatchState
	list       ui.ListState
	selected   int
	video      ui.Handle
	overlay    ui.Handle
	frame      Rect
	volume     float64
	drawerOpen bool

	// OnFrame reports the video box in window points whenever it changes.
	OnFrame func(Rect)
	// OnResume starts the last channel after an explicit click.
	OnResume func()
	// OnRetry reloads the active channel.
	OnRetry func()
	// OnBack returns to the channel that played before the failed switch.
	OnBack func()
	// OnBrowse sends the viewer to Discover.
	OnBrowse func()
	// OnVolume changes the player volume.
	OnVolume func(volume int)
	// OnToggleMute changes the mute state.
	OnToggleMute func()
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
}

// NewWatch returns an idle Watch page.
func NewWatch() *Watch {
	w := &Watch{
		state:    WatchState{Kind: WatchEmpty},
		selected: -1,
	}
	w.list.Selected = &w.selected
	w.list.Label = w.rowLabel

	return w
}

// SetState shows state; an empty volume keeps the current one.
func (w *Watch) SetState(state WatchState) {
	w.state = state
	if w.selected >= len(state.Channels) {
		w.selected = -1
	}
	if state.Volume != 0 || !state.Muted {
		w.volume = float64(state.Volume)
	}
}

// State returns what the page shows, for the app and for tests.
func (w *Watch) State() WatchState { return w.state }

// DrawerOpen reports whether the channel drawer is open.
func (w *Watch) DrawerOpen() bool { return w.drawerOpen }

// ToggleDrawer opens or closes the channel drawer.
func (w *Watch) ToggleDrawer() { w.drawerOpen = !w.drawerOpen }

// View draws the page on the MyGo main thread.
func (w *Watch) View(c *ui.Context, theme *ui.Theme) {
	w.header(c, theme)
	ui.Row(c).Grow(1).FillWidth().Gap(12).Children(func() {
		w.stage(c)
		if w.drawerOpen {
			w.drawer(c, theme)
		}
	})
	w.controls(c, theme)
	w.reportFrame(c)
}

// header draws the live format badge, channel identity and favorite action.
func (w *Watch) header(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		if w.state.Live {
			ui.Text(c, "LIVE").FontSize(theme.Rem(0.8)).FontWeight(600).
				TextColor(theme.TextMuted).Padding(2, 7).Radius(4).
				Background(theme.Surface).Border(1, theme.Border)
		}

		name := strings.TrimSpace(w.state.Channel)
		if name == "" {
			name = "Watch"
		}
		ui.Text(c, name).FontSize(theme.Rem(1.4)).FontWeight(600).
			Role(ui.RoleHeading).Level(1).Grow(1).SingleLine()

		if w.state.Source != "" {
			ui.Text(c, w.state.Source).FontSize(theme.Rem(0.85)).
				TextColor(theme.TextMuted).SingleLine()
		}

		if w.state.Channel != "" {
			label := "Add to Favorites"
			if w.state.Favorite {
				label = "Remove from Favorites"
			}
			ui.Button(c, label).AccessSelected(w.state.Favorite).OnClick(func() {
				if w.OnToggleFavorite != nil {
					w.OnToggleFavorite()
				}
			})
		}
	})
}

// stage draws the video box and whatever covers it while nothing plays.
//
// The native surface is a sibling view above MyGo's layer, so it can only
// cover what MyGo draws. The app detaches it for every state below, which
// is what makes these panels visible at all.
func (w *Watch) stage(c *ui.Context) {
	box := ui.Box(c).Grow(1).FillHeight().Radius(10).Background(watchVideoColor).
		Bind(&w.video)
	box.Children(func() {
		switch w.state.Kind {
		case WatchEmpty:
			w.placeholder(c, Status{
				Kind:     StatusEmpty,
				Title:    "Nothing playing",
				Message:  "Choose a channel in Discover to start watching.",
				Action:   "Browse Discover",
				OnAction: w.browse,
			})
		case WatchResume:
			w.placeholder(c, Status{
				Kind:    StatusEmpty,
				Title:   "Resume " + w.state.Channel,
				Message: "Your last channel. Playback starts only when you choose Resume.",
				Action:  "Resume",
				OnAction: func() {
					if w.OnResume != nil {
						w.OnResume()
					}
				},
			})
		case WatchLoading:
			w.overlayPanel(c, overlayPanel{
				kind:    StatusLoading,
				title:   w.loadingTitle(),
				message: "The stream is starting. You can switch channels at any time.",
			})
		case WatchError:
			w.overlayPanel(c, overlayPanel{
				kind:        StatusError,
				title:       "Stream unavailable",
				message:     w.state.Error,
				action:      "Retry",
				onAction:    w.retry,
				secondary:   w.backLabel(),
				onSecondary: w.back,
			})
		case WatchStopped:
			w.placeholder(c, Status{
				Kind:     StatusEmpty,
				Title:    "Playback stopped",
				Message:  "Choose another channel to keep watching.",
				Action:   "Browse Discover",
				OnAction: w.browse,
			})
		}
	})
}

// loadingTitle names the channel that is starting.
func (w *Watch) loadingTitle() string {
	name := strings.TrimSpace(w.state.Channel)
	if name == "" {
		return "Loading"
	}

	return "Loading " + name
}

// backLabel names the channel a failed switch can return to.
func (w *Watch) backLabel() string {
	name := strings.TrimSpace(w.state.Previous)
	if name == "" {
		return ""
	}

	return "Back to " + name
}

// back dispatches the return intent, when there is somewhere to return to.
func (w *Watch) back() {
	if w.state.Previous == "" || w.OnBack == nil {
		return
	}

	w.OnBack()
}

// placeholder centers a status view inside the video area.
func (w *Watch) placeholder(c *ui.Context, s Status) {
	s.View(c)
}

// overlayPanel is the glass panel that covers the video box while the native
// surface is detached.
type overlayPanel struct {
	kind        StatusKind
	title       string
	message     string
	action      string
	onAction    func()
	secondary   string
	onSecondary func()
}

// overlayPanel floats a centered panel over the empty video box. The glass
// samples the charcoal stage behind it, never a live frame: the app has
// already detached the surface in these states.
func (w *Watch) overlayPanel(c *ui.Context, p overlayPanel) {
	t := c.Theme()

	ui.Box(c).Absolute().Fill().Center().Children(func() {
		ui.Column(c).Bind(&w.overlay).Padding(26, 30).Gap(14).Radius(18).
			AlignItems(ui.Center).MaxWidth(480).
			Material(glass.Glass{}).Border(1, t.Border).Children(func() {
			switch p.kind {
			case StatusLoading:
				ui.Spinner(c).Size(22, 22).Label("Loading")
			case StatusError:
				ui.Text(c, "!").FontSize(t.Rem(1.5)).Bold().
					TextColor(t.Danger).AccessHidden(true)
			}

			titleColor := t.Text
			if p.kind == StatusError {
				titleColor = t.Danger
			}
			ui.Text(c, p.title).FontSize(t.Rem(1.35)).FontWeight(600).
				TextColor(titleColor).TextAlign(ui.Center)

			if p.message != "" {
				ui.Text(c, p.message).TextColor(t.TextMuted).
					TextAlign(ui.Center).MaxWidth(400)
			}

			if p.action == "" && p.secondary == "" {
				return
			}
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if p.action != "" && p.onAction != nil {
					ui.PrimaryButton(c, p.action).OnClick(p.onAction)
				}
				if p.secondary != "" && p.onSecondary != nil {
					ui.Button(c, p.secondary).OnClick(p.onSecondary)
				}
			})
		})
	})
}

// drawer draws the collapsible channel list beside the video.
func (w *Watch) drawer(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Width(280).FillHeight().Padding(10).Gap(8).
		Background(theme.Surface).Radius(10).Border(1, theme.Border).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Channels").FontSize(theme.Rem(1.08)).FontWeight(600).Grow(1)
			ui.Button(c, "Close").Label("Close channel list").OnClick(func() {
				w.drawerOpen = false
			})
		})

		if len(w.state.Channels) == 0 {
			ui.Text(c, "No channels in this catalog.").TextColor(theme.TextMuted)
			return
		}

		list := ui.List(c, &w.list, len(w.state.Channels), func(i int) {
			w.drawerRow(c, theme, i)
		}).Grow(1).Label("Channels").
			ItemKey(func(i int) any { return w.state.Channels[i].Key })
		if list.Submitted() {
			w.selectChannel()
		}
	})
}

// drawerRow draws one switchable channel.
func (w *Watch) drawerRow(c *ui.Context, theme *ui.Theme, i int) {
	channel := w.state.Channels[i]

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(6, 8).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, channel.Name).SingleLine().FontWeight(500)
			if channel.Meta != "" {
				ui.Text(c, channel.Meta).FontSize(theme.Rem(0.85)).
					TextColor(theme.TextMuted).SingleLine()
			}
		})
		if channel.Current {
			ui.Text(c, "Playing").FontSize(theme.Rem(0.8)).
				TextColor(theme.TextMuted)
		}
	})
}

// controls draws status and the transport controls below the video.
func (w *Watch) controls(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().Gap(8).Padding(6, 0, 0, 0).Children(func() {
		if w.state.Kind == WatchBuffering {
			w.bufferingRow(c, theme)
		}

		if status := w.statusText(); status != "" {
			ui.Text(c, status).FontSize(theme.Rem(0.85)).
				TextColor(theme.TextMuted).SingleLine()
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			mute := "Mute"
			if w.state.Muted {
				mute = "Unmute"
			}
			ui.Button(c, mute).OnClick(func() {
				if w.OnToggleMute != nil {
					w.OnToggleMute()
				}
			})

			volume := ui.Slider(c, &w.volume, 0, 100).Width(140).Label("Volume")
			if volume.Changed() {
				if w.OnVolume != nil {
					w.OnVolume(int(w.volume))
				}
			}

			ui.Spacer(c)

			channels := "Channels"
			if w.drawerOpen {
				channels = "Hide Channels"
			}
			ui.Button(c, channels).OnClick(w.ToggleDrawer).
				AccessSelected(w.drawerOpen)

			pip := ui.Button(c, "Picture-in-Picture").OnClick(func() {
				if w.OnEnterPiP != nil {
					w.OnEnterPiP()
				}
			})
			if !w.canEnterPiP() {
				pip.Disabled(true).Tooltip("Available while a channel is playing")
			}

			ui.Button(c, "Fullscreen").OnClick(func() {
				if w.OnFullscreen != nil {
					w.OnFullscreen()
				}
			})
		})
	})
}

// canEnterPiP reports whether the mini player is meaningful now.
func (w *Watch) canEnterPiP() bool {
	switch w.state.Kind {
	case WatchLoading, WatchPlaying, WatchBuffering:
		return !w.state.PiPActive
	default:
		return false
	}
}

// bufferingRow reports a stall under the video. It stays outside the video
// box on purpose: the attached surface would cover anything drawn inside it.
func (w *Watch) bufferingRow(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Spinner(c).Size(14, 14).Label("Buffering")
		ui.Text(c, "Buffering").FontSize(theme.Rem(0.85)).
			TextColor(theme.TextMuted).SingleLine()

		if detail := strings.TrimSpace(w.state.Status); detail != "" {
			ui.Text(c, detail).FontSize(theme.Rem(0.85)).
				TextColor(theme.TextMuted).SingleLine()
		}
	})
}

// statusText names the real player state, nothing when idle.
func (w *Watch) statusText() string {
	switch w.state.Kind {
	case WatchEmpty, WatchBuffering:
		// Buffering has its own row under the video.
		return ""
	case WatchResume:
		return "Not playing. Resume starts the stream."
	case WatchError:
		return "Playback failed."
	case WatchStopped:
		return "Playback stopped."
	default:
		return w.state.Status
	}
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

// rowLabel names a drawer row for assistive technology and type-select.
func (w *Watch) rowLabel(i int) string {
	if i < 0 || i >= len(w.state.Channels) {
		return ""
	}

	channel := w.state.Channels[i]
	parts := []string{channel.Name}
	if channel.Meta != "" {
		parts = append(parts, channel.Meta)
	}
	if channel.Current {
		parts = append(parts, "playing")
	}

	return strings.Join(parts, ", ")
}

// selectChannel dispatches the drawer choice.
func (w *Watch) selectChannel() {
	if w.selected < 0 || w.selected >= len(w.state.Channels) {
		return
	}
	if w.OnSelectChannel != nil {
		w.OnSelectChannel(w.state.Channels[w.selected].Key)
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
