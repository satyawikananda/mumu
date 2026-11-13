package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// pipPage wires the mini player into a headless tester at one size.
func pipPage(p *PiP, width, height int) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		p.View(c, c.Theme())
	}, width, height)
}

// pipSizes are the approved window sizes.
var pipSizes = []struct {
	name   string
	width  int
	height int
}{
	{name: "default", width: 640, height: 360},
	{name: "compact", width: 480, height: 270},
	{name: "mini", width: 360, height: 203},
}

// Every approved size shows the floating chrome and dispatches its actions.
func TestPiPChrome(t *testing.T) {
	for _, size := range pipSizes {
		t.Run(size.name, func(t *testing.T) {
			p := NewPiP()
			p.SetState(PiPState{Channel: "TVRI Nasional", Status: "Playing", Live: true})

			returned, closed, muted, paused := 0, 0, 0, 0
			p.OnReturn = func() { returned++ }
			p.OnClose = func() { closed++ }
			p.OnToggleMute = func() { muted++ }
			p.OnTogglePause = func() { paused++ }

			tt := pipPage(p, size.width, size.height)
			for _, want := range []string{
				"TVRI Nasional", "LIVE", "Auto", "Pause", "Mute",
				"Return to window", "Close Picture in Picture",
			} {
				if !tt.HasText(want) {
					t.Fatalf("texts %q, want %q", tt.Texts(), want)
				}
			}

			for _, label := range []string{"Pause", "Mute", "Return to window", "Close Picture in Picture"} {
				if err := tt.Click(label); err != nil {
					t.Fatalf("click %s: %v", label, err)
				}
			}
			if returned != 1 || closed != 1 || muted != 1 || paused != 1 {
				t.Fatalf("return=%d close=%d mute=%d pause=%d, want one each",
					returned, closed, muted, paused)
			}
		})
	}
}

// Below 360 points the chrome collapses to the close target alone.
func TestPiPCloseOnly(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "TVRI Nasional", Status: "Playing", Live: true})
	closed := 0
	p.OnClose = func() { closed++ }

	tt := pipPage(p, 320, 180)
	if tt.HasText("TVRI Nasional") || tt.HasText("Return to window") || tt.HasText("Pause") {
		t.Fatalf("texts %q, want the close target alone", tt.Texts())
	}
	if !tt.HasText("Close Picture in Picture") {
		t.Fatalf("texts %q, want the close target", tt.Texts())
	}
	if !p.ChromeVisible() {
		t.Fatal("the close-only chrome reported hidden")
	}
	if err := tt.Click("Close Picture in Picture"); err != nil {
		t.Fatal(err)
	}
	if closed != 1 {
		t.Fatalf("close clicked %d times, want 1", closed)
	}
}

// Two idle seconds hide the floating chrome; a click or NoteActivity brings
// it back.
func TestPiPIdle(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "TVRI Nasional", Live: true})
	tt := pipPage(p, 640, 360)

	tt.Advance(2 * time.Second)
	if p.ChromeVisible() {
		t.Fatal("chrome stayed after two idle seconds")
	}
	if tt.HasText("TVRI Nasional") || tt.HasText("Return to window") {
		t.Fatalf("texts %q, want the picture alone", tt.Texts())
	}

	if changed := p.NoteActivity(tt.Now()); !changed {
		t.Fatal("NoteActivity reported no change for hidden chrome")
	}
	tt.Frame()
	if !p.ChromeVisible() || !tt.HasText("TVRI Nasional") {
		t.Fatal("NoteActivity did not bring the chrome back")
	}

	tt.Advance(2 * time.Second)
	if p.ChromeVisible() {
		t.Fatal("chrome did not hide again")
	}
	tt.ClickAt(320, 180)
	tt.Frame()
	if !p.ChromeVisible() {
		t.Fatal("a click on the picture did not reveal the chrome")
	}
}

// The floating window keeps the design rule that pointer movement shows its
// chrome.
func TestPiPPointerMoveReveals(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "TVRI Nasional", Live: true})
	tt := pipPage(p, 640, 360)
	tt.Advance(2 * time.Second)
	if p.ChromeVisible() {
		t.Fatal("chrome did not hide")
	}

	tt.Move(400, 200)
	tt.Frame()
	if !p.ChromeVisible() || !tt.HasText("Return to window") {
		t.Fatal("pointer movement did not show the floating chrome")
	}
}

// Paused, muted and terminal states keep the chrome up.
func TestPiPPinned(t *testing.T) {
	tests := []struct {
		name  string
		state PiPState
		want  []string
	}{
		{
			name:  "paused",
			state: PiPState{Channel: "TVRI", Live: true, Paused: true},
			want:  []string{"PAUSED", "Resume"},
		},
		{
			name:  "muted",
			state: PiPState{Channel: "TVRI", Live: true, Muted: true},
			want:  []string{"Muted", "Unmute"},
		},
		{
			name:  "terminal",
			state: PiPState{Channel: "TVRI", Live: true, Recovery: WatchRecovery{Attempt: 3, MaxAttempts: 3, Terminal: true}},
			want:  []string{"Signal lost", "Automatic retries stopped.", "Try again"},
		},
		{
			name:  "pending",
			state: PiPState{Channel: "TVRI", Live: true, Recovery: WatchRecovery{Attempt: 2, Pending: true, RetryIn: 8 * time.Second}},
			want:  []string{"Signal lost", "Retrying in 8s"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPiP()
			p.OnRetry = func() {}
			p.SetState(tc.state)
			tt := pipPage(p, 640, 360)
			tt.Advance(5 * time.Second)
			if !p.ChromeVisible() {
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

// The pause target resumes the shared session.
func TestPiPPausedResume(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "TVRI", Live: true, Paused: true})
	resumed := 0
	p.OnTogglePause = func() { resumed++ }

	tt := pipPage(p, 480, 270)
	if err := tt.Click("Resume"); err != nil {
		t.Fatal(err)
	}
	if resumed != 1 {
		t.Fatalf("resume clicked %d times, want 1", resumed)
	}
}

// A terminal failure offers the real retry and never a stale countdown.
func TestPiPTerminalRetry(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{
		Channel: "TVRI", Live: true,
		Recovery: WatchRecovery{Attempt: 3, MaxAttempts: 3, Terminal: true},
	})
	retried := 0
	p.OnRetry = func() { retried++ }

	tt := pipPage(p, 640, 360)
	if err := tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	if retried != 1 {
		t.Fatalf("retry clicked %d times, want 1", retried)
	}
}

// A playback error is named in the picture, not only in a strip.
func TestPiPError(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "TVRI", Error: "The stream stopped."})

	tt := pipPage(p, 640, 360)
	if !tt.HasText("Stream unavailable") || !tt.HasText("The stream stopped.") {
		t.Fatalf("texts %q, want the error", tt.Texts())
	}
}

