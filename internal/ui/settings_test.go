package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// settingsPage draws the full Settings page in a headless tester.
func settingsPage(s *Settings) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		s.View(c, c.Theme())
	}, 1100, 720)
}

// The Appearance panel shows the theme and the start-muted control only.
func TestSettingsAppearance(t *testing.T) {
	s := NewSettings()
	tt := settingsPage(s)

	for _, want := range []string{
		"Appearance",
		"THEME",
		"Dark",
		"Light",
		"System",
		"PLAYBACK",
		"Start muted",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	for _, removed := range []string{"Default quality", "SOURCES", "Show community catalog", "Manage playlists", "Import M3U"} {
		if tt.HasText(removed) {
			t.Fatalf("texts %q, want no removed control %q", tt.Texts(), removed)
		}
	}
	if s.Section() != SettingsAppearance {
		t.Fatalf("section = %v, want Appearance", s.Section())
	}
	if s.StartMuted() {
		t.Fatal("start muted should default to off")
	}
}

// Clicking a theme card applies it and updates the selected card.
func TestSettingsTheme(t *testing.T) {
	s := NewSettings()
	var got ThemeMode
	s.OnTheme = func(mode ThemeMode) { got = mode }

	tt := settingsPage(s)
	if err := tt.Click("Light"); err != nil {
		t.Fatal(err)
	}
	if got != ThemeLight || s.Theme() != ThemeLight {
		t.Fatalf("theme = %v, callback = %v, want Light", s.Theme(), got)
	}
}

// Clicking the start-muted label toggles the switch and dispatches it.
func TestSettingsStartMuted(t *testing.T) {
	s := NewSettings()
	var (
		calls int
		got   bool
	)
	s.OnStartMuted = func(muted bool) {
		calls++
		got = muted
	}

	tt := settingsPage(s)
	if err := tt.Click("Start muted"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !got || !s.StartMuted() {
		t.Fatalf("start muted = %v, callback = %v x%d, want true x1", s.StartMuted(), got, calls)
	}
}

// The shortcut panel renders the designed Player, Browse and Control Center cards.
func TestSettingsShortcuts(t *testing.T) {
	s := NewSettings()
	tt := settingsPage(s)

	if err := tt.Click("Shortcuts"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Shortcuts") {
		t.Fatalf("texts %q, want the rail item focused after it opens the panel", tt.Texts())
	}
	for _, want := range []string{
		"PLAYER", "BROWSE", "CONTROL CENTER",
		"Play or pause", "Mute", "Volume", "Fullscreen", "Picture in picture",
		"Channel drawer", "Control Center", "Search", "Move between cards",
		"Open", "Favorite", "Back", "Close",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if tt.HasText("Resume playlists") {
		t.Fatalf("texts %q, want no deferred playlist command", tt.Texts())
	}
	if s.Section() != SettingsShortcuts {
		t.Fatalf("section = %v, want Shortcuts", s.Section())
	}

	groups := shortcutGroups()
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	seen := 0
	for _, group := range groups {
		for _, sc := range group.Shortcuts {
			seen++
			if !sc.Available {
				t.Fatalf("shortcut %q is unavailable but listed", sc.Label)
			}
		}
	}
	if seen != 15 {
		t.Fatalf("shortcut reference lists %d commands, want the 15 designed rows", seen)
	}
}

// About shows actual data, not the old placeholder copy.
func TestSettingsAbout(t *testing.T) {
	s := NewSettings()
	s.SetAbout(AboutInfo{
		Version:       "0.2.0",
		Commit:        "8eea19e",
		GoVersion:     "1.27.3",
		MyGoVersion:   "v0.9.0",
		OS:            "darwin",
		Arch:          "arm64",
		License:       "MIT",
		ChannelCount:  1284,
		SourceCount:   1,
		CountsKnown:   true,
		RepositoryURL: "https://github.com/satyawikananda/mumu",
	})

	tt := settingsPage(s)
	if err := tt.Click("About"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Mumu TV", "0.2.0 · 8eea19e", "1,284 channels · 1 source",
		"MIT", "github.com/satyawikananda/mumu",
		"iptv-org (CC0)", "Lucide (ISC)", "Lexend · Inter Tight (OFL 1.1)",
		"MyGo 0.9.0", "Go 1.27.3 · darwin/arm64",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	for _, removed := range []string{"Bundled notices", "Logs", "Open log folder"} {
		if tt.HasText(removed) {
			t.Fatalf("texts %q, want no removed row %q", tt.Texts(), removed)
		}
	}
	for _, stale := range []string{
		"0.1.0 · M2", "mumu.run", "Go 1.24", "Inter · JetBrains Mono",
		"Playback settings arrive in #40",
	} {
		if tt.HasText(stale) {
			t.Fatalf("texts %q, want no stale placeholder %q", tt.Texts(), stale)
		}
	}
	if !tt.HasText("Display") {
		t.Fatalf("texts %q, want display metrics", tt.Texts())
	}
}

// Copy system info writes a redacted report through the window's clipboard.
func TestSettingsCopySystemInfo(t *testing.T) {
	s := NewSettings()
	s.SetAbout(AboutInfo{
		Version:      "0.2.0",
		Commit:       "8eea19e",
		GoVersion:    "1.27.3",
		MyGoVersion:  "v0.9.0",
		OS:           "darwin",
		Arch:         "arm64",
		ChannelCount: 8,
		SourceCount:  2,
		CountsKnown:  true,
	})

	tt := settingsPage(s)
	if err := tt.Click("About"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Copy system info"); err != nil {
		t.Fatal(err)
	}

	info := tt.Clipboard()
	for _, want := range []string{
		"Mumu TV 0.2.0 · 8eea19e", "Runtime Go 1.27.3 · darwin/arm64",
		"Catalog 8 channels · 2 sources", "Display 1100 × 720",
	} {
		if !strings.Contains(info, want) {
			t.Fatalf("clipboard %q, want %q", info, want)
		}
	}
	for _, private := range []string{"http", "/Users/", "stream.example"} {
		if strings.Contains(info, private) {
			t.Fatalf("clipboard %q contains private data %q", info, private)
		}
	}
}

// The manual check exposes checking, current, available and error states.
func TestSettingsUpdateStates(t *testing.T) {
	s := NewSettings()
	var checks int
	s.OnCheckUpdates = func() { checks++ }

	tt := settingsPage(s)
	if err := tt.Click("About"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Check for updates"); err != nil {
		t.Fatal(err)
	}
	if checks != 1 || s.Update().Kind != UpdateChecking {
		t.Fatalf("checks=%d state=%v, want one checking check", checks, s.Update().Kind)
	}
	if !tt.HasText("Checking GitHub releases…") {
		t.Fatalf("texts %q, want the checking state", tt.Texts())
	}

	s.SetUpdate(UpdateState{
		Kind:    UpdateCurrent,
		Current: "0.2.0",
	})
	tt.Frame()
	if !tt.HasText("Up to date · 0.2.0") {
		t.Fatalf("texts %q, want the current state", tt.Texts())
	}

	s.SetUpdate(UpdateState{
		Kind:       UpdateAvailable,
		Current:    "0.2.0",
		Latest:     "0.3.0",
		ReleaseURL: "https://github.com/satyawikananda/mumu/releases/tag/v0.3.0",
	})
	tt.Frame()
	if !tt.HasText("Mumu TV 0.3.0 is available.") {
		t.Fatalf("texts %q, want the available state", tt.Texts())
	}
	if err := tt.Click("Open release page"); err != nil {
		t.Fatal(err)
	}
	opened := tt.OpenedURLs()
	if len(opened) != 1 || opened[0] != "https://github.com/satyawikananda/mumu/releases/tag/v0.3.0" {
		t.Fatalf("opened %v, want the verified release page", opened)
	}

	s.SetUpdate(UpdateState{Kind: UpdateNoRelease})
	tt.Frame()
	if !tt.HasText("No stable release is published yet.") {
		t.Fatalf("texts %q, want the no-release state", tt.Texts())
	}

	s.SetUpdate(UpdateState{Kind: UpdateError, Message: "GitHub rate limit reached. Try again later."})
	tt.Frame()
	if !tt.HasText("GitHub rate limit reached. Try again later.") {
		t.Fatalf("texts %q, want the explicit error", tt.Texts())
	}
}

// Settings fits the compact reference size.
func TestSettingsCompact(t *testing.T) {
	s := NewSettings()
	s.SetAbout(AboutInfo{
		Version: "0.2.0", GoVersion: "1.27.3", MyGoVersion: "v0.9.0",
		OS: "darwin", Arch: "arm64",
		ChannelCount: 1284, SourceCount: 1, CountsKnown: true,
	})
	tt := ui.NewTester(func(c *ui.Context) {
		s.View(c, c.Theme())
	}, 1024, 640)

	for _, text := range []string{"Appearance", "Dark", "Start muted"} {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("texts %q, want %q", tt.Texts(), text)
		}
		if r.X < 0 || r.Y < 0 || r.X+r.W > 1024 || r.Y+r.H > 640 {
			t.Errorf("%q box %+v is outside 1024x640", text, r)
		}
	}

	if err := tt.Click("About"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("0.2.0 · 8eea19e"); ok {
		t.Fatal("about shows a commit the app did not publish")
	}
}

// Switching panels resets the panel scroll and fits the compact reference.
func TestSettingsResetScroll(t *testing.T) {
	s := NewSettings()
	s.ShowSection(SettingsAbout)

	tt := ui.NewTester(func(c *ui.Context) {
		s.View(c, c.Theme())
	}, 1024, 640)

	s.scroll.Y = 200
	tt.Frame()
	if !s.Scrolled() {
		t.Fatal("the panel did not scroll")
	}

	if err := tt.Click("Appearance"); err != nil {
		t.Fatal(err)
	}
	if s.Scrolled() {
		t.Fatal("switching panels kept the previous scroll offset")
	}
}
