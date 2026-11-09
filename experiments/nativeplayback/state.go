package main

import "fmt"

type presentation string

const (
	mainPresentation presentation = "main"
	pipPresentation  presentation = "pip"
	transitioning    presentation = "transitioning"
	stopped          presentation = "stopped"
)

// transfer commits only after the native view has attached successfully.
func transfer(current *presentation, target presentation, attach func() error) error {
	if *current == stopped || *current == transitioning {
		return fmt.Errorf("cannot transfer from %s", *current)
	}
	if target != mainPresentation && target != pipPresentation {
		return fmt.Errorf("invalid presentation %s", target)
	}
	if *current == target {
		return nil
	}
	previous := *current
	*current = transitioning
	if err := attach(); err != nil {
		*current = previous
		return fmt.Errorf("attach %s: %w", target, err)
	}
	*current = target
	return nil
}
