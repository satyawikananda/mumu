package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// pipPage wires the mini player into a headless tester.
func pipPage(p *PiP) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		p.View(c, c.Theme())
	}, 480, 318)
}

// The mini player names the channel and offers Return and Close.
func TestPiPControls(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "TVRI Nasional", Status: "Playing"})

	returned, closed, muted := 0, 0, 0
	p.OnReturn = func() { returned++ }
	p.OnClose = func() { closed++ }
	p.OnToggleMute = func() { muted++ }

	tt := pipPage(p)
	for _, want := range []string{"TVRI Nasional", "Playing", "Return to Window", "Close", "Mute"} {
		if !tt.HasText(want) {
			t.Fatalf("texts %q, want %q", tt.Texts(), want)
		}
	}

	if err := tt.Click("Return to Window"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatal(err)
	}
	if returned != 1 || closed != 1 || muted != 1 {
		t.Fatalf("return=%d close=%d mute=%d, want 1 each", returned, closed, muted)
	}
}

// Muted reflects the shared player and shows its opposite action.
func TestPiPMuted(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "A", Muted: true})

	tt := pipPage(p)
	if !tt.HasText("Unmute") {
		t.Fatalf("texts %q, want Unmute", tt.Texts())
	}
}

// A playback error is visible in the compact strip.
func TestPiPError(t *testing.T) {
	p := NewPiP()
	p.SetState(PiPState{Channel: "A", Error: "The stream stopped."})

	tt := pipPage(p)
	if !tt.HasText("The stream stopped.") {
		t.Fatalf("texts %q, want the error", tt.Texts())
	}
}
