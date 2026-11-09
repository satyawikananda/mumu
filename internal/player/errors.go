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
	// ErrRuntimeUnavailable reports a build or a machine without the native
	// media runtime, so a caller can show an actionable message instead of
	// retrying. It never carries a URL, a header or a user path.
	ErrRuntimeUnavailable = errors.New("native playback runtime is unavailable")
)
