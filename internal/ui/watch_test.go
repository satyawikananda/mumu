package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// watchPage wires a page into a headless tester.
func watchPage(w *Watch) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		w.View(c, c.Theme())
	}, 960, 640)
}

// An idle Watch offers Discover and starts nothing.
func TestWatchEmpty(t *testing.T) {
	w := NewWatch()
	browsed := 0
	w.OnBrowse = func() { browsed++ }

	tt := watchPage(w)
	if !tt.HasText("Nothing playing") {
		t.Fatalf("texts %q, want the empty state", tt.Texts())
	}
	if err := tt.Click("Browse Discover"); err != nil {
		t.Fatal(err)
	}
	if browsed != 1 {
		t.Fatalf("browse clicked %d times, want 1", browsed)
	}
}

// A resume card names the last channel and plays only when chosen.
func TestWatchResumeNeedsClick(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:    WatchResume,
		Channel: "TVRI Nasional",
		Source:  "Community list",
	})

	resumed := 0
	w.OnResume = func() { resumed++ }

	tt := watchPage(w)
	if !tt.HasText("Resume TVRI Nasional") {
		t.Fatalf("texts %q, want the resume card", tt.Texts())
	}
	if resumed != 0 {
		t.Fatal("resume started playback without a click")
	}
	if err := tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	if resumed != 1 {
		t.Fatalf("resume clicked %d times, want 1", resumed)
	}
}

// Playing shows the real state, live format and the player controls.
func TestWatchPlayingControls(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:    WatchPlaying,
		Channel: "TVRI Nasional",
		Source:  "Community list",
		Status:  "Playing",
		Live:    true,
		Volume:  30,
	})

	fullscreen := 0
	w.OnFullscreen = func() { fullscreen++ }

	tt := watchPage(w)
	for _, want := range []string{"TVRI Nasional", "Community list", "LIVE", "Playing", "Add to Favorites", "Fullscreen"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if !tt.HasText("Picture-in-Picture") {
		t.Fatalf("texts %q, want the PiP control", tt.Texts())
	}

	if err := tt.Click("Fullscreen"); err != nil {
		t.Fatal(err)
	}
	if fullscreen != 1 {
		t.Fatalf("fullscreen clicked %d times, want 1", fullscreen)
	}
}

// A favorite channel offers the removal action.
func TestWatchFavoriteLabel(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:     WatchPlaying,
		Channel:  "TVRI Nasional",
		Favorite: true,
	})

	toggled := 0
	w.OnToggleFavorite = func() { toggled++ }

	tt := watchPage(w)
	if !tt.HasText("Remove from Favorites") {
		t.Fatalf("texts %q, want the removal action", tt.Texts())
	}
	if err := tt.Click("Remove from Favorites"); err != nil {
		t.Fatal(err)
	}
	if toggled != 1 {
		t.Fatalf("favorite toggled %d times, want 1", toggled)
	}
}

// Mute reflects the player state and dispatches the change.
func TestWatchMute(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "A", Muted: true})

	toggled := 0
	w.OnToggleMute = func() { toggled++ }

	tt := watchPage(w)
	if !tt.HasText("Unmute") {
		t.Fatalf("texts %q, want Unmute", tt.Texts())
	}
	if err := tt.Click("Unmute"); err != nil {
		t.Fatal(err)
	}
	if toggled != 1 {
		t.Fatalf("mute toggled %d times, want 1", toggled)
	}
}

// The channel drawer lists channels and switches on Enter.
func TestWatchChannelDrawer(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:    WatchPlaying,
		Channel: "TVRI Nasional",
		Channels: []WatchChannel{
			{Key: "a", Name: "TVRI Nasional", Current: true},
			{Key: "b", Name: "Bernama News"},
		},
	})

	selected := ""
	w.OnSelectChannel = func(key string) { selected = key }

	tt := watchPage(w)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	if !w.DrawerOpen() || !tt.HasText("Bernama News") {
		t.Fatalf("drawer open=%t texts %q", w.DrawerOpen(), tt.Texts())
	}

	// The drawer list follows the volume slider in tab order.
	for range 4 {
		tt.Key(0, ui.KeyTab)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)

	if selected != "a" && selected != "b" {
		t.Fatalf("selected key %q, want a drawer row", selected)
	}
}

// A failed stream offers Retry and its message.
func TestWatchErrorRetry(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:    WatchError,
		Channel: "TVRI Nasional",
		Error:   "The stream did not start in time.",
	})

	retries := 0
	w.OnRetry = func() { retries++ }

	tt := watchPage(w)
	if !tt.HasText("Stream unavailable") || !tt.HasText("The stream did not start in time.") {
		t.Fatalf("texts %q, want the error state", tt.Texts())
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if retries != 1 {
		t.Fatalf("retry clicked %d times, want 1", retries)
	}
}

// The page reports the video box so the app can place the native surface.
func TestWatchReportsVideoFrame(t *testing.T) {
	w := NewWatch()

	var frames []Rect
	w.OnFrame = func(frame Rect) { frames = append(frames, frame) }

	tt := watchPage(w)
	tt.Frame()

	if len(frames) == 0 {
		t.Fatal("no video frame reported")
	}
	if frames[0].W <= 0 || frames[0].H <= 0 {
		t.Fatalf("frame %+v, want a positive box", frames[0])
	}
}
