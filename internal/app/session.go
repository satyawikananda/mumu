package app

import (
	"context"
	"sync"
)

// runSession identifies one surface's current background run. Beginning a
// run cancels the previous one, so stale network results cannot republish
// over newer state.
type runSession struct {
	mu     sync.Mutex
	gen    uint64
	cancel context.CancelFunc
}

// begin starts a run, canceling the previous one.
func (s *runSession) begin(parent context.Context) (uint64, context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.gen++
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel

	return s.gen, ctx
}

// current reports whether gen is still the active run.
func (s *runSession) current(gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.gen == gen
}

// generation returns the active run's number, or zero before the first run.
func (s *runSession) generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.gen
}
