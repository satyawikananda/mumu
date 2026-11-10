package ui

import (
	"strings"
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

	if err := tt.Click("Bernama News"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)

	if selected != "b" {
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

// Loading floats a glass panel over the video box, because the app detaches
// the native surface in this state and nothing else can cover the area.
func TestWatchLoadingOverlayCentered(t *testing.T) {
	w := NewWatch()

	var frame Rect
	w.OnFrame = func(f Rect) { frame = f }

	w.SetState(WatchState{Kind: WatchLoading, Channel: "Metro TV"})

	tt := watchPage(w)
	tt.Frame()

	if !tt.HasText("Loading Metro TV") {
		t.Fatalf("texts %q, want the loading overlay", tt.Texts())
	}
	if tt.HasText("Retry") {
		t.Fatalf("texts %q, want no action while loading", tt.Texts())
	}
	if frame.W <= 0 || frame.H <= 0 {
		t.Fatalf("frame %+v, want a positive video box", frame)
	}

	title, ok := tt.Find("Loading Metro TV")
	if !ok {
		t.Fatal("loading title has no bounds")
	}

	// The panel fills the box and centers inside it; a title at the top-left
	// corner would mean the overlay did not stretch.
	boxCenterX := frame.X + frame.W/2
	if diff := title.X + title.W/2 - boxCenterX; diff > 4 || diff < -4 {
		t.Fatalf("title center %.1f, box center %.1f: overlay is not centered", title.X+title.W/2, boxCenterX)
	}
	if title.Y < frame.Y || title.Y+title.H > frame.Y+frame.H {
		t.Fatalf("title %+v, want it inside the video box %+v", title, frame)
	}
}

// A failed switch offers Retry and the channel that played before it.
func TestWatchErrorReturnsToPrevious(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:     WatchError,
		Channel:  "ANTV",
		Previous: "Metro TV",
		Error:    "The stream did not start in time.",
	})

	retries, backs := 0, 0
	w.OnRetry = func() { retries++ }
	w.OnBack = func() { backs++ }

	tt := watchPage(w)
	if !tt.HasText("Back to Metro TV") {
		t.Fatalf("texts %q, want the return action", tt.Texts())
	}

	if err := tt.Click("Back to Metro TV"); err != nil {
		t.Fatal(err)
	}
	if backs != 1 {
		t.Fatalf("back clicked %d times, want 1", backs)
	}

	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if retries != 1 {
		t.Fatalf("retry clicked %d times, want 1", retries)
	}
}

// Without an earlier channel the error keeps a single action.
func TestWatchErrorWithoutPrevious(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchError, Channel: "ANTV", Error: "Failed."})

	tt := watchPage(w)
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "Back to ") {
			t.Fatalf("texts %q, want no return action", tt.Texts())
		}
	}
	if !tt.HasText("Retry") {
		t.Fatalf("texts %q, want Retry", tt.Texts())
	}
}

// Buffering keeps the video and reports the stall below it, never over it:
// an attached native surface would hide a badge drawn inside the box.
func TestWatchBufferingKeepsVideo(t *testing.T) {
	w := NewWatch()

	var frame Rect
	w.OnFrame = func(f Rect) { frame = f }

	w.SetState(WatchState{Kind: WatchBuffering, Channel: "Metro TV", Status: "Waiting for data"})

	tt := watchPage(w)
	tt.Frame()

	badge, ok := tt.Find("Buffering")
	if !ok {
		t.Fatalf("texts %q, want the buffering badge", tt.Texts())
	}
	if frame.H <= 0 {
		t.Fatalf("frame %+v, want the video box to stay", frame)
	}
	if badge.Y < frame.Y+frame.H {
		t.Fatalf("badge %+v, want it below the video box %+v", badge, frame)
	}
}

// ResetFrame makes a rebuilt window report its video box again.
func TestWatchResetFrame(t *testing.T) {
	w := NewWatch()

	reports := 0
	w.OnFrame = func(Rect) { reports++ }

	tt := watchPage(w)
	tt.Frame()
	first := reports
	if first == 0 {
		t.Fatal("no frame reported")
	}

	// Same box: nothing to report again.
	w.SetState(WatchState{Kind: WatchLoading, Channel: "Metro TV"})
	tt.Frame()

	w.ResetFrame()
	tt.Frame()
	if reports <= first {
		t.Fatalf("frames reported %d, want more than %d after ResetFrame", reports, first)
	}
}
