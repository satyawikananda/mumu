//go:build darwin && arm64

package main

import "testing"

// TestTransparentRoot keeps the comparison mode honest: only the baseline may
// paint an opaque root, because every other mode shows the retained view under
// MyGo's surface.
func TestTransparentRoot(t *testing.T) {
	cases := map[mode]bool{
		modeBelow:      true,
		modeNativeView: true,
		modeAbove:      false,
	}
	for m, want := range cases {
		if got := m.transparentRoot(); got != want {
			t.Errorf("mode %q transparentRoot = %v, want %v", m, got, want)
		}
	}
}
