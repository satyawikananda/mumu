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
	Title     string
	Shortcuts []Shortcut
}

var shortcutRegistry = []Shortcut{
	{
		ID: ShortcutPlayPause, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeySpace}}, Keys: []string{"Space"},
		Label: "Play or pause", Available: false,
	},
	{
		ID: ShortcutMute, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyM}}, Keys: []string{"M"},
		Label: "Mute", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.ToggleMute() },
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
		ID: ShortcutFullscreen, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyF}}, Keys: []string{"F"},
		Label: "Fullscreen", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.ToggleFullscreen() },
	},
	{
		ID: ShortcutPiP, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyP}}, Keys: []string{"P"},
		Label: "Picture in picture", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.CanControl() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.TogglePiP() },
	},
	{
		ID: ShortcutDrawer, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyC}}, Keys: []string{"C"},
		Label: "Channel drawer", Available: true,
		Enabled: func(s *Shell) bool { return s.watch.HasChannels() },
		Run:     func(s *Shell, _ ui.Key) { s.watch.ToggleDrawer() },
	},
	{
		ID: ShortcutControlCenterCommand, Context: ShortcutPlayer,
		Bindings: []ShortcutBinding{{Key: ui.KeyEscape}}, Keys: []string{"Esc"},
		Label: "Control Center", Available: true,
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
		Label:    "Move", Available: true,
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

// shortcutGroups returns the stable Settings reference. Dynamic availability
// such as an active channel is not used here; only commands implemented in
// this build are listed.
func shortcutGroups() []ShortcutGroup {
	groups := []ShortcutGroup{
		{Title: "Player", Shortcuts: nil},
		{Title: "Browse", Shortcuts: nil},
		{Title: "Control Center", Shortcuts: nil},
	}
	for _, sc := range shortcutRegistry {
		if !sc.Available {
			continue
		}
		switch sc.Context {
		case ShortcutPlayer:
			groups[0].Shortcuts = append(groups[0].Shortcuts, sc)
		case ShortcutBrowse:
			groups[1].Shortcuts = append(groups[1].Shortcuts, sc)
		case ShortcutControlCenter:
			groups[2].Shortcuts = append(groups[2].Shortcuts, sc)
		}
	}

	return groups
}

// shortcutReference draws the read-only shortcut list used by Settings.
func shortcutReference(c *ui.Context, theme *ui.Theme) ui.Element {
	column := ui.Column(c).FillWidth().Gap(18)
	column.Children(func() {
		for _, group := range shortcutGroups() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, group.Title).Font(displayFont()).FontSize(textSize(theme, 19)).
					FontWeight(700).TextColor(theme.Text).Role(ui.RoleHeading).Level(2)
				for _, sc := range group.Shortcuts {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
						ui.Text(c, sc.Label).Font(bodyFont()).FontSize(textSize(theme, 13)).
							TextColor(theme.Text).Grow(1).SingleLine()
						for _, key := range sc.Keys {
							Keycap{Label: key}.View(c)
						}
					})
				}
			})
		}
	})

	return column
}
