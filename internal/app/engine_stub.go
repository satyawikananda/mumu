//go:build !(darwin && arm64)

package app

import (
	"fmt"

	"mumu/internal/player"
)

// ErrPlaybackUnavailable reports a build without the native engine.
var ErrPlaybackUnavailable = fmt.Errorf("native playback is unavailable on this platform: %w", player.ErrRuntimeUnavailable)

// newPlaybackEngine reports that this build has no libmpv adapter.
func newPlaybackEngine(string, int) (player.Engine, error) {
	return nil, ErrPlaybackUnavailable
}
