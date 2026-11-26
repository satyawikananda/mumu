//go:build !((darwin && arm64) || ((windows || linux) && amd64))

// Package libmpv provides either the real libmpv adapter or a stub that
// reports it is unavailable. The real adapter supports darwin/arm64,
// windows/amd64 and linux/amd64; other targets keep this stub. No build
// requires a native runtime at link time.
package libmpv

import (
	"context"
	"fmt"

	"mumu/internal/player"
)

// ErrUnavailable reports a build without the real libmpv adapter.
var ErrUnavailable = fmt.Errorf("libmpv adapter is unavailable on this platform: %w", player.ErrRuntimeUnavailable)

// Engine is unavailable in this build.
type Engine struct{}

var _ player.Engine = (*Engine)(nil)

// New reports that the adapter is not compiled into this binary.
func New(Config) (*Engine, error) {
	return nil, ErrUnavailable
}

// Load always reports ErrUnavailable.
func (e *Engine) Load(context.Context, uint64, player.Source) error { return ErrUnavailable }

// Stop always reports ErrUnavailable.
func (e *Engine) Stop(context.Context) error { return ErrUnavailable }

// SetVolume always reports ErrUnavailable.
func (e *Engine) SetVolume(context.Context, int) error { return ErrUnavailable }

// SetMuted always reports ErrUnavailable.
func (e *Engine) SetMuted(context.Context, bool) error { return ErrUnavailable }

// SetPaused reports that this build has no libmpv runtime.
func (e *Engine) SetPaused(context.Context, bool) error { return ErrUnavailable }

// SetReadAhead reports that this build has no libmpv runtime.
func (e *Engine) SetReadAhead(context.Context, int) error { return ErrUnavailable }

// Events returns a closed stream.
func (e *Engine) Events() <-chan player.Event {
	events := make(chan player.Event)
	close(events)

	return events
}

// Close is a no-op for the stub.
func (e *Engine) Close(context.Context) error { return nil }
