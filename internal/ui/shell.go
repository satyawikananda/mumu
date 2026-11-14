// Package ui holds mumu's native application shell.
package ui

import "github.com/egoist/mygo/ui"

// chromeHeight is the top bar's height. The bar overlays the content so a
// page's artwork can pass behind it; every page reserves the same inset.
const chromeHeight float32 = 56

// navigationHistoryLimit bounds the Back stack. It is a convenience, not a
// durable navigation log.
const navigationHistoryLimit = 32

// Shell is the native application shell. It owns the current destination,
// the Control Center overlay and the page objects the app publishes into.
type Shell struct {
	nav     string
	history []string
	err     string

	themeMode ThemeMode
	mark      *ui.Bitmap
	home      *Home
	discover  *Discover
	library   *Library
	settings  *Settings
	watch     *Watch
	pip       *PiP

	controlCenterOpen bool
	// ccButton keeps the Control Center opener's identity so closing the
	// overlay can hand the keyboard focus back to it.
	ccButton          ui.Handle
	focusAfterOverlay bool

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
	s.discover.OnOpenSettings = s.ShowSettings
	s.settings.OnTheme = s.SetThemeMode
	s.settings.SetMark(s.mark)
	s.watch.OnBack = s.leaveWatch
	s.watch.OnOpenControlCenter = s.OpenControlCenter
	s.watch.OnSettings = s.ShowSettings
	s.watch.SetMark(s.mark)
	s.watch.SetHints(func(limit int) []Hint {
		return shortcutHints(s, ShortcutPlayer, limit)
	})
	s.watch.OnBrowse = s.ShowDiscover
	s.home.OnBrowse = s.ShowDiscover
	s.home.OnWatch = s.ShowWatch
	s.home.OnAddPlaylist = func() {
		s.ShowLibrary()
		s.library.OpenAddPlaylist()
	}
	s.home.OnSeeAll = func(rail HomeRail) {
		s.ShowLibrary()
		if rail == HomeRailRecent {
			s.library.ShowRecent()

			return
		}
		s.library.ShowFavorites()
	}

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

// SetThemeMode changes the shell appearance and the Settings page's selected
// card. The app calls it after loading or confirming a stored preference.
func (s *Shell) SetThemeMode(mode ThemeMode) {
	if mode < ThemeDark || mode > ThemeSystem {
		mode = ThemeDark
	}
	s.themeMode = mode
	s.settings.SetTheme(mode)
}

// ThemeMode returns the current appearance.
func (s *Shell) ThemeMode() ThemeMode { return s.themeMode }

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

// CloseControlCenter hides the overlay and lets MyGo restore focus. The
// shell also returns the keyboard focus to the Control Center opener when the
// overlay was dismissed rather than used to navigate.
func (s *Shell) CloseControlCenter() {
	if !s.controlCenterOpen {
		return
	}
	s.controlCenterOpen = false
	s.focusAfterOverlay = !s.IsWatch()
}

// navigate switches destinations and reports the change once. The page left
// behind joins the bounded Back history.
func (s *Shell) navigate(id string) {
	if id == s.nav {
		s.CloseControlCenter()
		return
	}

	s.history = append(s.history, s.nav)
	if len(s.history) > navigationHistoryLimit {
		s.history = s.history[len(s.history)-navigationHistoryLimit:]
	}
	s.setDestination(id)
}

// setDestination switches the visible page without touching history.
func (s *Shell) setDestination(id string) {
	s.nav = id
	s.CloseControlCenter()
	s.focusAfterOverlay = false
	if id != destWatch {
		s.watch.CloseDrawer()
	}
	if s.onDestinationChanged != nil {
		s.onDestinationChanged(id == destWatch)
	}
}

// goBack returns to the most recent different destination in history. It
// reports whether it moved.
func (s *Shell) goBack() bool {
	for len(s.history) > 0 {
		id := s.history[len(s.history)-1]
		s.history = s.history[:len(s.history)-1]
		if id == "" || id == s.nav {
			continue
		}
		s.setDestination(id)

		return true
	}

	return false
}

// CanGoBack reports whether Back has a destination in history.
func (s *Shell) CanGoBack() bool {
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i] != "" && s.history[i] != s.nav {
			return true
		}
	}

	return false
}

// PiPView draws the floating player with the app's theme.
func (s *Shell) PiPView(c *ui.Context) {
	theme := themeForContext(s.themeMode, c)
	ui.Themed(c, theme).Fill().Background(ui.Transparent).Children(func() {
		s.pip.View(c, theme)
	})
}

// View draws the shell on the MyGo main thread.
func (s *Shell) View(c *ui.Context) {
	theme := themeForContext(s.themeMode, c)

	themed := ui.Themed(c, theme).Fill()
	if s.IsWatch() {
		// Watch is a hole in MyGo's surface: the retained video sits under it.
		// The Themed column would otherwise paint the opaque canvas over it.
		themed = themed.Background(ui.Transparent)
	}
	themed.Children(func() {
		// Watch is full-bleed: the picture owns the window and the page draws
		// its own chrome, so the shell bar and its inset stay out of the way.
		if !s.IsWatch() {
			s.chrome(c, theme)
		}
		s.content(c, theme)
		if !s.controlCenterOpen {
			s.handleShortcuts(c)
		} else if s.watch.NoteActivity(c.Now()) {
			// The overlay is not the picture's chrome; keep the page's own
			// controls from idling away underneath it.
			c.Invalidate()
		}
		if s.controlCenterOpen {
			s.controlCenter().View(c)
		}
	})

	if !s.IsWatch() && !s.controlCenterOpen && s.focusAfterOverlay {
		s.focusAfterOverlay = false
		s.ccButton.Focus(c)
	}
}

