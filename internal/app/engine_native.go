//go:build (darwin && arm64) || ((windows || linux) && amd64)

package app

import (
	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

// newPlaybackEngine builds the real libmpv engine, or reports why it cannot.
// The runtime is opened here, on the first Play, never at launch. The volume is
// the effective launch volume: libmpv's own zero would otherwise play silently
// while the page shows a different number.
func newPlaybackEngine(videoOutput string, volume int) (player.Engine, error) {
	return libmpv.New(libmpv.Config{
		VideoOutput:       videoOutput,
		Volume:            volume,
		Muted:             false,
		BlockPrivateMedia: true,
	})
}
