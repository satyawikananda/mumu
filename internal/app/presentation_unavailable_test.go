//go:build !(darwin && arm64)

package app

import (
	"testing"

	"mumu/internal/ui"
)

func TestWatchRefusesUnsupportedPresentation(t *testing.T) {
	a, _ := discoverTestApp(t)
	a.newEngine = newWatchEngine
	a.playChannel(playbackTestChannel(), "My list")
	state := a.shell.Watch().State()
	if state.Kind != ui.WatchError || state.Error != presentationUnavailableMessage {
		t.Fatalf("watch state=%+v", state)
	}
	if a.engine != nil || a.controller != nil {
		t.Fatal("unsupported presentation created playback")
	}
}
