package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHomeResumeIsExplicit(t *testing.T) {
	resumed := 0
	shell := NewShell()
	shell.Home().OnResume = func() { resumed++ }
	shell.Home().SetState(HomeState{Kind: HomeResume, Channel: "Metro TV", Source: "Community"})
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1100, 720)
	if !tt.HasText("LAST WATCHED") || !tt.HasText("Metro TV") {
		t.Fatalf("texts %q, want the resume hero", tt.Texts())
	}
	if resumed != 0 {
		t.Fatal("Home started playback without a click")
	}
	if err := tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	if resumed != 1 {
		t.Fatalf("resume calls = %d, want 1", resumed)
	}
}

func TestHomeEmptyBrowsesDiscover(t *testing.T) {
	browsed := 0
	shell := NewShell()
	shell.Home().OnBrowse = func() { browsed++ }
	shell.ShowHome()

	tt := ui.NewTester(shell.View, 1100, 720)
	if !tt.HasText("Nothing to resume") {
		t.Fatalf("texts %q, want the empty Home state", tt.Texts())
	}
	if err := tt.Click("Browse Discover"); err != nil {
		t.Fatal(err)
	}
	if browsed != 1 {
		t.Fatalf("browse calls = %d, want 1", browsed)
	}
}
