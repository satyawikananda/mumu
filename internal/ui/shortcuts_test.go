package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPlayerShortcuts(t *testing.T) {
	shell := NewShell()
	muted, fullscreen, pip, volume := 0, 0, 0, 0
	shell.Watch().OnToggleMute = func() { muted++ }
	shell.Watch().OnFullscreen = func() { fullscreen++ }
	shell.Watch().OnEnterPiP = func() { pip++ }
	shell.Watch().OnVolume = func(v int) { volume = v }
	shell.Watch().SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV", Volume: 50, Channels: []WatchChannel{{Key: "one", Name: "One"}}})
	shell.ShowWatch()

	tt := ui.NewTester(shell.View, 1100, 720)
	tt.Key(0, ui.KeyM)
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyF)
	tt.Key(0, ui.KeyP)
	tt.Key(0, ui.KeyC)
	tt.Key(0, ui.KeySpace)

	if muted != 1 || fullscreen != 1 || pip != 1 {
		t.Fatalf("mute=%d fullscreen=%d pip=%d, want one each", muted, fullscreen, pip)
	}
	if volume != 55 {
		t.Fatalf("volume = %d, want 55", volume)
	}
	if !shell.Watch().DrawerOpen() {
		t.Fatal("C did not open the channel drawer")
	}
}

func TestBrowseShortcuts(t *testing.T) {
	shell := NewShell()
	toggled := ""
	shell.Discover().SetState(DiscoverState{
		Kind:     DiscoverReady,
		Channels: []DiscoverChannel{{Key: "one", Name: "One"}},
	})
	shell.Discover().selectedKey = "one"
	shell.Discover().OnToggleFavorite = func(key string) { toggled = key }
	tt := ui.NewTester(shell.View, 1100, 720)

	tt.Key(0, ui.KeyF)
	if toggled != "one" {
		t.Fatalf("favorite key %q, want one", toggled)
	}

	shell.ShowHome()
	tt.Frame()
	tt.Key(0, ui.KeySlash)
	if shell.Destination() != destDiscover {
		t.Fatalf("destination %q, want Discover after /", shell.Destination())
	}
	tt.Frame()
	tt.Frame()

	tt.Type("tvri")
	if got := shell.Discover().Filters().Query; got != "tvri" {
		t.Fatalf("query %q, want tvri after the search shortcut", got)
	}
}
