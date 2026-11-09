package player

import "errors"

// Controller errors.
var (
	// ErrClosed reports use after Close.
	ErrClosed = errors.New("player is closed")
	// ErrBusy reports an intent queue that cannot accept more work.
	ErrBusy = errors.New("player is busy")
	// ErrInvalid reports an unusable source or setting.
	ErrInvalid = errors.New("invalid player request")
)
