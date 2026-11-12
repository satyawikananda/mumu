// Package ui holds mumu's native application shell.
package ui

import "github.com/egoist/mygo/ui"

// Shell is the native application shell. It owns the current destination,
// the Control Center overlay and the page objects the app publishes into.
type Shell struct {
	nav      string
	previous string
	err      string

	themeMode ThemeMode
	mark      *ui.Bitmap
	home      *Home
	discover  *Discover
	library   *Library
	settings  *Settings
	watch     *Watch
	pip       *PiP

	controlCenterOpen bool

	onDestinationChanged func(watch bool)
}

// NewShell returns a shell showing Discover in the dark theme.
func NewShell() *Shell {
	s := &Shell{
		nav:       destDiscover,
		themeMode: ThemeDark,
		mark:      brandMark(),
		home:      NewHome(),
		discover:  NewDiscover(),
		library:   NewLibrary(),
		settings:  NewSettings(),
		watch:     NewWatch(),
		pip:       NewPiP(),
	}
	// Add Playlist from Discover opens the Library and its import sheet.
	s.discover.OnAddPlaylist = func() {
		s.ShowLibrary()
		s.library.OpenAddPlaylist()
	}
	s.settings.OnManagePlaylists = func() {
		s.ShowLibrary()
		s.library.ShowPlaylists()
	}
	s.watch.OnBrowse = s.ShowDiscover
	s.home.OnBrowse = s.ShowDiscover
	s.home.OnWatch = s.ShowWatch

	return s
}

// Home returns the Home page for the app to publish state to.
func (s *Shell) Home() *Home { return s.home }

// Discover returns the Discover page for the app to publish state to.
func (s *Shell) Discover() *Discover { return s.discover }

// Library returns the Library page for the app to publish state to.
func (s *Shell) Library() *Library { return s.library }

// Settings returns the Settings page for the app to publish state to.
func (s *Shell) Settings() *Settings { return s.settings }

// Watch returns the Watch page for the app to publish state to.
func (s *Shell) Watch() *Watch { return s.watch }

// PiP returns the mini player for the app to publish state to.
func (s *Shell) PiP() *PiP { return s.pip }

// ShowHome makes Home the current destination.
func (s *Shell) ShowHome() { s.navigate(destHome) }

// ShowWatch makes Watch the current destination.
func (s *Shell) ShowWatch() { s.navigate(destWatch) }

// ShowDiscover makes Discover the current destination.
func (s *Shell) ShowDiscover() { s.navigate(destDiscover) }

// ShowLibrary makes Library the current destination.
func (s *Shell) ShowLibrary() { s.navigate(destLibrary) }

// ShowSettings makes Settings the current destination.
func (s *Shell) ShowSettings() { s.navigate(destSettings) }

// OnDestinationChanged reports every destination change. The callback runs on
// the UI thread and receives whether Watch is now visible.
func (s *Shell) OnDestinationChanged(fn func(watch bool)) {
	s.onDestinationChanged = fn
}

// IsWatch reports whether Watch is the current destination.
func (s *Shell) IsWatch() bool { return s.nav == destWatch }

// IsDiscover reports whether Discover is the current destination.
func (s *Shell) IsDiscover() bool { return s.nav == destDiscover }

// Destination returns the current destination ID, for the app and tests.
func (s *Shell) Destination() string { return s.nav }

// ControlCenterOpen reports whether the overlay is open.
func (s *Shell) ControlCenterOpen() bool { return s.controlCenterOpen }

// OpenControlCenter shows the overlay without changing playback or the page.
func (s *Shell) OpenControlCenter() {
	if s.controlCenterOpen {
		return
	}
	s.watch.CloseDrawer()
	s.controlCenterOpen = true
}

// CloseControlCenter hides the overlay and lets MyGo restore focus.
func (s *Shell) CloseControlCenter() { s.controlCenterOpen = false }

// navigate switches destinations and reports the change once.
func (s *Shell) navigate(id string) {
	if id == s.nav {
		s.CloseControlCenter()
		return
	}

	s.previous = s.nav
	s.nav = id
	s.CloseControlCenter()
	if id != destWatch {
		s.watch.CloseDrawer()
	}
	if s.onDestinationChanged != nil {
		s.onDestinationChanged(id == destWatch)
	}
}

