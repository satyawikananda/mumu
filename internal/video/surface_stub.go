//go:build !(darwin && arm64)

package video

import "mumu/internal/player"

// surfaceSupported reports that this build has no native video adapter. Only
// darwin/arm64 carries the M0-proven NSOpenGLView bridge.
const surfaceSupported = false

// newSurface reports that this build has no native video adapter. Callers get
// ErrUnsupported from New, so no Surface exists here and Attach, Detach,
// SetFrame and Size are unreachable on this platform. The renderer stays usable
// as a headless engine, as it does when the darwin adapter cannot be created.
func newSurface(player.Renderer) (Surface, error) {
	return nil, ErrUnsupported
}
