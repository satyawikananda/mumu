package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// watchPage wires the Watch page into a headless tester at one size.
func watchPage(w *Watch, width, height int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		w.View(c, c.Theme())
	}, width, height)
}

// watchSizes are the three reference window sizes.
var watchSizes = []struct {
	name   string
	width  int
	height int
}{
	{name: "wide", width: 1470, height: 956},
	{name: "medium", width: 1280, height: 720},
	{name: "compact", width: 1024, height: 640},
}

// watchChannels returns deterministic drawer rows.
func watchChannels(count int) []WatchChannel {
	channels := make([]WatchChannel, 0, count)
	for i := range count {
		channels = append(channels, WatchChannel{
			Key:  fmt.Sprintf("key-%d", i),
			Name: fmt.Sprintf("Channel %02d", i),
			Meta: "News · Community",
			Live: i%2 == 0,
		})
	}

	return channels
}

// A playing page shows the channel, the honest state and every transport
// action, including the favorite control the frames omit.
func TestWatchPlayingControls(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:    WatchPlaying,
		Channel: "TVRI Nasional",
		Source:  "Community list",
		Status:  "Playing",
		Live:    true,
		Volume:  30,
		Quality: WatchQuality{Selected: "auto", Current: "720p", Known: true},
	})
	fullscreen, favorite, muted, paused := 0, 0, 0, 0
	w.OnFullscreen = func() { fullscreen++ }
	w.OnToggleFavorite = func() { favorite++ }
	w.OnToggleMute = func() { muted++ }
	w.OnTogglePause = func() { paused++ }

	tt := watchPage(w, 1470, 956)
	for _, want := range []string{
		"TVRI Nasional", "Community list · 720p", "TVRI Nasional · Live broadcast",
		"LIVE", "720p", "Pause", "Mute", "Volume", "Add to Favorites",
		"Channels", "Quality", "Picture in Picture", "Fullscreen",
		"Back", "Control Center", "Settings",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}

	for _, label := range []string{"Fullscreen", "Add to Favorites", "Pause", "Mute"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %s: %v", label, err)
		}
	}
	if fullscreen != 1 || favorite != 1 || paused != 1 || muted != 1 {
		t.Fatalf("fullscreen=%d favorite=%d pause=%d mute=%d, want one each",
			fullscreen, favorite, paused, muted)
	}
}

// A favorite channel offers the removal action.
func TestWatchFavoriteLabel(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "TVRI", Favorite: true})

	toggled := 0
	w.OnToggleFavorite = func() { toggled++ }

	tt := watchPage(w, 1280, 720)
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

// The quality menu lists Auto plus the real declared labels and reports the
// selection.
func TestWatchQualityMenu(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:    WatchPlaying,
		Channel: "Metro TV",
		Quality: WatchQuality{Selected: "auto", Current: "1080p", Known: true},
		Qualities: []WatchQualityOption{
			{Label: "auto", Selected: true},
			{Label: "720p"},
			{Label: "1080p"},
		},
	})
	selected := ""
	w.OnSelectQuality = func(label string) { selected = label }

	tt := watchPage(w, 1280, 720)
	if err := tt.Click("Quality"); err != nil {
		t.Fatal(err)
	}
	if !w.QualityOpen() {
		t.Fatal("quality menu did not open")
	}
	for _, want := range []string{"QUALITY", "Auto", "720p", "1080p"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if auto, ok := tt.Access("Auto"); !ok || !auto.Selected {
		t.Fatalf("Auto selected=%v found=%v, want the current option", auto.Selected, ok)
	}

	if err := tt.Click("720p"); err != nil {
		t.Fatal(err)
	}
	if selected != "720p" {
		t.Fatalf("selected %q, want 720p", selected)
	}
	tt.Frame()
	if w.QualityOpen() {
		t.Fatal("quality menu stayed open after a choice")
	}
}

// An unknown resolution is said plainly instead of invented.
func TestWatchQualityUnknown(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:      WatchPlaying,
		Channel:   "Metro TV",
		Quality:   WatchQuality{Selected: "auto"},
		Qualities: []WatchQualityOption{{Label: "auto", Selected: true}},
	})

	tt := watchPage(w, 1024, 640)
	if err := tt.Click("Quality"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("This stream doesn't report its resolution.") {
		t.Fatalf("texts %q, want the unknown-resolution note", tt.Texts())
	}
	if !tt.HasText("Auto") {
		t.Fatalf("texts %q, want the Auto option", tt.Texts())
	}
}

