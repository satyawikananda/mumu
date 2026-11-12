package ui

import "github.com/egoist/mygo/ui"

// The shell's routes. Watch is a route but not a permanent navigation item.
const (
	destHome     = "/home"
	destWatch    = "/watch"
	destDiscover = "/discover"
	destLibrary  = "/library"
	destSettings = "/settings"
)

// destinationLabel names a route in the shell chrome and tests.
func destinationLabel(id string) string {
	switch id {
	case destHome:
		return "HOME"
	case destWatch:
		return "WATCH"
	case destLibrary:
		return "LIBRARY"
	case destSettings:
		return "SETTINGS"
	default:
		return "DISCOVER"
	}
}

// pageHeading draws a page's level-one heading.
func pageHeading(c *ui.Context, title string) ui.Element {
	t := c.Theme()

	return ui.Text(c, title).Font(displayFont()).FontSize(textSize(t, 34)).
		FontWeight(700).TextColor(t.Text).Role(ui.RoleHeading).Level(1)
}

// pageStatus fills the rest of a page with a status view.
func pageStatus(c *ui.Context, s Status) {
	ui.Column(c).Grow(1).FillWidth().Children(func() { s.View(c) })
}

// drawPage draws the screen for the current destination.
func (s *Shell) drawPage(c *ui.Context, theme *ui.Theme) {
	switch s.nav {
	case destHome:
		s.home.View(c, theme)
	case destWatch:
		s.watch.View(c, theme)
	case destLibrary:
		s.library.View(c, theme)
	case destSettings:
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				pageHeading(c, "Settings")
				s.drawSettings(c, theme)
			})
		})
	default:
		s.discover.View(c, theme)
	}
}

// drawSettings draws appearance controls, catalog sources and the shared
// shortcut reference. #40 owns the final Settings rail and sections.
func (s *Shell) drawSettings(c *ui.Context, theme *ui.Theme) {
	ui.Text(c, "Appearance").Font(displayFont()).FontSize(textSize(theme, 19)).
		FontWeight(700).TextColor(theme.Text)

	choice := int(s.themeMode) - 1
	appearance := ui.Segmented(c, &choice, "Dark", "Light", "System").Label("Appearance").MaxWidth(320)
	if appearance.Changed() {
		s.themeMode = ThemeMode(choice + 1)
	}

	s.settings.Sources(c, theme)

	ui.Text(c, "Shortcuts").Font(displayFont()).FontSize(textSize(theme, 19)).
		FontWeight(700).TextColor(theme.Text)
	shortcutReference(c, theme)

	ui.Column(c).Gap(10).Children(func() {
		settingsRow(c, theme, "Playback", "Playback settings arrive in #37")
		settingsRow(c, theme, "About", "mumu 0.1.0 · M5")
	})
}

// settingsRow draws a static settings line that is not interactive.
func settingsRow(c *ui.Context, theme *ui.Theme, title, detail string) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Text(c, title)
		ui.Text(c, detail).TextColor(theme.TextMuted)
	})
}