// goBack returns to the previous destination once, then clears the link so a
// repeated Escape cannot toggle between two screens.
func (s *Shell) goBack() {
	id := s.previous
	if id == "" || id == s.nav {
		return
	}

	s.previous = ""
	s.nav = id
	s.CloseControlCenter()
	if id != destWatch {
		s.watch.CloseDrawer()
	}
	if s.onDestinationChanged != nil {
		s.onDestinationChanged(id == destWatch)
	}
}

// PiPView draws the floating player with the app's theme.
func (s *Shell) PiPView(c *ui.Context) {
	theme := themeForContext(s.themeMode, c)
	ui.Themed(c, theme).Fill().Background(watchVideoColor).Children(func() {
		s.pip.View(c, theme)
	})
}

// View draws the shell on the MyGo main thread.
func (s *Shell) View(c *ui.Context) {
	theme := themeForContext(s.themeMode, c)

	ui.Themed(c, theme).Fill().Children(func() {
		s.chrome(c, theme)
		s.content(c, theme)
		if !s.controlCenterOpen {
			s.handleShortcuts(c)
		}
		if s.controlCenterOpen {
			s.controlCenter(c).View(c)
		}
	})
}

// chrome draws the persistent top bar and the visible Control Center entry.
func (s *Shell) chrome(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())

	ui.Row(c).FillWidth().Height(48).Padding(8, layout.Padding).
		AlignItems(ui.Center).Gap(12).Children(func() {
		brand := ui.ButtonBase(c.Key("open-control-center")).Padding(4, 6).Radius(8).
			FocusRingOffset(-2)
		brand = brand.Label("Open Control Center").Tooltip("Control Center")
		brand.Children(func() { BrandLockup(c, s.mark) })
		if brand.Clicked() {
			s.OpenControlCenter()
		}

		ui.Text(c, destinationLabel(s.nav)).Font(bodyFont()).FontSize(textSize(theme, 11)).
			FontWeight(700).LetterSpacing(1).TextColor(theme.TextMuted).SingleLine()

		ui.Spacer(c)

		s.topAction(c, icons().panelLeft, "Control Center", s.OpenControlCenter)
		s.topIcon(c, icons().settings, "Settings", s.ShowSettings)
	})
}

// topAction draws a labelled top-bar action. The label stays visible so
// Control Center is never keyboard-only.
func (s *Shell) topAction(c *ui.Context, icon *ui.SVG, label string, action func()) {
	t := c.Theme()
	button := ui.ButtonBase(c).Height(30).Padding(6, 10).Radius(8).
		Border(1, t.Border).FocusRingOffset(-2)
	if button.Hovered() {
		button = button.Background(t.SurfaceHover)
	}
	button = button.Label(label).Tooltip(label)
	button.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(t.Text)
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(t, 12)).
			FontWeight(600).TextColor(t.Text).SingleLine()
	})
	if button.Clicked() && action != nil {
		action()
	}
}

// topIcon draws a labelled icon action in the top bar.
func (s *Shell) topIcon(c *ui.Context, icon *ui.SVG, label string, action func()) {
	t := c.Theme()
	button := ui.ButtonBase(c).Size(32, 32).Radius(8).FocusRingOffset(-2)
	if button.Hovered() {
		button = button.Background(t.SurfaceHover)
	}
	button = button.Label(label).Tooltip(label)
	button.Children(func() {
		ui.Icon(c, icon).Size(18, 18).TextColor(t.Text)
	})
	if button.Clicked() && action != nil {
		action()
	}
}

// content draws the error banner, the current page and the shortcut strip.
func (s *Shell) content(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())
	p := paletteFor(theme.Dark)

	ui.Column(c).Grow(1).FillWidth().Background(p.canvas).
		Padding(layout.Padding/2, layout.Padding).Children(func() {
		if s.err != "" {
			s.errorBanner(c, theme, p)
		}

		ui.Column(c).Grow(1).FillWidth().Gap(16).Children(func() {
			s.drawPage(c, theme)
		})

		if !s.controlCenterOpen {
			hints := shortcutHints(s, s.shortcutContext(), layout.HintCount)
			if len(hints) > 0 {
				HintBar{Hints: hints}.View(c)
			}
		}
	})
}

