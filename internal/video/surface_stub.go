//go:build !(darwin && arm64)

package video

import "mumu/internal/player"

// surfaceSupported reports that this build has no native video adapter. Only
// darwin/arm64 carries the M0-proven NSOpenGLView bridge.
const surfaceSupported = false

// newSurface reports that this build has no native video adapter.
func newSurface(player.Renderer) (Surface, error) {
	return nil, ErrUnsupported
}