// chrome draws the top bar above the page. It stays transparent while the page
// is at its start, so page artwork can pass behind it, and takes a translucent
// canvas scrim once the page scrolls under it.
func (s *Shell) chrome(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())
	p := paletteFor(theme.Dark)
	scrolled := s.pageScrolled()

	bar := ui.Row(c).FillWidth().Height(chromeHeight).ZIndex(1).
		Padding(4, layout.Padding).AlignItems(ui.Center).Gap(12)
	if scrolled {
		bar = bar.Background(p.canvas.Alpha(0.86)).
			Border(1, p.border).BorderWidth(0, 0, 1, 0)
	}
	bar.Transition(ui.ElementTransition{Colors: true, Duration: hoverMotion.Duration})
	bar.Children(func() {
		// The slot stays reserved so the brand never shifts when Back appears.
		if s.CanGoBack() {
			s.topIcon(c, nil, icons().arrowLeft, "Back", func() { s.goBack() })
		} else {
			ui.Box(c).Size(32, 32).AccessHidden(true)
		}

		brand := ui.ButtonBase(c.Key("brand-home")).Padding(2, 6).Radius(8).
			FocusRingOffset(-2)
		brand = brand.Label("Go to Home").Tooltip("Home")
		brand.Children(func() { BrandLockup(c, s.mark) })
		if brand.Clicked() {
			s.ShowHome()
		}

		ui.Text(c, destinationLabel(s.nav)).Font(bodyFont()).FontSize(textSize(theme, 11)).
			FontWeight(700).LetterSpacing(1).TextColor(theme.TextMuted).SingleLine()

		ui.Spacer(c)

		s.topIcon(c, &s.ccButton, icons().panelLeft, "Control Center", s.OpenControlCenter)
		s.topIcon(c, nil, icons().settings, "Settings", s.ShowSettings)
	})
}

// topIcon draws a labelled icon action in the top bar. A non-nil handle keeps
// the control's identity for focus restoration.
func (s *Shell) topIcon(c *ui.Context, handle *ui.Handle, icon *ui.SVG, label string, action func()) {
	t := c.Theme()
	button := ui.ButtonBase(c)
	if handle != nil {
		button = button.Bind(handle)
	}
	button = button.Size(32, 32).Radius(8).FocusRingOffset(-2)
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

// pageScrolled reports whether the current page is scrolled away from its
// start, which turns the top bar into a scrim.
func (s *Shell) pageScrolled() bool {
	switch s.nav {
	case destHome:
		return s.home.Scrolled()
	case destDiscover:
		return s.discover.Scrolled()
	case destLibrary:
		return s.library.Scrolled()
	case destSettings:
		return s.settings.Scrolled()
	default:
		return false
	}
}

// ChromeScrolled reports whether the top bar shows its scrim, for tests.
func (s *Shell) ChromeScrolled() bool { return s.pageScrolled() }

// content draws the error banner, the current page and the shortcut strip.
func (s *Shell) content(c *ui.Context, theme *ui.Theme) {
	if s.IsWatch() {
		s.watchContent(c, theme)

		return
	}

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

// watchContent draws Watch edge to edge. The page owns its hint bar, so the
// shell draws neither padding, canvas nor strip on this route.
func (s *Shell) watchContent(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		s.drawPage(c, theme)
	})

	if s.err == "" || s.controlCenterOpen {
		return
	}
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Padding(12, 20).ZIndex(2).
		Children(func() {
			s.errorBanner(c, theme, paletteFor(theme.Dark))
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
				if context == ShortcutPlayer && s.watch.NoteActivity(c.Now()) {
					// A key is activity: the picture's chrome comes back for a
					// viewer driving the player from the keyboard.
					c.Invalidate()
				}

				return
			}
		}
	}
}

// handleEscape closes the topmost local layer, cancels a first connect, or
// navigates back.
func (s *Shell) handleEscape() {
	if s.watch.DrawerOpen() {
		s.watch.CloseDrawer()
		return
	}
	if s.watch.QualityOpen() {
		s.watch.CloseQuality()
		return
	}
	if s.nav == destWatch && s.watch.IsInitialLoading() {
		s.watch.Cancel()
		return
	}

	switch s.nav {
	case destHome, destWatch:
		s.OpenControlCenter()
	default:
		s.goBack()
	}
}

// leaveWatch returns to the previous destination, or Home when Watch was
// opened directly.
func (s *Shell) leaveWatch() {
	if s.goBack() {
		return
	}
	s.setDestination(destHome)
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

// moveBrowse moves the Discover grid cursor with the arrow keys. Other Browse
// screens keep their own focus handling.
func (s *Shell) moveBrowse(key ui.Key) {
	if s.nav != destDiscover {
		return
	}

	switch key {
	case ui.KeyLeft:
		s.discover.MoveTile(-1, 0)
	case ui.KeyRight:
		s.discover.MoveTile(1, 0)
	case ui.KeyUp:
		s.discover.MoveTile(0, -1)
	case ui.KeyDown:
		s.discover.MoveTile(0, 1)
	}
}

// controlCenter builds the overlay from published page state.
func (s *Shell) controlCenter() ControlCenter {
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
	case WatchLoading, WatchBuffering, WatchPlaying, WatchPaused:
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
