package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// HomeKind is the state the Home shell shows. The full feed lands in #35.
type HomeKind int

const (
	// HomeEmpty reports nothing to resume.
	HomeEmpty HomeKind = iota + 1
	// HomeResume offers the last channel without autoplay.
	HomeResume
	// HomeActive reports a session that is still playing.
	HomeActive
)

// HomeState is the small Home contract #34 needs. #35 expands it into the
// data-backed feed, rails and favorites.
type HomeState struct {
	Kind    HomeKind
	Channel string
	Source  string
}

// Home is the landing screen when a valid recent channel exists. It never
// creates a player or starts playback by itself.
type Home struct {
	state HomeState

	// OnResume starts the offered channel after an explicit click.
	OnResume func()
	// OnWatch opens the active Watch screen.
	OnWatch func()
	// OnBrowse sends the viewer to Discover.
	OnBrowse func()
}

// NewHome returns an empty Home screen.
func NewHome() *Home {
	return &Home{state: HomeState{Kind: HomeEmpty}}
}

// SetState shows state.
func (h *Home) SetState(state HomeState) { h.state = state }

// State returns what Home shows, for the app and tests.
func (h *Home) State() HomeState { return h.state }

// Resume dispatches the explicit resume intent.
func (h *Home) Resume() {
	if h.OnResume != nil {
		h.OnResume()
	}
}

// View draws the provisional Home shell.
func (h *Home) View(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())
	t := c.Theme()

	ui.Column(c).Fill().Gap(20).Children(func() {
		pageHeading(c, "Home")

		switch h.state.Kind {
		case HomeResume, HomeActive:
			h.hero(c, theme, layout)
		default:
			ui.Column(c).Grow(1).Children(func() {
				Status{
					Kind:    StatusEmpty,
					Title:   "Nothing to resume",
					Message: "Choose a channel in Discover to start watching.",
					Action:  "Browse Discover",
					OnAction: func() {
						if h.OnBrowse != nil {
							h.OnBrowse()
						}
					},
				}.View(c)
			})
		}

		if h.state.Kind == HomeResume || h.state.Kind == HomeActive {
			ui.Text(c, "Use Control Center for Home, Discover, Library and Settings.").
				Font(bodyFont()).FontSize(textSize(t, 12)).TextColor(t.TextMuted).
				SingleLine()
		}
	})
}

// hero draws the explicit resume or active-session action.
func (h *Home) hero(c *ui.Context, theme *ui.Theme, layout Layout) {
	label := "LAST WATCHED"
	action := "Resume"
	if h.state.Kind == HomeActive {
		label = "NOW PLAYING"
		action = "Open Watch"
	}

	ui.Column(c).FillWidth().Padding(24).Gap(10).Radius(radiusTile).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 10)).
			FontWeight(700).LetterSpacing(0.7).TextColor(theme.TextMuted).SingleLine()
		name := strings.TrimSpace(h.state.Channel)
		if name == "" {
			name = "Unknown channel"
		}
		ui.Text(c, name).Font(displayFont()).
			FontSize(textSize(theme, layout.HeroTitle)).FontWeight(700).
			TextColor(theme.Text).MaxLines(2).Ellipsis("…")
		if source := strings.TrimSpace(h.state.Source); source != "" {
			ui.Text(c, source).Font(bodyFont()).FontSize(textSize(theme, 12)).
				TextColor(theme.TextMuted).SingleLine()
		}

		ui.Row(c).Gap(10).Children(func() {
			ui.PrimaryButton(c, action).OnClick(func() {
				if h.state.Kind == HomeActive {
					if h.OnWatch != nil {
						h.OnWatch()
					}
					return
				}
				h.Resume()
			})
			ui.Button(c, "Other channels").OnClick(func() {
				if h.OnBrowse != nil {
					h.OnBrowse()
				}
			})
		})
	})
}
