package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// settingsPage wires the Sources section into a headless tester.
func settingsPage(s *Settings) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		s.Sources(c, c.Theme())
	}, 800, 420)
}

// The Sources section shows the switch, attribution and playlist shortcut.
func TestSettingsSources(t *testing.T) {
	s := NewSettings()
	tt := settingsPage(s)

	for _, want := range []string{
		"Sources",
		"Show community catalog",
		attributionText,
		"Manage playlists",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if !s.Community() {
		t.Fatal("community catalog should default to on")
	}
}

// Clicking the checkbox flips the page and dispatches the new value.
func TestSettingsCommunityToggle(t *testing.T) {
	s := NewSettings()

	var (
		calls   int
		enabled bool
	)
	s.OnCommunity = func(on bool) {
		calls++
		enabled = on
	}

	tt := settingsPage(s)
	if err := tt.Click("Show community catalog"); err != nil {
		t.Fatal(err)
	}

	if calls != 1 || enabled {
		t.Fatalf("toggle call = %v x%d, want false x1", enabled, calls)
	}
	if s.Community() {
		t.Fatal("page still shows the community catalog on")
	}
}

// A failed write shows an inline error and the previous value.
func TestSettingsCommunityError(t *testing.T) {
	s := NewSettings()
	s.SetCommunity(false)
	s.SetCommunityError("mumu could not save the setting. Try again.")

	tt := settingsPage(s)

	if !tt.HasText("mumu could not save the setting. Try again.") {
		t.Fatalf("texts %q, want the inline error", tt.Texts())
	}
	if s.Community() {
		t.Fatal("error state lost the reverted value")
	}
}

// Manage playlists switches the shell to the Library.
func TestShellSettingsManagePlaylists(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 960, 640)

	openSettings(t, tt)
	if !tt.HasText("Show community catalog") {
		t.Fatalf("texts %q, want the Sources section", tt.Texts())
	}

	if err := tt.Click("Manage playlists"); err != nil {
		t.Fatal(err)
	}
	if shell.nav != destLibrary {
		t.Fatalf("navigation %q, want Library", shell.nav)
	}
	if !tt.HasText("No playlists yet") {
		t.Fatalf("texts %q, want the Library page", tt.Texts())
	}
}
