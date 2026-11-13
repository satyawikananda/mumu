package player

// State is one observed playback state.
//
// The zero value is unknown so an uninitialized Snapshot cannot be mistaken
// for idle.
type State uint8

// Playback states.
const (
	StateUnknown State = iota
	StateIdle
	StateLoading
	StatePlaying
	StateBuffering
	StatePaused
	StateError
	StateStopping
	StateStopped
)

// String returns a stable label for logs and tests.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateLoading:
		return "loading"
	case StatePlaying:
		return "playing"
	case StateBuffering:
		return "buffering"
	case StatePaused:
		return "paused"
	case StateError:
		return "error"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// CanPlay reports whether a new Play may start from this state.
func (s State) CanPlay() bool {
	switch s {
	case StateIdle, StateLoading, StatePlaying, StateBuffering, StatePaused, StateError, StateStopped:
		return true
	default:
		return false
	}
}

// Active reports whether the state has a source in progress.
func (s State) Active() bool {
	switch s {
	case StateLoading, StatePlaying, StateBuffering, StatePaused, StateStopping:
		return true
	default:
		return false
	}
}