// Three seconds without activity hide title, live line and buttons together,
// leaving the chevron as the idle affordance.
func TestWatchChromeIdles(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV", Live: true})

	tt := watchPage(w, 1470, 956)
	if !w.ChromeVisible() {
		t.Fatal("chrome started hidden")
	}

	tt.Advance(3 * time.Second)
	if w.ChromeVisible() {
		t.Fatal("chrome stayed after three idle seconds")
	}
	for _, hidden := range []string{"Pause", "Mute", "Volume", "Metro TV · Live broadcast"} {
		if tt.HasText(hidden) {
			t.Fatalf("texts %q, still show %q after idling", tt.Texts(), hidden)
		}
	}
	if !tt.HasText("Open Control Center") {
		t.Fatalf("texts %q, want the reveal chevron", tt.Texts())
	}
}

// Input activity reveals hidden chrome and reports the change once.
func TestWatchNoteActivity(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})

	tt := watchPage(w, 1280, 720)
	tt.Advance(3 * time.Second)
	if w.ChromeVisible() {
		t.Fatal("chrome did not hide")
	}

	if changed := w.NoteActivity(tt.Now()); !changed {
		t.Fatal("NoteActivity reported no change for hidden chrome")
	}
	tt.Frame()
	if !w.ChromeVisible() {
		t.Fatal("NoteActivity did not show the chrome")
	}
	if changed := w.NoteActivity(tt.Now()); changed {
		t.Fatal("NoteActivity reported a change for visible chrome")
	}
}

// A single pointer move brings the whole chrome back: title, live line and
// every transport button.
func TestWatchPointerMoveReveals(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV", Live: true})

	tt := watchPage(w, 1470, 956)
	tt.Advance(3 * time.Second)
	if w.ChromeVisible() {
		t.Fatal("chrome did not hide")
	}
	if tt.HasText("Pause") || tt.HasText("Metro TV · Live broadcast") {
		t.Fatalf("texts %q, want the chrome hidden", tt.Texts())
	}

	tt.Move(200, 200)
	tt.Frame()
	if !w.ChromeVisible() {
		t.Fatal("a pointer move did not reveal the chrome")
	}
	for _, want := range []string{
		"Metro TV", "Metro TV · Live broadcast",
		"Live stream, playing at the live edge.",
		"Pause", "Mute", "Volume", "Fullscreen", "Quality", "Channels",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q after a single move", tt.Texts(), want)
		}
	}

	// A click also reveals it, and it hides again after the idle delay.
	tt.Advance(3 * time.Second)
	if w.ChromeVisible() {
		t.Fatal("chrome did not hide again")
	}
	tt.ClickAt(200, 200)
	tt.Frame()
	if !w.ChromeVisible() {
		t.Fatal("a click did not reveal the chrome")
	}
}

// A pointer move also resets the idle deadline while the chrome shows.
func TestWatchNotePointerMoveResetsDeadline(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})

	tt := watchPage(w, 1280, 720)
	tt.Advance(2 * time.Second)
	if !w.ChromeVisible() {
		t.Fatal("chrome hid before the idle delay")
	}

	w.NotePointerMove(tt.Now())
	tt.Frame()
	tt.Advance(2 * time.Second)
	if !w.ChromeVisible() {
		t.Fatal("pointer movement did not reset the idle deadline")
	}

	tt.Advance(1100 * time.Millisecond)
	if w.ChromeVisible() {
		t.Fatal("chrome stayed past the reset deadline")
	}
}

