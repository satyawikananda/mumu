// Package ui holds mumu's native application shell.
package ui

import "github.com/egoist/mygo/ui"

const (
	wideMinWidth     = 860
	wideSidebarWidth = 208
	railWidth        = 72
	railItemSize     = 48
)

// Shell is the native application shell.
type Shell struct {
	nav       string
	err       string
	themeMode ThemeMode
	mark      *ui.Bitmap
}

// NewShell returns a shell showing Discover in the dark theme.
func NewShell() *Shell {
	return &Shell{
		nav:       destDiscover,
		themeMode: ThemeDark,
		mark:      brandMark(),
	}
}

// View draws the shell on the MyGo main thread.
func (s *Shell) View(c *ui.Context) {
	systemDark := c.Theme().Dark
	theme := themeFor(s.themeMode, systemDark)
	p := paletteFor(theme.Dark)

	ui.Themed(c, theme).Fill().Children(func() {
		width, _ := c.Size()
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			if width >= wideMinWidth {
				s.wideSidebar(c, p)
			} else {
				s.rail(c, p)
			}
			s.content(c, theme, p)
		})
	})
}

// wideSidebar draws the expanded navigation and the brand row.
func (s *Shell) wideSidebar(c *ui.Context, p palette) {
	page := s.nav

	ui.Column(c).Width(wideSidebarWidth).FillHeight().Background(p.sidebar).Children(func() {
		ui.Row(c).FillWidth().Padding(16, 16, 12, 16).Children(func() {
			brandRow(c, s.mark)
		})

		if ui.Sidebar(c, &page, func() {
			ui.SidebarSection(c, "Browse", nil, func() {
				ui.SidebarItem(c, destWatch, icons().watch, "Watch")
				ui.SidebarItem(c, destDiscover, icons().discover, "Discover")
				ui.SidebarItem(c, destLibrary, icons().library, "Library")
			})
			ui.SidebarSection(c, "App", nil, func() {
				ui.SidebarItem(c, destSettings, icons().settings, "Settings")
			})
		}).Width(wideSidebarWidth).Grow(1).Background(p.sidebar).Changed() {
			s.nav = page
		}
	})
}

// rail draws the narrow icon-only navigation.
func (s *Shell) rail(c *ui.Context, p palette) {
	dests := destinations()

	rail := ui.Column(c).Width(railWidth).FillHeight().Background(p.sidebar).
		AlignItems(ui.Center).Padding(16, 0, 12, 0).Gap(6)
	rail.Children(func() {
		drawBrandMark(c, s.mark)
		ui.Box(c).Height(8)

		for _, d := range dests[:3] {
			s.railItem(c, d, p)
		}

		// The rail keeps Tab order; arrow keys only navigate the wide sidebar.
		ui.Spacer(c)
		s.railItem(c, dests[3], p)
	})
}

// railItem draws one 48 pt icon button with an active indicator.
func (s *Shell) railItem(c *ui.Context, d destination, p palette) {
	selected := s.nav == d.id

	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		indicator := ui.Box(c).Width(3).Height(20).Radius(1.5)
		if selected {
			indicator.Background(p.tomato)
		}

		button := ui.Button(c, "").Size(railItemSize, railItemSize).Radius(8).Border(0, ui.Transparent)
		if selected {
			button = button.Background(p.pressed)
		}
		button.Children(func() {
			ui.Icon(c, iconFor(d.id)).FontSize(18)
		}).Label(d.label).Tooltip(d.label).AccessSelected(selected).OnClick(func() {
			s.nav = d.id
		})
	})
}

// iconFor returns the icon for a destination ID.
func iconFor(id string) *ui.SVG {
	switch id {
	case destWatch:
		return icons().watch
	case destLibrary:
		return icons().library
	case destSettings:
		return icons().settings
	default:
		return icons().discover
	}
}

// content draws the error banner and the current page.
func (s *Shell) content(c *ui.Context, theme *ui.Theme, p palette) {
	ui.Column(c).Grow(1).FillHeight().Background(p.canvas).Children(func() {
		if s.err != "" {
			s.errorBanner(c, theme, p)
		}

		ui.Column(c).Grow(1).FillWidth().Padding(24).Gap(16).Children(func() {
			s.drawPage(c, theme)
		})
	})
}

// errorBanner shows a dismissable error above the page content.
func (s *Shell) errorBanner(c *ui.Context, theme *ui.Theme, p palette) {
	banner := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(10, 16).
		Background(p.raised).Border(1, p.border)
	banner.Children(func() {
		ui.Text(c, s.err).TextColor(theme.Danger).Grow(1)
		ui.Button(c, "Dismiss").OnClick(s.ClearError)
	})
}

// ShowError puts message in the error banner; call on the main thread.
func (s *Shell) ShowError(message string) {
	s.err = message
}

// ClearError removes the error banner; call on the main thread.
func (s *Shell) ClearError() {
	s.err = ""
}

// ErrorMessage returns the banner's message, empty for none.
func (s *Shell) ErrorMessage() string {
	return s.err
}
