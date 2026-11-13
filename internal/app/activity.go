package app

// activityKind tells a window whether the pointer moved or something actionable
// happened. Watch treats the two differently so its hidden chevron stays
// clickable; PiP shows its chrome on either.
type activityKind uint8

const (
	activityMove activityKind = iota + 1
	activityInput
)
