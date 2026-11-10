//go:build darwin && arm64

package app

import (
	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

// newPlaybackEngine builds the real libmpv engine, or reports why it cannot.
// The runtime is opened here, on the first Play, never at launch.
func newPlaybackEngine(videoOutput string) (player.Engine, error) {
	return libmpv.New(libmpv.Config{VideoOutput: videoOutput})
}
