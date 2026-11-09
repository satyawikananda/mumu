package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStatusStates(t *testing.T) {
	tests := []struct {
		name  string
		state Status
	}{
		{"empty", Status{Kind: StatusEmpty, Title: "Nothing here", Message: "Empty message"}},
		{"loading", Status{Kind: StatusLoading, Title: "Loading channels", Message: "Please wait"}},
		{"error", Status{Kind: StatusError, Title: "Playback failed", Message: "Try again"}},
		{"offline", Status{Kind: StatusOffline, Title: "You are offline", Message: "Check the network"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := ui.NewTester(tc.state.View, 640, 400)
			if !tt.HasText(tc.state.Title) || !tt.HasText(tc.state.Message) {
				t.Fatalf("texts %q, want the title and message", tt.Texts())
			}
		})
	}
}

func TestStatusAction(t *testing.T) {
	clicked := 0
	status := Status{
		Kind:     StatusEmpty,
		Title:    "No channels",
		Action:   "Retry",
		OnAction: func() { clicked++ },
	}

	tt := ui.NewTester(status.View, 640, 400)
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if clicked != 1 {
		t.Fatalf("action clicked %d times, want 1", clicked)
	}
}
