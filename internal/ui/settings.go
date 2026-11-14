package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// SettingsSection is one panel in the screen-local Settings rail. The rail is
// not a permanent shell destination.
type SettingsSection int

const (
	// SettingsAppearance holds the theme and playback preferences.
	SettingsAppearance SettingsSection = iota + 1
	// SettingsShortcuts lists the scoped commands from the one registry.
	SettingsShortcuts
	// SettingsAbout holds build, license and update information.
	SettingsAbout
)

// AboutInfo is the actual build and local catalog information Settings shows.
// The app owns every value; the page only formats it. No field holds a stream
// URL, credential, favorites entry or local path.
type AboutInfo struct {
	Version       string
	Commit        string
	GoVersion     string
	MyGoVersion   string
	OS            string
	Arch          string
	License       string
	ChannelCount  int
	SourceCount   int
	CountsKnown   bool
	CountsLoading bool
	RepositoryURL string
}

// UpdateKind is the state of the manual release check.
type UpdateKind int

const (
	// UpdateIdle means no check ran in this session.
	UpdateIdle UpdateKind = iota + 1
	// UpdateChecking means one check is in flight.
	UpdateChecking
	// UpdateCurrent means the installed build is at least as new as the
	// newest release in its channel.
	UpdateCurrent
	// UpdateAvailable means a newer release exists.
	UpdateAvailable
	// UpdateNoRelease means the channel has no published release.
	UpdateNoRelease
	// UpdateError means the check could not complete.
	UpdateError
)

// UpdateState is what the app publishes after a manual release check.
type UpdateState struct {
	Kind       UpdateKind
	Current    string
	Latest     string
	ReleaseURL string
	Message    string
}

// Settings is the Settings page. The app publishes stored preferences and
// real build data; the page dispatches intents and owns no I/O.
type Settings struct {
	section SettingsSection
	theme   ThemeMode
	mark    *ui.Bitmap

	startMuted bool

	themeErr    string
	playbackErr string
	aboutErr    string

	about  AboutInfo
	update UpdateState
	copied bool

	scroll ui.ScrollState

	// OnTheme applies and persists the chosen appearance.
	OnTheme func(ThemeMode)
	// OnStartMuted persists the start-muted preference.
	OnStartMuted func(bool)
	// OnCheckUpdates starts one manual release check.
	OnCheckUpdates func()
}

// NewSettings returns the default settings: dark appearance and start muted
// off.
func NewSettings() *Settings {
	return &Settings{
		section: SettingsAppearance,
		theme:   ThemeDark,
		update:  UpdateState{Kind: UpdateIdle},
	}
}

// SetMark gives the About hero the app's brand bitmap.
func (s *Settings) SetMark(mark *ui.Bitmap) { s.mark = mark }

// SetTheme shows the stored appearance.
func (s *Settings) SetTheme(mode ThemeMode) {
	if mode < ThemeDark || mode > ThemeSystem {
		mode = ThemeDark
	}
	s.theme = mode
	s.themeErr = ""
}

// Theme returns the appearance the page shows.
func (s *Settings) Theme() ThemeMode { return s.theme }

// SetThemeError shows an inline appearance failure.
func (s *Settings) SetThemeError(message string) { s.themeErr = message }

// ThemeError returns the inline appearance failure, empty for none.
func (s *Settings) ThemeError() string { return s.themeErr }

// SetStartMuted shows the stored start-muted preference.
func (s *Settings) SetStartMuted(muted bool) {
	s.startMuted = muted
	s.playbackErr = ""
}

// StartMuted returns the preference the page shows.
func (s *Settings) StartMuted() bool { return s.startMuted }

// SetPlaybackError shows an inline playback preference failure.
func (s *Settings) SetPlaybackError(message string) { s.playbackErr = message }

// PlaybackError returns the inline playback failure, empty for none.
func (s *Settings) PlaybackError() string { return s.playbackErr }

// SetAbout publishes actual build and catalog data.
func (s *Settings) SetAbout(info AboutInfo) {
	s.about = info
	s.aboutErr = ""
}

// SetAboutError shows an inline About failure.
func (s *Settings) SetAboutError(message string) { s.aboutErr = message }

// AboutError returns the inline About failure, empty for none.
func (s *Settings) AboutError() string { return s.aboutErr }

// About returns the build data the page shows.
func (s *Settings) About() AboutInfo { return s.about }

// SetUpdate publishes the manual release-check state.
func (s *Settings) SetUpdate(state UpdateState) { s.update = state }

// Update returns the release-check state.
func (s *Settings) Update() UpdateState { return s.update }

// ShowSection selects one Settings panel.
func (s *Settings) ShowSection(section SettingsSection) {
	s.section = section
	s.scroll.Y = 0
	s.copied = false
}

// Section returns the visible panel.
func (s *Settings) Section() SettingsSection { return s.section }

// Scrolled reports whether the panel is scrolled from its start.
func (s *Settings) Scrolled() bool { return s.scroll.Y > 1 }

