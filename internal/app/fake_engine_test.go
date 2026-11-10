package app

import (
	"context"
	"sync"

	"mumu/internal/player"
)

// fakeEngine is a recording player.Engine, so app tests can prove when
// playback is created and loaded without a native runtime.
type fakeEngine struct {
	mu     sync.Mutex
	loads  int
	source player.Source
	events chan player.Event
	once   sync.Once
}

var _ player.Engine = (*fakeEngine)(nil)

func newFakeEngine(t interface{ Cleanup(func()) }) *fakeEngine {
	engine := &fakeEngine{events: make(chan player.Event, 8)}
	t.Cleanup(func() { _ = engine.Close(context.Background()) })

	return engine
}

func (e *fakeEngine) Load(_ context.Context, _ uint64, source player.Source) error {
	e.mu.Lock()
	e.loads++
	e.source = source
	e.mu.Unlock()

	return nil
}

func (e *fakeEngine) Stop(context.Context) error { return nil }

func (e *fakeEngine) SetVolume(context.Context, int) error { return nil }

func (e *fakeEngine) SetMuted(context.Context, bool) error { return nil }

func (e *fakeEngine) Events() <-chan player.Event { return e.events }

func (e *fakeEngine) Close(context.Context) error {
	e.once.Do(func() { close(e.events) })

	return nil
}

func (e *fakeEngine) loadCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.loads
}

func (e *fakeEngine) loadSource() player.Source {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.source
}