// The pointer hides while the chrome is idle and returns with it.
func TestWatchCursorFollowsChrome(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Metro TV", Live: true,
		Channels: watchChannels(3),
	})
	tt := watchPage(w, 1470, 956)
	tt.Move(400, 300)
	tt.Frame()

	tt.Advance(3 * time.Second)
	if w.ChromeVisible() {
		t.Fatal("chrome did not hide")
	}
	if got := tt.Cursor(); got != ui.CursorNone {
		t.Fatalf("cursor = %v over the idle picture, want CursorNone", got)
	}

	// A single move reveals the chrome and the pointer with it.
	tt.Move(410, 310)
	tt.Frame()
	if !w.ChromeVisible() {
		t.Fatal("a pointer move did not reveal the chrome")
	}
	if got := tt.Cursor(); got == ui.CursorNone {
		t.Fatalf("cursor = %v after the reveal, want a visible pointer", got)
	}

	// Paused keeps the chrome, so the pointer stays visible.
	w.SetState(WatchState{Kind: WatchPaused, Channel: "Metro TV", Live: true})
	tt.Frame()
	tt.Advance(5 * time.Second)
	if got := tt.Cursor(); got == ui.CursorNone {
		t.Fatalf("cursor = %v while paused, want a visible pointer", got)
	}

	// The drawer pins the chrome too.
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Metro TV", Live: true,
		Channels: watchChannels(3),
	})
	tt.Frame()
	tt.Advance(3 * time.Second)
	if w.ChromeVisible() {
		t.Fatal("chrome did not hide before the drawer")
	}
	w.ToggleDrawer()
	tt.Frame()
	if got := tt.Cursor(); got == ui.CursorNone {
		t.Fatalf("cursor = %v over the open drawer, want a visible pointer", got)
	}
}

// The top-bar Back is the route action; the error screen's previous-channel
// action is a separate callback.
func TestWatchBackVsPreviousChannel(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV", Live: true})
	backs, previous := 0, 0
	w.OnBack = func() { backs++ }
	w.OnPreviousChannel = func() { previous++ }

	tt := watchPage(w, 1280, 720)
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if backs != 1 || previous != 0 {
		t.Fatalf("back=%d previous=%d, want only the route action", backs, previous)
	}
}

// States that need the controls pin them; so do the drawer, the menu, focus
// and the floating player.
func TestWatchChromePinned(t *testing.T) {
	tests := []struct {
		name  string
		state WatchState
		setup func(*Watch)
		want  []string
	}{
		{
			name:  "paused",
			state: WatchState{Kind: WatchPaused, Channel: "Metro TV", Live: true},
			want:  []string{"Paused", "Resume", "Pause"},
		},
		{
			name:  "buffering",
			state: WatchState{Kind: WatchBuffering, Channel: "Metro TV", Live: true},
			want:  []string{"Buffering"},
		},
		{
			name:  "error",
			state: WatchState{Kind: WatchError, Channel: "Metro TV", Error: "Failed."},
			want:  []string{"Stream unavailable", "Retry"},
		},
		{
			name:  "recovery",
			state: WatchState{Kind: WatchBuffering, Channel: "Metro TV", Live: true, Recovery: WatchRecovery{Attempt: 2, MaxAttempts: 3, Pending: true, RetryIn: 8 * time.Second}},
			want:  []string{"Attempt 2 of 3 · retrying in 8s"},
		},
		{
			name:  "drawer",
			state: WatchState{Kind: WatchPlaying, Channel: "Metro TV", Channels: watchChannels(3)},
			setup: func(w *Watch) { w.ToggleDrawer() },
			want:  []string{"CHANNEL"},
		},
		{
			name:  "quality",
			state: WatchState{Kind: WatchPlaying, Channel: "Metro TV", Qualities: []WatchQualityOption{{Label: "auto", Selected: true}}},
			setup: func(w *Watch) { w.qualityOpen = true },
			want:  []string{"QUALITY"},
		},
		{
			name:  "pip",
			state: WatchState{Kind: WatchPlaying, Channel: "Metro TV", PiPActive: true},
			want:  []string{"Playing in Picture-in-Picture"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWatch()
			w.SetState(tc.state)
			if tc.setup != nil {
				tc.setup(w)
			}
			tt := watchPage(w, 1280, 720)
			tt.Advance(5 * time.Second)
			if !w.ChromeVisible() {
				t.Fatal("chrome hid in a pinned state")
			}
			for _, want := range tc.want {
				if !tt.HasText(want) {
					t.Fatalf("texts %q, want %q", tt.Texts(), want)
				}
			}
		})
	}
}