// View draws Settings on the MyGo main thread.
func (s *Settings) View(c *ui.Context, theme *ui.Theme) {
	layout := layoutFor(c.Size())
	width, _ := c.Size()
	railWidth := settingsRailWidth(width, layout)
	gap := float32(28)
	if layout.Breakpoint == BreakpointCompact {
		gap = 16
	}

	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Start).Gap(gap).Children(func() {
		s.rail(c, theme, railWidth)
		ui.Scroll(c).Grow(1).FillHeight().TrackScroll(&s.scroll).Children(func() {
			ui.Column(c).FillWidth().Gap(28).Children(func() {
				switch s.section {
				case SettingsShortcuts:
					s.shortcuts(c, theme, settingsPanelWidth(width, layout, railWidth, gap))
				case SettingsAbout:
					s.aboutPanel(c, theme)
				default:
					s.appearance(c, theme, settingsPanelWidth(width, layout, railWidth, gap))
				}
			})
		})
	})
}

// settingsRailWidth keeps the designed rail while leaving a usable panel at
// the minimum window size.
func settingsRailWidth(width float32, layout Layout) float32 {
	return min(layout.SettingsRail, max(180, width*0.28))
}

// settingsPanelWidth estimates the panel width inside the shell padding.
func settingsPanelWidth(width float32, layout Layout, rail, gap float32) float32 {
	panel := width - layout.Padding*2 - rail - gap
	if panel < 240 {
		return 240
	}

	return panel
}

// rail draws the screen-local Settings navigation.
func (s *Settings) rail(c *ui.Context, theme *ui.Theme, width float32) {
	ui.Column(c).Width(width).FillHeight().Shrink(0).Gap(4).Children(func() {
		ui.Text(c, "SETTINGS").Font(bodyFont()).FontSize(textSize(theme, 10)).
			FontWeight(700).LetterSpacing(1).TextColor(theme.TextMuted).
			Padding(0, 10).Margin(0, 0, 6, 0)

		s.railItem(c, theme, "settings-rail-appearance", SettingsAppearance,
			"Appearance", icons().sliders)
		s.railItem(c, theme, "settings-rail-shortcuts", SettingsShortcuts,
			"Shortcuts", icons().panelLeft)
		s.railItem(c, theme, "settings-rail-about", SettingsAbout,
			"About", icons().compass)

		ui.Spacer(c)

		if version := s.versionLabel(false); version != "" {
			ui.Text(c, ProductName+" "+version).Font(bodyFont()).FontSize(textSize(theme, 12)).
				TextColor(theme.TextMuted).Padding(0, 10).SingleLine()
		}
	})
}

// railItem draws one selectable rail row.
func (s *Settings) railItem(
	c *ui.Context,
	theme *ui.Theme,
	key string,
	section SettingsSection,
	label string,
	icon *ui.SVG,
) {
	active := s.section == section
	item := ui.ButtonBase(c.Key(key)).FillWidth().Height(39).Padding(8, 10).
		Radius(radiusRow).Gap(10).FocusRingOffset(-2).Label(label)
	if active {
		item = item.Background(theme.Surface).Border(1, theme.Border.Alpha(0.7))
	} else if item.Hovered() {
		item = item.Background(theme.SurfaceHover)
	}
	item.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(theme.Text)
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 13)).
			FontWeight(600).TextColor(theme.Text).SingleLine()
	})
	if item.Clicked() {
		s.ShowSection(section)
	}
}

// appearance draws the theme and playback preference groups.
func (s *Settings) appearance(c *ui.Context, theme *ui.Theme, panelWidth float32) {
	s.panelHead(c, theme, "Appearance",
		"How "+ProductName+" looks on this display. Changes apply immediately, no restart.")

	s.themeGroup(c, theme, panelWidth)
	s.playbackGroup(c, theme)
}

// panelHead draws the panel title and its explanation.
func (s *Settings) panelHead(c *ui.Context, theme *ui.Theme, title, description string) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		pageHeading(c, title)
		ui.Text(c, description).Font(bodyFont()).FontSize(textSize(theme, 13)).
			TextColor(theme.TextMuted).MaxWidth(720)
	})
}

// settingsGroup draws one labelled group with rows.
func settingsGroup(c *ui.Context, theme *ui.Theme, label string, body func()) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 10)).
			FontWeight(700).LetterSpacing(1).TextColor(theme.TextMuted)

		ui.Column(c).FillWidth().Clip().Radius(radiusTile).
			Background(theme.Surface).Border(1, theme.Border.Alpha(0.7)).
			Gap(1).Dividers(1, theme.Border.Alpha(0.55)).Children(body)
	})
}

// settingsRow draws a title and description with a control at the end.
func settingsRow(
	c *ui.Context,
	theme *ui.Theme,
	title, description string,
	control func(title, description ui.Element),
) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(20).Padding(12, 14).Children(func() {
		text := ui.Column(c).Grow(1).Gap(3)
		var titleElement, descriptionElement ui.Element
		text.Children(func() {
			titleElement = ui.Text(c, title).Font(bodyFont()).FontSize(textSize(theme, 13)).
				FontWeight(600).TextColor(theme.Text)
			descriptionElement = ui.Text(c, description).Font(bodyFont()).
				FontSize(textSize(theme, 12)).TextColor(theme.TextMuted)
		})
		if control != nil {
			control(titleElement, descriptionElement)
		}
	})
}

