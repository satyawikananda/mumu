package ui

import "github.com/egoist/mygo/ui"

// The shell's destinations and their sidebar IDs.
const (
	destWatch    = "/watch"
	destDiscover = "/discover"
	destLibrary  = "/library"
	destSettings = "/settings"
)

// destination is one screen of the shell.
type destination struct {
	id    string
	label string
}

// destinations lists the shell's screens in sidebar order.
func destinations() [4]destination {
	return [4]destination{
		{destWatch, "Watch"},
		{destDiscover, "Discover"},
		{destLibrary, "Library"},
		{destSettings, "Settings"},
	}
}

// pageHeading draws a page's level-one heading.
func pageHeading(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(c.Theme().Rem(2)).FontWeight(600).Role(ui.RoleHeading).Level(1)
}

// pageStatus fills the rest of a page with a status view.
func pageStatus(c *ui.Context, s Status) {
	ui.Column(c).Grow(1).FillWidth().Children(func() { s.View(c) })
}

// drawPage draws the screen for the current destination.
func (s *Shell) drawPage(c *ui.Context, theme *ui.Theme) {
	switch s.nav {
	case destWatch:
		pageHeading(c, "Watch")
		if s.watchChannel != "" {
			pageStatus(c, Status{
				Kind:    StatusEmpty,
				Title:   "Ready to play",
				Message: s.watchChannel + ". Native playback arrives in #11.",
				Action:  "Browse Discover",
				OnAction: func() {
					s.nav = destDiscover
				},
			})

			return
		}
		pageStatus(c, Status{
			Kind:    StatusEmpty,
			Title:   "Nothing playing",
			Message: "Choose a channel in Discover. Playback arrives in #11.",
			Action:  "Browse Discover",
			OnAction: func() {
				s.nav = destDiscover
			},
		})
	case destLibrary:
		s.library.View(c, theme)
	case destSettings:
		pageHeading(c, "Settings")
		s.drawSettings(c, theme)
	default:
		s.discover.View(c, theme)
	}
}

// drawSettings draws appearance controls, catalog sources and the static
// settings rows.
func (s *Shell) drawSettings(c *ui.Context, theme *ui.Theme) {
	ui.Text(c, "Appearance").FontSize(theme.Rem(1.4)).FontWeight(600)

	choice := int(s.themeMode) - 1
	appearance := ui.Segmented(c, &choice, "Dark", "Light", "System").Label("Appearance").MaxWidth(320)
	if appearance.Changed() {
		s.themeMode = ThemeMode(choice + 1)
	}

	s.settings.Sources(c, theme)

	ui.Column(c).Gap(10).Children(func() {
		settingsRow(c, theme, "Playback", "Watch arrives in #11")
		settingsRow(c, theme, "About", "mumu 0.1.0 · M2")
	})
}

// settingsRow draws a static settings line that is not interactive.
func settingsRow(c *ui.Context, theme *ui.Theme, title, detail string) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Text(c, title)
		ui.Text(c, detail).TextColor(theme.TextMuted)
	})
}