// Focus inside the chrome keeps it up, so a keyboard viewer is never left
// with hidden controls.
func TestWatchFocusKeepsChrome(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})

	tt := watchPage(w, 1280, 720)
	tt.Key(0, ui.KeyTab)
	tt.Advance(3 * time.Second)
	if !w.ChromeVisible() {
		t.Fatal("chrome hid while a control had focus")
	}
}

// A paused session keeps the frozen frame, dims it and explains resume.
func TestWatchPaused(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPaused, Channel: "Metro TV", Live: true, Paused: true})
	resumed := 0
	w.OnTogglePause = func() { resumed++ }

	tt := watchPage(w, 1470, 956)
	for _, want := range []string{
		"PAUSED", "Paused", "Frame frozen · audio stopped.",
		"Resume reconnects to live.", "Resume",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if live := tt.HasText("Live stream, playing at the live edge."); live {
		t.Fatalf("texts %q still claim the live edge", tt.Texts())
	}

	if err := tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	if resumed != 1 {
		t.Fatalf("resume clicked %d times, want 1", resumed)
	}
}

// Buffering keeps the last frame and reports the real attempt.
func TestWatchBuffering(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchBuffering, Channel: "Metro TV", Live: true,
		Recovery: WatchRecovery{Attempt: 2, MaxAttempts: 3, Pending: true, RetryIn: 8 * time.Second},
	})

	var frame Rect
	w.OnFrame = func(f Rect) { frame = f }
	tt := watchPage(w, 1470, 956)
	tt.Frame()

	for _, want := range []string{
		"Buffering", "Signal dropped. Reconnecting to live.",
		"Attempt 2 of 3 · retrying in 8s",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if frame.W <= 0 || frame.H <= 0 {
		t.Fatalf("frame %+v, want the picture box to stay", frame)
	}
}

// A terminal recovery never says it is retrying and offers a real retry.
func TestWatchRecoveryTerminal(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchBuffering, Channel: "Metro TV", Live: true,
		Recovery: WatchRecovery{Attempt: 3, MaxAttempts: 3, Terminal: true},
	})
	retries := 0
	w.OnRetry = func() { retries++ }

	tt := watchPage(w, 1280, 720)
	for _, text := range tt.Texts() {
		if strings.Contains(strings.ToLower(text), "retrying") {
			t.Fatalf("texts %q claim a retry after the terminal attempt", tt.Texts())
		}
	}
	if !tt.HasText("Signal lost") || !tt.HasText("Attempt 3 of 3 · retries stopped") {
		t.Fatalf("texts %q, want the terminal context", tt.Texts())
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if retries != 1 {
		t.Fatalf("retry clicked %d times, want 1", retries)
	}
}

// The connecting screen is the whole window, with the real clock and its own
// actions.
func TestWatchConnecting(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchLoading, Channel: "TVRI Bali", Source: "Community",
		Quality: WatchQuality{Selected: "auto", Current: "480p", Known: true},
	})
	cancelled, browsed, retried, opened := 0, 0, 0, 0
	w.OnCancel = func() { cancelled++ }
	w.OnBrowse = func() { browsed++ }
	w.OnRetry = func() { retried++ }
	w.OnOpenControlCenter = func() { opened++ }

	tt := watchPage(w, 1470, 956)
	clock := tt.Now().Format("15:04")
	for _, want := range []string{
		"Mumu TV", clock, "TVRI Bali", "Community · 480p",
		"Connecting to stream…", "Cancel", "Other channels", "Try again",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if !w.IsInitialLoading() {
		t.Fatal("a first connect is not reported as initial loading")
	}

	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if cancelled != 1 || browsed != 0 || retried != 0 {
		t.Fatalf("cancel=%d browse=%d retry=%d, want only cancel", cancelled, browsed, retried)
	}

	if err := tt.Click("Other channels"); err != nil {
		t.Fatal(err)
	}
	if !w.DrawerOpen() {
		t.Fatal("Other channels did not open the drawer")
	}
}

// After a channel has played, a new connect is not the first one and offers
// no cancel that would stop the session.
func TestWatchLoadingAfterPlayback(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV"})
	tt := watchPage(w, 1280, 720)
	tt.Frame()

	w.SetState(WatchState{Kind: WatchLoading, Channel: "ANTV"})
	tt.Frame()
	if w.IsInitialLoading() {
		t.Fatal("a switch is reported as initial loading")
	}
	if tt.HasText("Cancel") {
		t.Fatalf("texts %q, want no cancel after playback", tt.Texts())
	}
	if !tt.HasText("Connecting to stream…") {
		t.Fatalf("texts %q, want the connecting screen", tt.Texts())
	}
}

