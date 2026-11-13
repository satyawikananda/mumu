package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// ShortcutContext scopes a command to the screen that owns it.
type ShortcutContext int

const (
	// ShortcutHome is the Home feed.
	ShortcutHome ShortcutContext = iota + 1
	// ShortcutPlayer is the active Watch screen.
	ShortcutPlayer
	// ShortcutBrowse is Discover, Library and Settings.
	ShortcutBrowse
	// ShortcutControlCenter is the Control Center overlay.
	ShortcutControlCenter
)

// ShortcutID names one command shared by handlers, hints and Settings.
type ShortcutID int

const (
	// ShortcutPlayPause is reserved for #37.
	ShortcutPlayPause ShortcutID = iota + 1
	// ShortcutMute toggles audio.
	ShortcutMute
	// ShortcutVolume changes the volume.
	ShortcutVolume
	// ShortcutFullscreen toggles native fullscreen.
	ShortcutFullscreen
	// ShortcutPiP enters or leaves Picture-in-Picture.
	ShortcutPiP
	// ShortcutDrawer toggles the channel drawer.
	ShortcutDrawer
	// ShortcutControlCenterCommand opens or closes Control Center.
	ShortcutControlCenterCommand
	// ShortcutSearch focuses the channel search.
	ShortcutSearch
	// ShortcutMove moves the focused card or row.
	ShortcutMove
	// ShortcutOpen opens the focused item.
	ShortcutOpen
	// ShortcutFavorite toggles a channel favorite.
	ShortcutFavorite
	// ShortcutBack goes to the previous destination.
	ShortcutBack
	// ShortcutClose closes the Control Center.
	ShortcutClose
	// ShortcutRetry repeats the active attempt while loading or failed.
	ShortcutRetry
	// ShortcutSettings opens Settings from any screen.
	ShortcutSettings
)

// ShortcutBinding is one key combination.
type ShortcutBinding struct {
	Mods ui.Modifiers
	Key  ui.Key
}

// Shortcut is one command definition. Hints and Settings read the same entry
// that the handler dispatches.
type Shortcut struct {
	ID       ShortcutID
	Context  ShortcutContext
	Bindings []ShortcutBinding
	Keys     []string
	Label    string
	// HintLabel overrides Label in the current screen's hint bar.
	HintLabel func(*Shell) string
	// Available is false for a command that is not implemented yet.
	Available bool
	Enabled   func(*Shell) bool
	Run       func(*Shell, ui.Key)
}

// ShortcutGroup is one Settings section.
type ShortcutGroup struct {
	Context   ShortcutContext
	Title     string
	Shortcuts []Shortcut
}

