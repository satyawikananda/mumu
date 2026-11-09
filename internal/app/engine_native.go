//go:build darwin && arm64 && mumu_mpv

package app

import (
	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

// newPlaybackEngine builds the real libmpv engine, or reports why it cannot.
func newPlaybackEngine(videoOutput string) (player.Engine, error) {
	return libmpv.New(libmpv.Config{VideoOutput: videoOutput})
}
