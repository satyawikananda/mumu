package app

import (
	"testing"

	"mumu/internal/player"
	"mumu/internal/ui"
	"mumu/internal/video"
)

// TestPlayReportsUnsupportedPresentation covers the platform-independent app
// path: an engine factory that reports no video surface must produce the
// presentation error, without a controller or engine.
func TestPlayReportsUnsupportedPresentation(t *testing.T) {
	a, _ := discoverTestApp(t)
	a.newEngine = func(string, int) (player.Engine, error) { return nil, video.ErrUnsupported }

	a.playChannel(playbackTestChannel(), "My list")

	state := a.shell.Watch().State()
	if state.Kind != ui.WatchError || state.Error != presentationUnavailableMessage {
		t.Fatalf("watch state=%+v", state)
	}
	if a.engine != nil || a.controller != nil {
		t.Fatal("unsupported presentation created playback")
	}
}
