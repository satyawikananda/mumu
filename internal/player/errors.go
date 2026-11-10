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
	// ErrUnsupportedScheme reports a stream this build cannot play. Only http
	// and https are enabled; file, udp, rtp, multicast, rtsp, rtmp and srt
	// stay rejected until a recorded macOS libmpv smoke test proves them.
	ErrUnsupportedScheme = errors.New("unsupported stream scheme")
)