var shortcutRegistry = []Shortcut{
	{
		ID: ShortcutPlayPause, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeySpace}}, Keys: []string{"Space"},
		Label: "Play or pause", Available: true,
		HintLabel: func(s *Shell) string {
			if s.watch.Paused() {
				return "Resume"
			}

			return "Pause"
		},
		Enabled: func(s *Shell) bool { return s.watch.CanPause() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.TogglePause() },
	},
	{
		ID: ShortcutMute, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyM}}, Keys: []string{"M"},
		Label: "Mute", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.ToggleMute() },
	},
	{
		ID: ShortcutDrawer, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyC}}, Keys: []string{"C"},
		Label: "Channel drawer", Available: true,
		HintLabel: func(*Shell) string { return "Channels" },
		Enabled:   func(s *Shell) bool { return s.watch.HasChannels() },
		Run:       func(s *Shell, _ ui.Key) { s.watch.ToggleDrawer() },
	},
	{
		ID: ShortcutPiP, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyP}}, Keys: []string{"P"},
		Label: "Picture in picture", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.TogglePiP() },
	},
	{
		ID: ShortcutFullscreen, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyF}}, Keys: []string{"F"},
		Label: "Fullscreen", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.ToggleFullscreen() },
	},
	{
		ID: ShortcutVolume, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyUp}, {Key: ui.KeyDown}}, Keys: []string{"↑", "↓"},
		Label: "Volume", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run: func(s *Shell, key ui.Key) {
			if key == ui.KeyUp {
				s.watch.AdjustVolume(5)
				return
			}
			s.watch.AdjustVolume(-5)
		},
	},
	{
		ID: ShortcutRetry, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyR}}, Keys: []string{"R"},
		Label: "Try again", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanRetry() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.Retry() },
	},
	{
		ID: ShortcutControlCenterCommand, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyEscape}}, Keys: []string{"Esc"},
		Label: "Control Center", Available: true,
		HintLabel: func(s *Shell) string {
			if s.watch.IsInitialLoading() {
				return "Cancel"
			}

			return "Control Center"
		},
		Run: func(s *Shell, _ ui.Key) { s.handleEscape() },
	},
	{
		ID: ShortcutSearch, Context: ShortcutBrowse,
		Bindings: []ShortcutBinding{{Key: ui.KeySlash}}, Keys: []string{"/"},
		Label: "Search", Available: true,
		Run: func(s *Shell, _ ui.Key) { s.focusSearch() },
	},
	{
		ID: ShortcutMove, Context: ShortcutBrowse,
		Bindings: []ShortcutBinding{{Key: ui.KeyLeft}, {Key: ui.KeyRight}, {Key: ui.KeyUp}, {Key: ui.KeyDown}},
		Keys:     []string{"←", "→"},
		Label:    "Move between cards", Available: true,
		HintLabel: func(s *Shell) string {
			if s.IsDiscover() {
				return "Move"
			}

			return "Move between cards"
		},
		Run: func(s *Shell, key ui.Key) { s.moveBrowse(key) },
	},
	{
		ID: ShortcutOpen, Context: ShortcutBrowse,
		Bindings: []ShortcutBinding{{Key: ui.KeyEnter}}, Keys: []string{"⏎"},
		Label: "Open", Available: true,
		HintLabel: func(s *Shell) string {
			if s.IsDiscover() {
				return "Watch"
			}

			return "Open"
		},
	},
	{
		ID: ShortcutFavorite, Context: ShortcutBrowse,
		Bindings: []ShortcutBinding{{Key: ui.KeyF}}, Keys: []string{"F"},
		Label: "Favorite", Available: true,
		HintLabel: func(s *Shell) string {
			if s.IsDiscover() {
				return "Favorites"
			}

			return "Favorite"
		},
		Run: func(s *Shell, _ ui.Key) { s.toggleBrowseFavorite() },
	},
	{
		ID: ShortcutBack, Context: ShortcutBrowse,
		Bindings: []ShortcutBinding{{Key: ui.KeyEscape}}, Keys: []string{"Esc"},
		Label: "Back", Available: true,
		HintLabel: func(s *Shell) string {
			if s.IsDiscover() {
				return "Close"
			}

			return "Back"
		},
		Run: func(s *Shell, _ ui.Key) { s.handleEscape() },
	},
	{
		ID: ShortcutOpen, Context: ShortcutHome,
		Bindings: []ShortcutBinding{{Key: ui.KeyEnter}}, Keys: []string{"⏎"},
		Label: "Resume", Available: true,
		HintLabel: func(s *Shell) string {
			if s.home.OnCardFocus() {
				return "Open channel"
			}

			return "Resume"
		},
		Enabled: func(s *Shell) bool { return s.home.CanActivate() },
		Run:     func(s *Shell, _ ui.Key) { s.home.Activate() },
	},
	{
		ID: ShortcutMove, Context: ShortcutHome,
		Bindings: []ShortcutBinding{{Key: ui.KeyLeft}, {Key: ui.KeyRight}},
		Keys:     []string{"←", "→"},
		Label:    "Move between cards", Available: true,
		Enabled: func(s *Shell) bool { return s.home.FocusWithinRail() },
		Run: func(s *Shell, key ui.Key) {
			if key == ui.KeyLeft {
				s.home.MoveRail(-1)
			} else {
				s.home.MoveRail(1)
			}
		},
	},
	{
		ID: ShortcutControlCenterCommand, Context: ShortcutHome,
		Bindings: []ShortcutBinding{{Key: ui.KeyEscape}}, Keys: []string{"Esc"},
		Label: "Control Center", Available: true,
		Run: func(s *Shell, _ ui.Key) { s.handleEscape() },
	},
	{
		ID: ShortcutSearch, Context: ShortcutHome,
		Bindings: []ShortcutBinding{{Key: ui.KeySlash}}, Keys: []string{"/"},
		Label: "Search channels", Available: true,
		Run: func(s *Shell, _ ui.Key) { s.focusSearch() },
	},
	{
		ID: ShortcutMove, Context: ShortcutControlCenter,
		Bindings: []ShortcutBinding{{Key: ui.KeyLeft}, {Key: ui.KeyRight}},
		Keys:     []string{"←", "→"},
		Label:    "Move between cards", Available: true,
	},
	{
		ID: ShortcutOpen, Context: ShortcutControlCenter,
		Bindings: []ShortcutBinding{{Key: ui.KeyEnter}}, Keys: []string{"⏎"},
		Label: "Open", Available: true,
	},
	{
		ID: ShortcutClose, Context: ShortcutControlCenter,
		Bindings: []ShortcutBinding{{Key: ui.KeyEscape}}, Keys: []string{"Esc"},
		Label: "Close", Available: true,
	},
	{
		ID: ShortcutSettings, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Mods: ui.Cmd, Key: ui.KeyComma}}, Keys: []string{"⌘,"},
		Label: "Settings", Available: true,
		Run: func(s *Shell, _ ui.Key) { s.ShowSettings() },
	},
	{
		ID: ShortcutSettings, Context: ShortcutBrowse,
		Bindings: []ShortcutBinding{{Mods: ui.Cmd, Key: ui.KeyComma}}, Keys: []string{"⌘,"},
		Label: "Settings", Available: true,
		Run: func(s *Shell, _ ui.Key) { s.ShowSettings() },
	},
	{
		ID: ShortcutSettings, Context: ShortcutHome,
		Bindings: []ShortcutBinding{{Mods: ui.Cmd, Key: ui.KeyComma}}, Keys: []string{"⌘,"},
		Label: "Settings", Available: true,
		Run: func(s *Shell, _ ui.Key) { s.ShowSettings() },
	},
}

