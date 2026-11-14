package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A new shell opens on Discover without a permanent sidebar.
func TestShellDefaultsToDiscover(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	if got := shell.ErrorMessage(); got != "" {
		t.Errorf("ErrorMessage() = %q, want empty", got)
	}
	if got := shell.themeMode; got != ThemeDark {
		t.Errorf("themeMode = %v, want ThemeDark", got)
	}
	if !tt.HasText("Discover") || !tt.HasText("Choose a country to get started") {
		t.Errorf("texts %q, want Discover and its no-country state", tt.Texts())
	}
	if !tt.HasText(ProductName) {
		t.Errorf("texts %q, want the product wordmark", tt.Texts())
	}
	if tt.HasText("HOME") || tt.HasText("LIBRARY") {
		t.Errorf("texts %q, still contain sidebar destinations", tt.Texts())
	}
	if err := tt.Click("Control Center"); err != nil {
		t.Fatalf("Control Center icon is not clickable: %v", err)
	}
}

// The header brand returns to Home instead of opening Control Center.
func TestShellBrandGoesHome(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.ShowLibrary()
	tt.Frame()
	if err := tt.Click("Go to Home"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destHome {
		t.Fatalf("destination %q, want Home after the brand click", got)
	}
}

// The header Back walks the visit history and disappears at its start.
func TestShellHeaderBack(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	if shell.CanGoBack() {
		t.Fatal("fresh shell reports a Back destination")
	}

	shell.ShowLibrary()
	shell.ShowSettings()
	tt.Frame()
	if !shell.CanGoBack() {
		t.Fatal("two visits did not fill the Back history")
	}

	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destLibrary {
		t.Fatalf("destination %q, want Library after one Back", got)
	}
	if !shell.CanGoBack() {
		t.Fatal("Back history lost the first visit")
	}

	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destDiscover {
		t.Fatalf("destination %q, want Discover after the second Back", got)
	}
	if shell.CanGoBack() {
		t.Fatal("Back remains after walking the whole history")
	}
}

// Control Center is reachable by pointer and exposes the page destinations.
func TestShellControlCenterMouse(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	if err := tt.Click("Control Center"); err != nil {
		t.Fatal(err)
	}
	if !shell.ControlCenterOpen() || !tt.HasText("CONTROL CENTER") {
		t.Fatalf("texts %q, want the open Control Center", tt.Texts())
	}
	for _, label := range []string{"Go to Home", "Go to Discover", "Go to Watch", "Picture in Picture", "Close Control Center"} {
		if !tt.HasText(label) {
			t.Errorf("texts %q, want %q", tt.Texts(), label)
		}
	}

	if err := tt.Click("Go to Home"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destHome {
		t.Fatalf("destination %q, want Home", got)
	}
	if shell.ControlCenterOpen() || shell.IsWatch() {
		t.Fatal("Home navigation left Control Center open or Watch selected")
	}
	if !tt.HasText("Nothing to watch yet") {
		t.Errorf("texts %q, want the Home empty state", tt.Texts())
	}
}

// Every page remains reachable with the pointer through Control Center or the
// top bar.
func TestShellDestinationsReachable(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	open := func() {
		t.Helper()
		if err := tt.Click("Control Center"); err != nil {
			t.Fatal(err)
		}
	}

	open()
	if err := tt.Click("Go to Discover"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destDiscover {
		t.Fatalf("destination %q, want Discover", got)
	}

	open()
	if err := tt.Click("Go to Watch"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destWatch {
		t.Fatalf("destination %q, want Watch", got)
	}

	open()
	if err := tt.Click("No favorites yet"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destLibrary {
		t.Fatalf("destination %q, want Library", got)
	}

	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destSettings {
		t.Fatalf("destination %q, want Settings", got)
	}
}

// Home and Watch are distinct destinations and never share a page.
func TestShellHomeWatchDistinct(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.ShowHome()
	tt.Frame()
	if got := shell.Destination(); got != destHome {
		t.Fatalf("destination %q, want Home", got)
	}
	if shell.IsWatch() || !tt.HasText("Nothing to watch yet") {
		t.Fatalf("texts %q, want the Home empty state", tt.Texts())
	}

	shell.ShowWatch()
	tt.Frame()
	if !shell.IsWatch() || !tt.HasText("Nothing playing") {
		t.Fatalf("texts %q, want Watch", tt.Texts())
	}
}

// Escape closes the top layer, then returns to the previous destination.
func TestShellEscapeLayers(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.ShowLibrary()
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if got := shell.Destination(); got != destDiscover {
		t.Fatalf("destination %q, want Discover after Back", got)
	}

	shell.ShowHome()
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if !shell.ControlCenterOpen() {
		t.Fatal("Escape on Home did not open Control Center")
	}
	tt.Key(0, ui.KeyEscape)
	if shell.ControlCenterOpen() {
		t.Fatal("Escape did not close Control Center")
	}

	shell.ShowWatch()
	shell.Watch().ToggleDrawer()
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if shell.Watch().DrawerOpen() || shell.ControlCenterOpen() {
		t.Fatal("Escape did not close the Watch drawer first")
	}
	tt.Key(0, ui.KeyEscape)
	if !shell.ControlCenterOpen() {
		t.Fatal("Escape on Watch did not open Control Center")
	}
}

// A focused text field keeps typing keys and never triggers global commands.
func TestShellInputShortcutConflict(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	if err := tt.Click("Search channels"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/M ")
	if got := shell.Discover().Filters().Query; got != "/M" {
		t.Fatalf("query %q, want /M", got)
	}
	if shell.Destination() != destDiscover || shell.ControlCenterOpen() {
		t.Fatal("typing in search triggered navigation")
	}
}

// Settings and hint bars read the one shortcut registry.
func TestShellShortcutReference(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	openSettings(t, tt)
	if err := tt.Click("Shortcuts"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Mute") || !tt.HasText("Fullscreen") || !tt.HasText("Control Center") {
		t.Fatalf("texts %q, want shared shortcut labels", tt.Texts())
	}
	if !tt.HasText("Play or pause") || !tt.HasText("Channel drawer") {
		t.Fatalf("texts %q, want the implemented player actions", tt.Texts())
	}

	groups := shortcutGroups()
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	for _, group := range groups {
		for _, sc := range group.Shortcuts {
			if !sc.Available {
				t.Fatalf("shortcut %q is unavailable but listed", sc.Label)
			}
		}
	}
}

// Control Center fits the compact reference size without clipping.
func TestShellControlCenterCompact(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1024, 640)

	if err := tt.Click("Control Center"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"CONTROL CENTER", "Go to Home", "Close Control Center"} {
		if !tt.HasText(text) {
			t.Fatalf("texts %q, want %q", tt.Texts(), text)
		}
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
	if _, ok := tt.Find("System"); !ok {
		t.Fatalf("texts %q, want the appearance control", tt.Texts())
	}

	tt.Scroll(500, 300, 0, 400)
	tt.Frame()
	system, ok := tt.Find("System")
	if !ok {
		t.Fatalf("texts %q, want the appearance control after scrolling", tt.Texts())
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

// Watch is full-bleed: the shell bar, its inset and its hint strip stay out
// of the picture.
func TestShellWatchFullBleed(t *testing.T) {
	shell := NewShell()
	shell.ShowWatch()
	tt := ui.NewTester(shell.View, 1280, 720)

	if tt.HasText("WATCH") {
		t.Fatalf("texts %q, want no shell destination label over the picture", tt.Texts())
	}
	if tt.HasText("Open Control Center") {
		t.Fatalf("texts %q, want no shell top bar on Watch", tt.Texts())
	}
	if !tt.HasText("Nothing playing") {
		t.Fatalf("texts %q, want the Watch page itself", tt.Texts())
	}
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if got := shell.Destination(); got != destDiscover {
		t.Fatalf("destination %q, want Discover after Back", got)
	}
}

// Escape cancels a first connect instead of opening Control Center.
func TestShellEscapeCancelsConnecting(t *testing.T) {
	shell := NewShell()
	cancelled := 0
	shell.Watch().OnCancel = func() { cancelled++ }
	shell.Watch().SetState(WatchState{Kind: WatchLoading, Channel: "TVRI"})
	shell.ShowWatch()

	tt := ui.NewTester(shell.View, 1100, 720)
	tt.Key(0, ui.KeyEscape)
	if cancelled != 1 {
		t.Fatalf("cancel called %d times, want 1", cancelled)
	}
	if shell.ControlCenterOpen() {
		t.Fatal("Escape opened Control Center while connecting")
	}
}

// Control Center counts as activity: the page's chrome is still there when
// the overlay closes.
func TestShellControlCenterKeepsWatchChrome(t *testing.T) {
	shell := NewShell()
	shell.Watch().SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})
	shell.ShowWatch()

	tt := ui.NewTester(shell.View, 1280, 720)
	if err := tt.Click("Control Center"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(5 * time.Second)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if shell.ControlCenterOpen() {
		t.Fatal("Escape did not close Control Center")
	}
	if !shell.Watch().ChromeVisible() {
		t.Fatal("the Watch chrome idled away under Control Center")
	}
}

// A handled player key brings hidden chrome back for a keyboard viewer.
func TestShellKeyRevealsChrome(t *testing.T) {
	shell := NewShell()
	muted := 0
	shell.Watch().OnToggleMute = func() { muted++ }
	shell.Watch().SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})
	shell.ShowWatch()

	tt := ui.NewTester(shell.View, 1280, 720)
	tt.Advance(3 * time.Second)
	if shell.Watch().ChromeVisible() {
		t.Fatal("chrome did not hide")
	}

	tt.Key(0, ui.KeyM)
	tt.Frame()
	if !shell.Watch().ChromeVisible() {
		t.Fatal("a handled key did not reveal the chrome")
	}
	if muted != 1 {
		t.Fatalf("mute toggled %d times, want 1", muted)
	}
}

// Space has no session to pause while the first connect runs.
func TestShellSpaceIgnoresConnecting(t *testing.T) {
	shell := NewShell()
	paused := 0
	shell.Watch().OnTogglePause = func() { paused++ }
	shell.Watch().SetState(WatchState{Kind: WatchLoading, Channel: "TVRI"})
	shell.ShowWatch()

	tt := ui.NewTester(shell.View, 1100, 720)
	tt.Key(0, ui.KeySpace)
	if paused != 0 {
		t.Fatalf("Space paused %d times while connecting, want none", paused)
	}
}

// Cmd+, opens Settings from the Watch route.
func TestShellCmdCommaSettings(t *testing.T) {
	shell := NewShell()
	shell.ShowWatch()
	tt := ui.NewTester(shell.View, 1100, 720)

	tt.Key(ui.Cmd, ui.KeyComma)
	if got := shell.Destination(); got != destSettings {
		t.Fatalf("destination %q, want Settings", got)
	}
}

// Space and R reach the player actions from the shell registry.
func TestShellWatchSpaceAndRetry(t *testing.T) {
	shell := NewShell()
	paused, retried := 0, 0
	shell.Watch().OnTogglePause = func() { paused++ }
	shell.Watch().OnRetry = func() { retried++ }
	shell.Watch().SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})
	shell.ShowWatch()

	tt := ui.NewTester(shell.View, 1100, 720)
	tt.Key(0, ui.KeySpace)
	if paused != 1 {
		t.Fatalf("Space paused %d times, want 1", paused)
	}

	shell.Watch().SetState(WatchState{Kind: WatchError, Channel: "Metro TV", Error: "Failed."})
	tt.Frame()
	tt.Key(0, ui.KeyR)
	if retried != 1 {
		t.Fatalf("R retried %d times, want 1 in the error state", retried)
	}
}

// openSettings chooses Settings in the top bar.
func openSettings(t *testing.T, tt *ui.Tester) {
	t.Helper()

	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
}

// Escape closes the Library import sheet before it can navigate back.
func TestShellEscapeClosesLibrarySheet(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.ShowLibrary()
	shell.Library().OpenAddPlaylist()
	tt.Frame()
	if !shell.Library().SheetOpen() {
		t.Fatal("import sheet did not open")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if shell.Library().SheetOpen() {
		t.Fatal("Escape did not close the import sheet")
	}
	if got := shell.Destination(); got != destLibrary {
		t.Fatalf("destination %q, want Library after closing the sheet", got)
	}
}

// Control Center remains usable at the application's minimum window.
func TestShellControlCenterMinimumWindow(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 640, 420)

	if err := tt.Click("Control Center"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"CONTROL CENTER", "Go to Home", "Close Control Center"} {
		if !tt.HasText(text) {
			t.Fatalf("texts %q, want %q", tt.Texts(), text)
		}
	}
}