// A terminal failure shows the error, both actions and the real attempt.
func TestWatchTerminalError(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind:     WatchError,
		Channel:  "ANTV",
		Previous: "Metro TV",
		Error:    "The stream did not start in time.",
		Recovery: WatchRecovery{Attempt: 3, MaxAttempts: 3, Terminal: true},
	})
	retried, backed, previous, route := 0, 0, 0, 0
	w.OnRetry = func() { retried++ }
	w.OnBrowse = func() { backed++ }
	w.OnPreviousChannel = func() { previous++ }
	w.OnBack = func() { route++ }

	tt := watchPage(w, 1470, 956)
	for _, want := range []string{
		"Stream unavailable", "The stream did not start in time.",
		"Retry", "Pick another channel", "Back to Metro TV",
		"Attempt 3 of 3 · retries stopped", "OFFLINE",
	} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	for _, text := range tt.Texts() {
		if strings.Contains(strings.ToLower(text), "retrying") {
			t.Fatalf("texts %q claim a retry after the terminal attempt", tt.Texts())
		}
	}

	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if retried != 1 {
		t.Fatalf("retry clicked %d times, want 1", retried)
	}
	if err := tt.Click("Back to Metro TV"); err != nil {
		t.Fatal(err)
	}
	if previous != 1 || route != 0 {
		t.Fatalf("previous=%d route=%d, want only the previous-channel action", previous, route)
	}
	if err := tt.Click("Pick another channel"); err != nil {
		t.Fatal(err)
	}
	if !w.DrawerOpen() && backed != 1 {
		t.Fatal("Pick another channel neither opened the drawer nor Discover")
	}
}

// Without an earlier channel the error keeps a single action.
func TestWatchErrorWithoutPrevious(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchError, Channel: "ANTV", Error: "Failed."})

	tt := watchPage(w, 1024, 640)
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "Back to ") {
			t.Fatalf("texts %q, want no return action", tt.Texts())
		}
	}
	if !tt.HasText("Retry") {
		t.Fatalf("texts %q, want Retry", tt.Texts())
	}
}

// An idle page offers the shared empty state and Discover.
func TestWatchEmpty(t *testing.T) {
	for _, size := range watchSizes {
		t.Run(size.name, func(t *testing.T) {
			w := NewWatch()
			browsed := 0
			w.OnBrowse = func() { browsed++ }

			tt := watchPage(w, size.width, size.height)
			if !tt.HasText("Nothing playing") {
				t.Fatalf("texts %q, want the empty state", tt.Texts())
			}
			if err := tt.Click("Browse Discover"); err != nil {
				t.Fatal(err)
			}
			if browsed != 1 {
				t.Fatalf("browse clicked %d times, want 1", browsed)
			}
		})
	}
}

// A resume card names the last channel and plays only when chosen.
func TestWatchResumeNeedsClick(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchResume, Channel: "TVRI Nasional", Source: "Community list"})
	resumed := 0
	w.OnResume = func() { resumed++ }

	tt := watchPage(w, 1280, 720)
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

// The drawer batches twenty tiles and then more on request.
func TestWatchDrawerInfiniteScroll(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Channel 00", Channels: watchChannels(45),
	})
	tt := watchPage(w, 1470, 956)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !w.DrawerOpen() {
		t.Fatal("drawer did not open")
	}
	for _, want := range []string{"Channel 00", "Channel 19", "45 channels"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q in the first batch", tt.Texts(), want)
		}
	}
	if tt.HasText("Channel 20") {
		t.Fatalf("texts %q, want the first batch capped at 20", tt.Texts())
	}
	if hasShowRow(tt.Texts()) {
		t.Fatalf("texts %q, want no Show button or batch row", tt.Texts())
	}

	// Reaching the end lays out the next batch without a control.
	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 39") || tt.HasText("Channel 40") {
		t.Fatalf("texts %q, want exactly the next 20 rows", tt.Texts())
	}
	if hasShowRow(tt.Texts()) {
		t.Fatalf("texts %q, want no Show button after loading", tt.Texts())
	}

	// The last batch stops at the final row.
	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 44") || tt.HasText("Channel 45") {
		t.Fatalf("texts %q, want the catalog to end at its last row", tt.Texts())
	}

	// Scrolling past the end asks for nothing more.
	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 44") || w.drawerLimit != 45 {
		t.Fatalf("limit = %d, want the catalog to stay at 45 rows", w.drawerLimit)
	}
}