// shortcutContext returns the command scope for the current screen.
func (s *Shell) shortcutContext() ShortcutContext {
	switch s.nav {
	case destWatch:
		return ShortcutPlayer
	case destHome:
		return ShortcutHome
	default:
		return ShortcutBrowse
	}
}

// shortcutAvailable reports whether a definition may be shown and handled.
func shortcutAvailable(s *Shell, sc Shortcut) bool {
	if !sc.Available {
		return false
	}
	if sc.Enabled == nil {
		return true
	}

	return sc.Enabled(s)
}

// shortcutHints returns at most limit hints for one context.
func shortcutHints(s *Shell, context ShortcutContext, limit int) []Hint {
	hints := make([]Hint, 0, len(shortcutRegistry))
	for _, sc := range shortcutRegistry {
		if sc.Context != context || !shortcutAvailable(s, sc) {
			continue
		}
		label := sc.Label
		if sc.HintLabel != nil {
			label = sc.HintLabel(s)
		}
		hints = append(hints, Hint{Key: strings.Join(sc.Keys, " "), Action: label})
		if limit > 0 && len(hints) == limit {
			break
		}
	}

	return hints
}

// shortcutReferenceLayout is the visual order of the Settings reference. It
// names registry entries; labels and keys stay owned by shortcutRegistry.
var shortcutReferenceLayout = []struct {
	Context ShortcutContext
	Title   string
	IDs     []ShortcutID
}{
	{
		Context: ShortcutPlayer,
		Title:   "Player",
		IDs: []ShortcutID{
			ShortcutPlayPause, ShortcutMute, ShortcutVolume, ShortcutFullscreen,
			ShortcutPiP, ShortcutDrawer, ShortcutControlCenterCommand,
		},
	},
	{
		Context: ShortcutBrowse,
		Title:   "Browse",
		IDs: []ShortcutID{
			ShortcutSearch, ShortcutMove, ShortcutOpen, ShortcutFavorite, ShortcutBack,
		},
	},
	{
		Context: ShortcutControlCenter,
		Title:   "Control Center",
		IDs:     []ShortcutID{ShortcutMove, ShortcutOpen, ShortcutClose},
	},
}

