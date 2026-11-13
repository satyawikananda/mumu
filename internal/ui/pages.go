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
		s.settings.View(c, theme)
	default:
		s.discover.View(c, theme)
	}
}