// Reopening the drawer starts again from the first batch.
func TestWatchDrawerInfiniteScrollResets(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Channel 00", Channels: watchChannels(45),
	})
	tt := watchPage(w, 1470, 956)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 39") {
		t.Fatalf("texts %q, want the second batch before closing", tt.Texts())
	}

	w.CloseDrawer()
	tt.Frame()
	if w.drawerLimit != watchDrawerBatch {
		t.Fatalf("limit = %d after closing, want %d", w.drawerLimit, watchDrawerBatch)
	}

	w.ToggleDrawer()
	tt.Frame()
	if w.drawerLimit != watchDrawerBatch || w.drawerScroll.Y != 0 {
		t.Fatalf("limit = %d y = %v after reopening, want the first batch at the top",
			w.drawerLimit, w.drawerScroll.Y)
	}
	if tt.HasText("Channel 20") {
		t.Fatalf("texts %q, want only the first batch after reopening", tt.Texts())
	}
}

// hasShowRow reports the removed paging controls in the drawer text.
func hasShowRow(texts []string) bool {
	for _, text := range texts {
		if strings.HasPrefix(text, "Show ") || strings.HasPrefix(text, "Showing ") {
			return true
		}
	}

	return false
}

// Search filters the drawer with the virtualized list and watches a row.
func TestWatchDrawerSearch(t *testing.T) {
	w := NewWatch()
	channels := watchChannels(30)
	channels[1].Name = "Beta News"
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Channel 00", Channels: channels})
	selected := ""
	w.OnSelectChannel = func(key string) { selected = key }

	tt := watchPage(w, 1470, 956)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if err := tt.Click("Search channels"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Beta")
	tt.Frame()

	if !tt.HasText("1 match") {
		t.Fatalf("texts %q, want the real match count", tt.Texts())
	}
	// The result is a drawer tile, not a row in a virtualized list.
	if !tt.HasText("Beta News, News · Community") {
		t.Fatalf("texts %q, want the searched drawer tile", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleList, "Channels"); ok {
		t.Fatalf("texts %q, want no row list in the drawer", tt.Texts())
	}

	// Leaving the field puts the filtered grid under the arrow keys and
	// Enter watches the focused tile.
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if selected != "key-1" {
		t.Fatalf("selected %q, want the searched tile", selected)
	}
}

// Search pages the filtered tiles with the same infinite scroll.
func TestWatchDrawerSearchInfiniteScroll(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Channel 00", Live: true,
		Channels: watchChannels(60),
	})
	tt := watchPage(w, 1470, 956)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Search channels"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Channel")
	tt.Frame()

	if !tt.HasText("60 matches") {
		t.Fatalf("texts %q, want the real match count", tt.Texts())
	}
	if !tt.HasText("Channel 19") || tt.HasText("Channel 20") {
		t.Fatalf("texts %q, want the filtered first batch of 20", tt.Texts())
	}
	if hasShowRow(tt.Texts()) {
		t.Fatalf("texts %q, want no Show button in search", tt.Texts())
	}

	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 39") || tt.HasText("Channel 40") {
		t.Fatalf("texts %q, want the next filtered batch", tt.Texts())
	}

	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 59") || tt.HasText("Channel 60") {
		t.Fatalf("texts %q, want the filtered catalog to end", tt.Texts())
	}
	if w.drawerLimit != 60 {
		t.Fatalf("limit = %d, want all 60 filtered rows", w.drawerLimit)
	}
}

