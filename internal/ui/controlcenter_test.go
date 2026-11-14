package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestControlCenterUsesRealCounts(t *testing.T) {
	shell := NewShell()
	shell.Library().SetState(LibraryState{
		Kind:      LibraryReady,
		Favorites: []LibraryChannel{{Name: "TVRI"}},
		Sources:   []LibrarySource{{Name: "My List"}},
	})
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.OpenControlCenter()
	tt.Frame()

	for _, text := range []string{"CONTROL CENTER", "1 channel", "TVRI", "1 source", "My List"} {
		if !tt.HasText(text) {
			t.Fatalf("texts %q, want %q", tt.Texts(), text)
		}
	}
}

func TestControlCenterEscapeAndOutsideClick(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.OpenControlCenter()
	tt.Frame()
	if !shell.ControlCenterOpen() {
		t.Fatal("Control Center did not open")
	}
	if _, ok := tt.AccessByRole(ui.RoleDialog, "Control Center"); !ok {
		t.Fatal("Control Center is not a labelled dialog")
	}

	tt.Key(0, ui.KeyEscape)
	if shell.ControlCenterOpen() {
		t.Fatal("Escape did not close Control Center")
	}

	shell.OpenControlCenter()
	tt.Frame()
	tt.ClickAt(8, 8)
	if shell.ControlCenterOpen() {
		t.Fatal("outside click did not close Control Center")
	}
}

func TestControlCenterFocusHint(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.OpenControlCenter()
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Open Discover") && !tt.HasText("Open Favorites") {
		t.Fatalf("texts %q, want the focused card action hint", tt.Texts())
	}
}

func TestControlCenterKeyboardNavigation(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	shell.OpenControlCenter()
	tt.Frame()
	tt.Frame()

	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)

	if got := shell.Destination(); got != destLibrary {
		t.Fatalf("destination %q, want Library from the second card", got)
	}
}

func TestControlCenterRestoresKeyboardFocus(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	labels := []string{"Control Center"}
	for i := 0; i < 4; i++ {
		tt.Key(0, ui.KeyTab)
		for _, label := range labels {
			if !tt.Focused(label) {
				continue
			}
			tt.Key(0, ui.KeyEnter)
			if !shell.ControlCenterOpen() {
				t.Fatalf("Enter on %q did not open Control Center", label)
			}
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if !tt.Focused(label) {
				t.Fatalf("focus %q was not restored", label)
			}
			return
		}
	}
	t.Skip("no keyboard-focused opener found")
}

func TestTopBarKeyboardReachesSettings(t *testing.T) {
	shell := NewShell()
	tt := ui.NewTester(shell.View, 1100, 720)

	for range 3 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Key(0, ui.KeyEnter)
	if got := shell.Destination(); got != destSettings {
		t.Fatalf("destination %q, want Settings after top-bar tabs", got)
	}
}