// shortcutGroups returns the Settings reference in visual order. Home, retry
// and the Settings command stay implemented but are not part of this panel.
func shortcutGroups() []ShortcutGroup {
	groups := make([]ShortcutGroup, 0, len(shortcutReferenceLayout))
	for _, layout := range shortcutReferenceLayout {
		group := ShortcutGroup{Context: layout.Context, Title: layout.Title}
		for _, id := range layout.IDs {
			for _, sc := range shortcutRegistry {
				if sc.Context == layout.Context && sc.ID == id && sc.Available {
					group.Shortcuts = append(group.Shortcuts, sc)

					break
				}
			}
		}
		groups = append(groups, group)
	}

	return groups
}

// shortcutReference draws the read-only shortcut cards used by Settings.
func shortcutReference(c *ui.Context, theme *ui.Theme, width float32) ui.Element {
	groups := shortcutGroups()
	if width < 680 || len(groups) < 3 {
		column := ui.Column(c).FillWidth().Gap(16)
		column.Children(func() {
			for _, group := range groups {
				shortcutCard(c, theme, group)
			}
		})

		return column
	}

	grid := ui.Grid(c).FillWidth().Columns(2).Gap(16).AlignItems(ui.Start)
	grid.Children(func() {
		ui.Column(c).RowSpan(2).FillWidth().Children(func() {
			shortcutCard(c, theme, groups[0])
		})
		shortcutCard(c, theme, groups[1])
		shortcutCard(c, theme, groups[2])
	})

	return grid
}

// shortcutCard draws one group as the designed surface card.
func shortcutCard(c *ui.Context, theme *ui.Theme, group ShortcutGroup) {
	if len(group.Shortcuts) == 0 {
		return
	}

	card := ui.Column(c).FillWidth().Background(theme.Surface).
		Border(1, theme.Border.Alpha(0.55)).Radius(radiusTile).
		Padding(16, 18)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			if icon := shortcutGroupIcon(group.Context); icon != nil {
				ui.Icon(c, icon).Size(15, 15).TextColor(theme.TextMuted).
					AccessHidden(true)
			}
			ui.Text(c, strings.ToUpper(group.Title)).Font(bodyFont()).
				FontSize(textSize(theme, 10)).FontWeight(700).
				LetterSpacing(1.2).TextColor(theme.TextMuted).
				Role(ui.RoleHeading).Level(2)
		})

		rows := ui.Column(c).FillWidth().Padding(0, 0, 0, 4).
			Dividers(1, theme.Border.Alpha(0.45))
		rows.Children(func() {
			for _, sc := range group.Shortcuts {
				shortcutRow(c, theme, sc)
			}
		})
	})
}

// shortcutRow draws one label and its keys as plain right-aligned text.
func shortcutRow(c *ui.Context, theme *ui.Theme, sc Shortcut) {
	ui.Row(c).FillWidth().Height(40).AlignItems(ui.Center).Gap(16).Children(func() {
		ui.Text(c, sc.Label).Font(bodyFont()).FontSize(textSize(theme, 13)).
			TextColor(theme.TextMuted).Grow(1).SingleLine()
		ui.Text(c, strings.Join(sc.Keys, " ")).Font(bodyFont()).
			FontSize(textSize(theme, 12)).TextColor(theme.TextMuted).SingleLine()
	})
}

// shortcutGroupIcon selects the Lucide mark for one reference card.
func shortcutGroupIcon(context ShortcutContext) *ui.SVG {
	switch context {
	case ShortcutPlayer:
		return icons().play
	case ShortcutBrowse:
		return icons().compass
	case ShortcutControlCenter:
		return icons().layers
	default:
		return nil
	}
}
