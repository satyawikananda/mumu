package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// PiPState is what the app publishes to the floating player.
type PiPState struct {
	// Channel names the playing channel.
	Channel string
	// Status is the player's short state text.
	Status string
	// Muted reflects the shared player.
	Muted bool
	// Error is a credential-free failure message.
	Error string
}

// PiP is the native mini player. It owns no player: the same surface and
// session move into its window.
type PiP struct {
	state PiPState

	// OnReturn moves the video back to the main window.
	OnReturn func()
	// OnClose ends the mini player.
	OnClose func()
	// OnToggleMute changes the shared player's mute state.
	OnToggleMute func()
}

// NewPiP returns an empty mini player.
func NewPiP() *PiP { return &PiP{} }

// SetState shows state.
func (p *PiP) SetState(state PiPState) { p.state = state }

// State returns what the mini player shows, for the app and for tests.
func (p *PiP) State() PiPState { return p.state }

// View draws the mini player on the MyGo main thread.
func (p *PiP) View(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Fill().Gap(0).Background(watchVideoColor).Children(func() {
		// The native surface covers this box; it keeps the 16:9 area reserved.
		ui.Box(c).Grow(1).FillWidth().Background(watchVideoColor)
		p.controls(c, theme)
	})
}

// controls draws the compact strip under the video.
func (p *PiP) controls(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().Height(48).AlignItems(ui.Center).Gap(8).
		Padding(6, 10).Background(theme.Surface).Children(func() {
		mute := "Mute"
		if p.state.Muted {
			mute = "Unmute"
		}
		ui.Button(c, mute).OnClick(func() {
			if p.OnToggleMute != nil {
				p.OnToggleMute()
			}
		})

		ui.Column(c).Grow(1).Gap(0).Children(func() {
			name := strings.TrimSpace(p.state.Channel)
			if name == "" {
				name = "mumu"
			}
			ui.Text(c, name).FontSize(theme.Rem(0.85)).SingleLine()

			status := p.state.Error
			if status == "" {
				status = p.state.Status
			}
			if status != "" {
				ui.Text(c, status).FontSize(theme.Rem(0.75)).
					TextColor(theme.TextMuted).SingleLine()
			}
		})

		ui.Button(c, "Return to Window").OnClick(func() {
			if p.OnReturn != nil {
				p.OnReturn()
			}
		})
		ui.Button(c, "Close").OnClick(func() {
			if p.OnClose != nil {
				p.OnClose()
			}
		})
	})
}