// settingsError draws an inline failure under a control.
func settingsError(c *ui.Context, theme *ui.Theme, message string) {
	if message == "" {
		return
	}
	ui.Text(c, message).Font(bodyFont()).FontSize(textSize(theme, 12)).
		TextColor(theme.Danger).Padding(0, 14).MaxWidth(620)
}

// themeGroup draws the three appearance cards.
func (s *Settings) themeGroup(c *ui.Context, theme *ui.Theme, panelWidth float32) {
	columns := 3
	if panelWidth < 700 {
		columns = 1
	}

	settingsGroup(c, theme, "THEME", func() {
		ui.Column(c).FillWidth().Gap(10).Padding(12, 14).Children(func() {
			ui.Grid(c).FillWidth().Columns(columns).Gap(10).Children(func() {
				s.themeCard(c, theme, ThemeDark, "Dark")
				s.themeCard(c, theme, ThemeLight, "Light")
				s.themeCard(c, theme, ThemeSystem, "System")
			})
			settingsError(c, theme, s.themeErr)
		})
	})
}

// themeCard draws one appearance choice with a small palette preview.
func (s *Settings) themeCard(
	c *ui.Context,
	theme *ui.Theme,
	mode ThemeMode,
	label string,
) {
	selected := s.theme == mode
	border := theme.Border.Alpha(0.8)
	if selected {
		border = theme.Text
	}

	card := ui.ButtonBase(c.Key("settings-theme-"+strings.ToLower(label))).
		FillWidth().Column().AlignItems(ui.Start).Padding(8).Gap(8).
		Radius(radiusRow).Background(theme.Background).Border(1, border).
		FocusRingOffset(-2).Label(label + " appearance")
	if card.Hovered() {
		card = card.Background(theme.SurfaceHover)
	}
	card.Children(func() {
		s.themePreview(c, mode)

		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).Font(bodyFont()).FontSize(textSize(theme, 13)).
				FontWeight(600).TextColor(theme.Text).Grow(1).SingleLine()
			if selected {
				ui.Icon(c, icons().check).Size(14, 14).TextColor(theme.Text).
					AccessHidden(true)
			}
		})
	})
	if card.Clicked() {
		s.theme = mode
		s.themeErr = ""
		if s.OnTheme != nil {
			s.OnTheme(mode)
		}
	}
}

// themePreview draws a miniature interface in the mode's palette.
func (s *Settings) themePreview(c *ui.Context, mode ThemeMode) {
	if mode == ThemeSystem {
		ui.Row(c).FillWidth().Height(72).Clip().Radius(8).
			Border(1, c.Theme().Border.Alpha(0.5)).Children(func() {
			themePreviewHalf(c, paletteFor(true))
			themePreviewHalf(c, paletteFor(false))
		})

		return
	}

	p := paletteFor(mode == ThemeDark)

	ui.Column(c).FillWidth().Height(72).Clip().Radius(8).
		Background(p.canvas).Children(func() {
		ui.Box(c).FillWidth().Height(18).Background(p.surface2)
		ui.Row(c).Padding(8).Gap(6).Children(func() {
			for range 3 {
				ui.Box(c).Grow(1).Height(38).Radius(4).Background(p.surface)
			}
		})
	})
}

// themePreviewHalf draws one side of the System preview.
func themePreviewHalf(c *ui.Context, p palette) {
	ui.Column(c).Grow(1).FillHeight().Background(p.canvas).Padding(6).Gap(5).Children(func() {
		ui.Box(c).FillWidth().Height(7).Radius(2).Background(p.surface2)
		ui.Box(c).FillWidth().Height(30).Radius(4).Background(p.surface)
	})
}

// playbackGroup draws the start-muted preference.
func (s *Settings) playbackGroup(c *ui.Context, theme *ui.Theme) {
	settingsGroup(c, theme, "PLAYBACK", func() {
		settingsRow(c, theme, "Start muted",
			"New channels open with audio off until you press M. The current channel is not changed.",
			func(title, description ui.Element) {
				toggle := ui.Switch(c.Key("settings-start-muted"), &s.startMuted).
					Label("Start muted")
				toggle.LabelledBy(title).DescribedBy(description)
				if toggle.Changed() {
					s.playbackErr = ""
					if s.OnStartMuted != nil {
						s.OnStartMuted(s.startMuted)
					}
				}
			})

		settingsError(c, theme, s.playbackErr)
	})
}

// versionLabel formats the actual build version, optionally with its commit.
func (s *Settings) versionLabel(withCommit bool) string {
	version := strings.TrimSpace(s.about.Version)
	if version == "" {
		version = "Development build"
	}
	if !withCommit {
		return version
	}
	commit := strings.TrimSpace(s.about.Commit)
	if commit == "" || strings.EqualFold(commit, "unknown") {
		return version
	}

	return version + " · " + commit
}