// The floating window has no shell registry: its own keys work while the
// chrome is hidden and in every state.
func TestPiPShortcuts(t *testing.T) {
	t.Run("hidden chrome", func(t *testing.T) {
		p := NewPiP()
		p.SetState(PiPState{Channel: "TVRI", Live: true})
		closed, returned, paused, muted := 0, 0, 0, 0
		p.OnClose = func() { closed++ }
		p.OnReturn = func() { returned++ }
		p.OnTogglePause = func() { paused++ }
		p.OnToggleMute = func() { muted++ }

		tt := pipPage(p, 640, 360)
		tt.Advance(2 * time.Second)
		if p.ChromeVisible() {
			t.Fatal("chrome did not hide")
		}

		tt.Key(0, ui.KeyP)
		tt.Key(0, ui.KeySpace)
		tt.Key(0, ui.KeyM)
		tt.Key(0, ui.KeyEscape)
		if returned != 1 || paused != 1 || muted != 1 || closed != 1 {
			t.Fatalf("return=%d pause=%d mute=%d close=%d, want one each",
				returned, paused, muted, closed)
		}
	})

	t.Run("terminal recovery", func(t *testing.T) {
		p := NewPiP()
		p.SetState(PiPState{
			Channel: "TVRI", Live: true,
			Recovery: WatchRecovery{Attempt: 3, MaxAttempts: 3, Terminal: true},
		})
		closed, returned, retried, paused := 0, 0, 0, 0
		p.OnClose = func() { closed++ }
		p.OnReturn = func() { returned++ }
		p.OnRetry = func() { retried++ }
		p.OnTogglePause = func() { paused++ }

		tt := pipPage(p, 640, 360)
		tt.Advance(5 * time.Second)
		tt.Key(0, ui.KeyEscape)
		tt.Key(0, ui.KeyP)
		tt.Key(0, ui.KeyR)
		tt.Key(0, ui.KeySpace)
		if closed != 1 || returned != 1 || retried != 1 || paused != 1 {
			t.Fatalf("close=%d return=%d retry=%d pause=%d, want one each",
				closed, returned, retried, paused)
		}
	})

	t.Run("retry only when terminal", func(t *testing.T) {
		p := NewPiP()
		p.SetState(PiPState{Channel: "TVRI", Live: true})
		retried := 0
		p.OnRetry = func() { retried++ }

		tt := pipPage(p, 480, 270)
		tt.Key(0, ui.KeyR)
		if retried != 0 {
			t.Fatal("R retried a healthy stream")
		}

		p.SetState(PiPState{
			Channel: "TVRI", Live: true,
			Recovery: WatchRecovery{Attempt: 3, Terminal: true},
		})
		tt.Frame()
		tt.Key(0, ui.KeyR)
		if retried != 1 {
			t.Fatalf("R retried %d times, want 1 after a terminal failure", retried)
		}
	})

	t.Run("paused and muted", func(t *testing.T) {
		p := NewPiP()
		p.SetState(PiPState{Channel: "TVRI", Live: true, Paused: true, Muted: true})
		returned := 0
		p.OnReturn = func() { returned++ }

		tt := pipPage(p, 360, 203)
		tt.Key(0, ui.KeyP)
		if returned != 1 {
			t.Fatalf("P returned %d times, want 1 while paused", returned)
		}
	})
}

// Every state fits the three approved sizes.
func TestPiPStatesAtSizes(t *testing.T) {
	states := []struct {
		name  string
		state PiPState
		want  string
	}{
		{name: "playing", state: PiPState{Channel: "TVRI Nasional", Live: true}, want: "TVRI Nasional"},
		{name: "paused", state: PiPState{Channel: "TVRI Nasional", Live: true, Paused: true}, want: "PAUSED"},
		{name: "muted", state: PiPState{Channel: "TVRI Nasional", Live: true, Muted: true}, want: "Muted"},
		{name: "recovery", state: PiPState{Channel: "TVRI Nasional", Recovery: WatchRecovery{Pending: true, RetryIn: 8 * time.Second}}, want: "Signal lost"},
		{name: "error", state: PiPState{Channel: "TVRI Nasional", Error: "Failed."}, want: "Stream unavailable"},
	}

	for _, tc := range states {
		for _, size := range pipSizes {
			t.Run(tc.name+"/"+size.name, func(t *testing.T) {
				p := NewPiP()
				p.SetState(tc.state)
				tt := pipPage(p, size.width, size.height)

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
