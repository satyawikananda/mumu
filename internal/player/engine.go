package player

import "context"

// EventKind classifies an engine event.
type EventKind uint8

// Engine events.
const (
	EventLoaded EventKind = iota + 1
	EventStarted
	EventPlaying
	EventBuffering
	EventEnded
	EventFailed
	EventMetrics
	EventShutdown
)

// Event is emitted by an Engine and tagged with the generation that produced
// it. The controller drops events from a generation it has already replaced.
type Event struct {
	Generation   uint64
	Kind         EventKind
	Err          error
	Detail       string
	Buffering    bool
	Metadata     Metadata
	Metrics      Metrics
	Capabilities Capabilities
}

// Engine is the media backend port. Implementations must:
//
//   - keep all client-library calls on one goroutine;
//   - return promptly from Load, Stop, SetVolume, SetMuted and SetPaused;
//   - tag every file event with the generation passed to Load;
//   - never put source URLs or credentials in Err or Detail;
//   - make Close idempotent.
//
// SetPaused(true) holds the displayed frame and stops audio. An implementation
// also bounds demuxer read-ahead while paused, as far as its backend allows, and
// restores it on SetPaused(false). Stop leaves the engine unpaused.
type Engine interface {
	Load(ctx context.Context, generation uint64, source Source) error
	Stop(ctx context.Context) error
	SetVolume(ctx context.Context, volume int) error
	SetMuted(ctx context.Context, muted bool) error
	SetPaused(ctx context.Context, paused bool) error
	Events() <-chan Event
	Close(ctx context.Context) error
}
