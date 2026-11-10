package video

import (
	"errors"
	"fmt"
)

// Presentation is where the one video surface currently shows.
type Presentation uint8

// Presentation targets and intermediate states.
const (
	// PresentationStopped is terminal during shutdown.
	PresentationStopped Presentation = iota
	// PresentationMain shows the surface in the main window.
	PresentationMain
	// PresentationTransitioning is set only while an attachment runs.
	PresentationTransitioning
	// PresentationPiP shows the surface in the floating window.
	PresentationPiP
)

// Errors reported by Transfer.
var (
	// ErrPresentationStopped reports a transfer after shutdown started.
	ErrPresentationStopped = errors.New("video: presentation is stopped")
	// ErrPresentationBusy reports a transfer while another is in progress.
	ErrPresentationBusy = errors.New("video: presentation is transitioning")
	// ErrPresentationTarget reports a target that is not main or PiP.
	ErrPresentationTarget = errors.New("video: invalid presentation target")
)

// String returns a stable label for logs and tests.
func (p Presentation) String() string {
	switch p {
	case PresentationMain:
		return "main"
	case PresentationTransitioning:
		return "transitioning"
	case PresentationPiP:
		return "pip"
	default:
		return "stopped"
	}
}

// Transfer commits target only after attach succeeds. A failed attachment
// rolls the state back to where it was; stopped and reentrant transfers are
// rejected, and a duplicate transfer is a no-op.
func Transfer(current *Presentation, target Presentation, attach func() error) error {
	if current == nil {
		return fmt.Errorf("%w: nil state", ErrPresentationTarget)
	}
	if *current == PresentationStopped {
		return ErrPresentationStopped
	}
	if *current == PresentationTransitioning {
		return ErrPresentationBusy
	}
	if target != PresentationMain && target != PresentationPiP {
		return fmt.Errorf("%w: %s", ErrPresentationTarget, target)
	}
	if *current == target {
		return nil
	}

	previous := *current
	*current = PresentationTransitioning

	if err := attach(); err != nil {
		*current = previous

		return fmt.Errorf("attach %s: %w", target, err)
	}

	*current = target

	return nil
}
