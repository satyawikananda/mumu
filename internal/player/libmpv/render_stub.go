//go:build !(darwin && arm64)

package libmpv

import (
	"unsafe"

	"mumu/internal/player"
)

var _ player.Renderer = (*Engine)(nil)

// NewRenderContext always reports ErrRenderDisabled: this build has no native
// renderer. The stub keeps callers that accept a player.Renderer compiling on
// every platform.
func (e *Engine) NewRenderContext(func(string) unsafe.Pointer) (player.FrameRenderer, error) {
	return nil, player.ErrRenderDisabled
}