// A new query starts the filtered grid again from its first batch.
func TestWatchDrawerSearchResetsBatch(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Channel 00", Live: true,
		Channels: watchChannels(60),
	})
	tt := watchPage(w, 1470, 956)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Search channels"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Channel")
	tt.Frame()
	tt.Scroll(1200, 600, 0, 100000)
	tt.Frame()
	if !tt.HasText("Channel 39") {
		t.Fatalf("texts %q, want the second filtered batch before the query changes", tt.Texts())
	}

	// Still 60 matches, but the page starts over at 20.
	tt.Type(" ")
	tt.Frame()
	if w.drawerLimit != watchDrawerBatch {
		t.Fatalf("limit = %d after the query changed, want %d", w.drawerLimit, watchDrawerBatch)
	}
	if !tt.HasText("Channel 19") || tt.HasText("Channel 20") {
		t.Fatalf("texts %q, want the first filtered batch after the query changed", tt.Texts())
	}
}

// The empty search result says so instead of showing stale rows.
func TestWatchDrawerSearchEmpty(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Channel 00", Channels: watchChannels(4)})
	tt := watchPage(w, 1024, 640)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Search channels"); err != nil {
		t.Fatal(err)
	}
	tt.Type("zzz")
	tt.Frame()
	if !tt.HasText(`No channels match "zzz".`) {
		t.Fatalf("texts %q, want the empty result copy", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleList, "Channels"); ok {
		t.Fatalf("texts %q, want no row list for an empty search", tt.Texts())
	}
}

// Arrow keys move the drawer cursor and Enter watches the focused tile.
func TestWatchDrawerKeyboard(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Channel 00", Channels: watchChannels(8),
	})
	selected := ""
	w.OnSelectChannel = func(key string) { selected = key }

	tt := watchPage(w, 1470, 956)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.drawerFocus != drawerFocusValue("key-0") {
		t.Fatalf("drawer focus %q, want the current channel", w.drawerFocus)
	}

	tt.Key(0, ui.KeyDown)
	if w.drawerFocus != drawerFocusValue("key-2") {
		t.Fatalf("focus after Down = %q, want two tiles on", w.drawerFocus)
	}
	tt.Key(0, ui.KeyRight)
	if w.drawerFocus != drawerFocusValue("key-3") {
		t.Fatalf("focus after Right = %q, want the next tile", w.drawerFocus)
	}
	tt.Key(0, ui.KeyEnter)
	if selected != "key-3" {
		t.Fatalf("selected %q, want the focused tile", selected)
	}
}

// While the drawer is open, the arrow keys belong to it and not to volume.
func TestWatchDrawerKeysDoNotChangeVolume(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Channel 00", Volume: 40, Channels: watchChannels(4),
	})
	volume := 0
	w.OnVolume = func(v int) { volume = v }

	tt := watchPage(w, 1280, 720)
	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	if volume != 0 {
		t.Fatalf("volume changed to %d while the drawer was open", volume)
	}
}

// Opening the drawer reflows the chrome into the remaining width; the video
// session and the page identity are untouched.
func TestWatchDrawerReflowsChrome(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{
		Kind: WatchPlaying, Channel: "Metro TV", Live: true, Channels: watchChannels(4),
	})
	tt := watchPage(w, 1470, 956)
	before, ok := tt.Find("Fullscreen")
	if !ok {
		t.Fatalf("texts %q, want the fullscreen control", tt.Texts())
	}

	if err := tt.Click("Channels"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	after, ok := tt.Find("Fullscreen")
	if !ok {
		t.Fatalf("texts %q, want the fullscreen control in the reflowed row", tt.Texts())
	}
	if after.X >= before.X {
		t.Fatalf("fullscreen X = %v after opening, want left of %v", after.X, before.X)
	}
	if after.X+after.W > 1470-watchDrawerWidth {
		t.Fatalf("fullscreen right edge = %v, want the chrome to stop before the drawer",
			after.X+after.W)
	}
	if got := w.State().Channel; got != "Metro TV" {
		t.Fatalf("channel %q changed while opening the drawer", got)
	}

	w.CloseDrawer()
	tt.Frame()
	restored, ok := tt.Find("Fullscreen")
	if !ok || restored.X != before.X {
		t.Fatalf("fullscreen X = %v after closing, want %v", restored.X, before.X)
	}
}

