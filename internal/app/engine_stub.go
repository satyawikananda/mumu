//go:build !(darwin && arm64 && mumu_mpv)

package app

import (
	"errors"

	"mumu/internal/player"
)

// ErrPlaybackUnavailable reports a build without the native engine.
var ErrPlaybackUnavailable = errors.New("native playback requires the mumu_mpv build tag on darwin/arm64")

// newPlaybackEngine reports that this build has no libmpv adapter.
func newPlaybackEngine(string) (player.Engine, error) {
	return nil, ErrPlaybackUnavailable
}
