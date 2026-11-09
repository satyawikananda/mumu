//go:build !(darwin && arm64 && mumu_mpv)

// Package libmpv provides either the real libmpv adapter or a stub that
// reports it is unavailable. The real adapter requires the mumu_mpv tag.
package libmpv

import (
	"context"
	"errors"

	"mumu/internal/player"
)

// ErrUnavailable reports a build without the real libmpv adapter.
var ErrUnavailable = errors.New("libmpv adapter requires the mumu_mpv build tag on darwin/arm64")

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

// Events returns a closed stream.
func (e *Engine) Events() <-chan player.Event {
	events := make(chan player.Event)
	close(events)

	return events
}

// Close is a no-op for the stub.
func (e *Engine) Close(context.Context) error { return nil }