// The page reports the full-bleed video box so the app can place the surface.
func TestWatchReportsVideoFrame(t *testing.T) {
	w := NewWatch()
	var frames []Rect
	w.OnFrame = func(frame Rect) { frames = append(frames, frame) }

	tt := watchPage(w, 1280, 720)
	tt.Frame()

	if len(frames) == 0 {
		t.Fatal("no video frame reported")
	}
	if frames[0] != (Rect{W: 1280, H: 720}) {
		t.Fatalf("frame %+v, want the whole window", frames[0])
	}
}

// ResetFrame makes a rebuilt window report its video box again.
func TestWatchResetFrame(t *testing.T) {
	w := NewWatch()
	reports := 0
	w.OnFrame = func(Rect) { reports++ }

	tt := watchPage(w, 1280, 720)
	tt.Frame()
	first := reports
	if first == 0 {
		t.Fatal("no frame reported")
	}

	w.SetState(WatchState{Kind: WatchLoading, Channel: "Metro TV"})
	tt.Frame()

	w.ResetFrame()
	tt.Frame()
	if reports <= first {
		t.Fatalf("frames reported %d, want more than %d after ResetFrame", reports, first)
	}
}

// The floating player's page keeps one session and offers its return.
func TestWatchPiPPlaceholder(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV", PiPActive: true})
	returned := 0
	w.OnReturn = func() { returned++ }

	tt := watchPage(w, 1280, 720)
	for _, want := range []string{"Playing in Picture-in-Picture", "Return to window"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}
	if err := tt.Click("Return to window"); err != nil {
		t.Fatal(err)
	}
	if returned != 1 {
		t.Fatalf("return clicked %d times, want 1", returned)
	}
}

// Every state fits the three reference sizes without leaving the window.
func TestWatchStatesAtReferenceSizes(t *testing.T) {
	states := []struct {
		name  string
		state WatchState
		want  string
	}{
		{name: "empty", state: WatchState{Kind: WatchEmpty}, want: "Nothing playing"},
		{name: "resume", state: WatchState{Kind: WatchResume, Channel: "Metro TV"}, want: "Resume Metro TV"},
		{name: "playing", state: WatchState{Kind: WatchPlaying, Channel: "Metro TV", Live: true}, want: "Metro TV · Live broadcast"},
		{name: "paused", state: WatchState{Kind: WatchPaused, Channel: "Metro TV", Live: true}, want: "Frame frozen · audio stopped."},
		{name: "buffering", state: WatchState{Kind: WatchBuffering, Channel: "Metro TV", Live: true}, want: "Signal dropped. Reconnecting to live."},
		{name: "error", state: WatchState{Kind: WatchError, Channel: "Metro TV", Error: "Failed."}, want: "Stream unavailable"},
		{name: "loading", state: WatchState{Kind: WatchLoading, Channel: "Metro TV"}, want: "Connecting to stream…"},
		{name: "pip", state: WatchState{Kind: WatchPlaying, Channel: "Metro TV", PiPActive: true}, want: "Playing in Picture-in-Picture"},
	}

	for _, tc := range states {
		for _, size := range watchSizes {
			t.Run(tc.name+"/"+size.name, func(t *testing.T) {
				w := NewWatch()
				w.SetState(tc.state)
				tt := watchPage(w, size.width, size.height)

				box, ok := tt.Find(tc.want)
				if !ok {
					t.Fatalf("texts %q, want %q", tt.Texts(), tc.want)
				}
				if box.X < 0 || box.Y < 0 ||
					box.X+box.W > float32(size.width) || box.Y+box.H > float32(size.height) {
					t.Fatalf("%q at %+v, want it inside %dx%d", tc.want, box, size.width, size.height)
				}
			})
		}
	}
}

// The chrome keeps its controls inside compact windows, where the bottom
// scrim is shorter.
func TestWatchCompactChrome(t *testing.T) {
	w := NewWatch()
	w.SetState(WatchState{Kind: WatchPlaying, Channel: "Metro TV", Live: true})
	tt := watchPage(w, 1024, 640)

	for _, label := range []string{"Back", "Pause", "Volume", "Fullscreen", "Control Center", "Settings"} {
		box, ok := tt.Find(label)
		if !ok {
			t.Fatalf("texts %q, want %q", tt.Texts(), label)
		}
		if box.X < 0 || box.Y < 0 || box.X+box.W > 1024 || box.Y+box.H > 640 {
			t.Errorf("%q at %+v, want it inside 1024x640", label, box)
		}
	}
}
