//go:build !darwin

package app

// startActivityMonitor is unavailable without AppKit. The chrome stays visible
// instead of guessing pointer activity.
func startActivityMonitor(uintptr, func(activityKind)) (func(), error) {
	return nil, nil
}