// errorBanner shows a dismissable error above the page content.
func (s *Shell) errorBanner(c *ui.Context, theme *ui.Theme, p palette) {
	banner := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(10, 16).
		Background(p.surface2).Border(1, p.border)
	banner.Children(func() {
		ui.Text(c, s.err).TextColor(theme.Danger).Grow(1)
		ui.Button(c, "Dismiss").OnClick(s.ClearError)
	})
}

// handleShortcuts dispatches the one registry for the current screen.
func (s *Shell) handleShortcuts(c *ui.Context) {
	if c.FocusTakesText() {
		return
	}

	context := s.shortcutContext()
	for _, sc := range shortcutRegistry {
		if sc.Context != context || sc.Run == nil || !shortcutAvailable(s, sc) {
			continue
		}
		for _, binding := range sc.Bindings {
			if c.Shortcut(binding.Mods, binding.Key) {
				sc.Run(s, binding.Key)
				return
			}
		}
	}
}

// handleEscape closes the topmost local layer or navigates back.
func (s *Shell) handleEscape() {
	if s.watch.DrawerOpen() {
		s.watch.CloseDrawer()
		return
	}

	switch s.nav {
	case destHome, destWatch:
		s.OpenControlCenter()
	default:
		s.goBack()
	}
}

// focusSearch sends the viewer to Discover and focuses its search field.
func (s *Shell) focusSearch() {
	s.ShowDiscover()
	s.discover.FocusSearch()
}

// toggleBrowseFavorite toggles the selected Discover favorite.
func (s *Shell) toggleBrowseFavorite() {
	if s.nav == destDiscover {
		s.discover.ToggleSelectedFavorite()
	}
}

// controlCenter builds the overlay from published page state.
func (s *Shell) controlCenter(c *ui.Context) ControlCenter {
	watch := s.watch.State()
	library := s.library.State()
	state := ControlCenterState{
		Channel:        watch.Channel,
		Source:         watch.Source,
		FavoriteCount:  len(library.Favorites),
		PlaylistCount:  len(library.Sources),
		LibraryLoading: library.Kind == LibraryLoading,
		LibraryFailed:  library.Kind == LibraryFailed,
		PiPActive:      watch.PiPActive,
		Current:        s.nav,
		Hints:          shortcutHints(s, ShortcutControlCenter, 4),
	}
	switch watch.Kind {
	case WatchResume:
		state.Resume = true
	case WatchLoading, WatchBuffering, WatchPlaying:
		state.Active = true
	}
	state.FavoriteNames = firstNames(library.Favorites)
	state.PlaylistNames = firstSourceNames(library.Sources)

	return ControlCenter{
		State: state,
		Actions: ControlCenterActions{
			Close:     s.CloseControlCenter,
			Home:      s.ShowHome,
			Discover:  s.ShowDiscover,
			Watch:     s.ShowWatch,
			PiP:       func() { s.CloseControlCenter(); s.watch.TogglePiP() },
			Favorites: func() { s.ShowLibrary(); s.library.ShowFavorites() },
			Playlists: func() { s.ShowLibrary(); s.library.ShowPlaylists() },
			Resume:    func() { s.CloseControlCenter(); s.home.Resume() },
		},
	}
}

// firstNames returns up to three real labels for a Control Center meta line.
func firstNames(entries []LibraryChannel) []string {
	names := make([]string, 0, 3)
	for i, entry := range entries {
		if i == 3 {
			break
		}
		if entry.Name != "" {
			names = append(names, entry.Name)
		}
	}

	return names
}

// firstSourceNames returns up to three real playlist labels.
func firstSourceNames(entries []LibrarySource) []string {
	names := make([]string, 0, 3)
	for i, entry := range entries {
		if i == 3 {
			break
		}
		if entry.Name != "" {
			names = append(names, entry.Name)
		}
	}

	return names
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
