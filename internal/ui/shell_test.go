package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// A new shell opens on Discover, in the dark theme, without a welcome screen.
func TestShellDefaultsToDiscover(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 960, 640)

	if got := shell.ErrorMessage(); got != "" {
		t.Errorf("ErrorMessage() = %q, want empty", got)
	}
	if got := shell.themeMode; got != ThemeDark {
		t.Errorf("themeMode = %v, want ThemeDark", got)
	}
	if !tt.HasText("Discover") {
		t.Errorf("texts %q, want Discover", tt.Texts())
	}
	if !tt.HasText("No channels yet") {
		t.Errorf("texts %q, want the Discover empty state", tt.Texts())
	}
	if tt.HasText("Welcome") {
		t.Errorf("texts %q, want no welcome screen", tt.Texts())
	}
}

// The wide sidebar switches pages and shows the selected screen's content.
func TestShellWideNavigation(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	if err := tt.Click("Library"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No favorites yet") {
		t.Errorf("texts %q, want the Library favorites empty state", tt.Texts())
	}

	openSettings(t, tt)
	if !tt.HasText("Appearance") {
		t.Errorf("texts %q, want the appearance setting", tt.Texts())
	}
	if !tt.HasText("mumu 0.1.0 · M2") {
		t.Errorf("texts %q, want the About row", tt.Texts())
	}
}

// Watch shows its empty state and can send the viewer to Discover.
func TestShellWatchEmptyAction(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	if err := tt.Click("Watch"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Nothing playing") {
		t.Fatalf("texts %q, want the Watch empty state", tt.Texts())
	}
	if err := tt.Click("Browse Discover"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No channels yet") {
		t.Fatalf("texts %q, want Discover after Browse Discover", tt.Texts())
	}
}

// System follows the desktop appearance for the content palette.
func TestShellSystemTheme(t *testing.T) {
	shell := NewShell()
	shell.themeMode = ThemeSystem
	tt := ui.NewTester(shell.View, 1100, 720)

	tt.SetDark(true)
	assertPixel(t, tt, 1000, 600, "#151517")

	tt.SetDark(false)
	assertPixel(t, tt, 1000, 600, "#F7F7F8")
}

func assertPixel(t *testing.T, tt *ui.Tester, x, y int, hex string) {
	t.Helper()

	want := ui.Hex(hex)
	got := tt.Image().RGBAAt(x, y)
	if abs(int(got.R)-int(want.R)) > 2 || abs(int(got.G)-int(want.G)) > 2 || abs(int(got.B)-int(want.B)) > 2 {
		t.Fatalf("pixel (%d,%d) = #%02X%02X%02X, want %s", x, y, got.R, got.G, got.B, hex)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}

// The wide sidebar is one Tab stop and the arrows choose items.
func TestShellKeyboardNavigation(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyDown)

	if !tt.HasText("No favorites yet") {
		t.Fatalf("texts %q, want Library after Down", tt.Texts())
	}
}

// The narrow rail navigates without rendering the wide sidebar labels.
func TestShellNarrowRail(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 960, 640)
	tt.SetSize(700, 480)
	tt.Frame()

	if err := tt.Click("Library"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Playlists"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No playlists yet") {
		t.Errorf("texts %q, want the playlists empty state", tt.Texts())
	}
	if n := countText(tt.Texts(), "Library"); n != 2 {
		t.Errorf("texts %q show Library %d times, want the rail item and heading", tt.Texts(), n)
	}
	if n := countText(tt.Texts(), "Discover"); n != 1 {
		t.Errorf("texts %q show Discover %d times, want only the rail item", tt.Texts(), n)
	}
	if tt.HasText("Browse") {
		t.Errorf("texts %q, want no wide sidebar sections", tt.Texts())
	}
}

// The appearance control updates the shell's theme mode in order.
func TestShellThemeControl(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	openSettings(t, tt)

	for _, want := range []struct {
		label string
		mode  ThemeMode
	}{
		{"Light", ThemeLight},
		{"System", ThemeSystem},
		{"Dark", ThemeDark},
	} {
		if err := tt.Click(want.label); err != nil {
			t.Fatal(err)
		}
		if got := shell.themeMode; got != want.mode {
			t.Errorf("themeMode after %s = %v, want %v", want.label, got, want.mode)
		}
	}
}

// Nothing clips at the minimum window size.
func TestShellMinimumSize(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 960, 640)
	tt.SetSize(640, 420)
	tt.Frame()

	r, ok := tt.Find("Discover")
	if !ok {
		t.Fatalf("texts %q, want Discover", tt.Texts())
	}
	if r.X < 0 || r.Y < 0 || r.X+r.W > 640 || r.Y+r.H > 420 {
		t.Errorf("Discover box %+v is outside 640x420", r)
	}

	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	system, ok := tt.Find("System")
	if !ok {
		t.Fatalf("texts %q, want the appearance control", tt.Texts())
	}
	if system.X < 0 || system.Y < 0 || system.X+system.W > 640 || system.Y+system.H > 420 {
		t.Errorf("System box %+v is outside 640x420", system)
	}
}

// An error shows a banner and its Dismiss button clears it.
func TestShellErrorBanner(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 960, 640)

	shell.ShowError("boom")
	tt.Frame()
	if !tt.HasText("boom") {
		t.Errorf("texts %q, want the error banner", tt.Texts())
	}

	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	if got := shell.ErrorMessage(); got != "" {
		t.Errorf("ErrorMessage() = %q, want empty", got)
	}
}

// countText counts the texts exactly matching want.
func countText(texts []string, want string) int {
	var n int

	for _, s := range texts {
		if s == want {
			n++
		}
	}

	return n
}

// openSettings chooses Settings in the sidebar.
func openSettings(t *testing.T, tt *ui.Tester) {
	t.Helper()

	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
}

// Destination changes are reported once, with Watch as a boolean.
func TestShellDestinationChanged(t *testing.T) {
	shell := NewShell()
	var got []bool
	shell.OnDestinationChanged(func(watch bool) { got = append(got, watch) })

	shell.ShowWatch()
	shell.ShowDiscover()
	shell.ShowDiscover()
	shell.ShowWatch()

	want := []bool{true, false, true}
	if len(got) != len(want) {
		t.Fatalf("reports %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reports %v, want %v", got, want)
		}
	}
	if !shell.IsWatch() {
		t.Fatal("IsWatch = false after ShowWatch")
	}
}
