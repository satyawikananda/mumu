package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mumu/internal/player"
	"mumu/internal/player/libmpv"
	"mumu/internal/video"
)

// session is one libmpv client and one retained native video view.
//
// There is exactly one of these per spike run, whichever window shows it. A
// window transfer moves the view; it never creates a second engine, decoder or
// stream load.
type session struct {
	engine  *libmpv.Engine
	surface video.Surface
	gen     uint64
	volume  int
}

// newSession opens the libmpv runtime and creates the native view. It must run
// on the main thread because video.New touches AppKit.
func newSession(volume int) (*session, error) {
	engine, err := libmpv.New(libmpv.Config{VideoOutput: "libmpv", Volume: volume})
	if err != nil {
		return nil, fmt.Errorf("open libmpv: %w", err)
	}

	renderer, ok := any(engine).(player.Renderer)
	if !ok {
		_ = engine.Close(context.Background())

		return nil, errors.New("engine does not implement player.Renderer")
	}

	surface, err := video.New(renderer)
	if err != nil {
		_ = engine.Close(context.Background())

		return nil, fmt.Errorf("create native surface: %w", err)
	}

	return &session{engine: engine, surface: surface, volume: volume}, nil
}

// load starts stream in the same session and returns the generation that tags
// its events.
func (s *session) load(ctx context.Context, stream string) (uint64, error) {
	s.gen++
	source := player.Source{URL: stream, Scheme: schemeOf(stream)}

	return s.gen, s.engine.Load(ctx, s.gen, source)
}

// setPaused holds or resumes the same source.
func (s *session) setPaused(ctx context.Context, paused bool) error {
	return s.engine.SetPaused(ctx, paused)
}

// events forwards the engine's event stream.
func (s *session) events() <-chan player.Event { return s.engine.Events() }

// syncDrawable refreshes the drawable from the hosted view's bounds.
func (s *session) syncDrawable() error { return s.surface.SyncDrawable() }

// setVolume changes the engine volume without persisting anything.
func (s *session) setVolume(ctx context.Context, volume int) error {
	s.volume = volume

	return s.engine.SetVolume(ctx, volume)
}

// close releases the surface before the engine, as the port requires.
func (s *session) close(ctx context.Context) {
	if s.surface != nil {
		s.surface.Close()
	}
	if s.engine != nil {
		_ = s.engine.Close(ctx)
	}
}

// schemeOf names the URL scheme for the player source, without copying the URL
// or any credentials into logs.
func schemeOf(rawURL string) string {
	if before, _, ok := strings.Cut(rawURL, "://"); ok && before != "" {
		return strings.ToLower(before)
	}

	return ""
}
