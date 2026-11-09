package libmpv

import "strings"

// renderVideoOutput is the only mpv video output that owns a frame loop. Any
// other value, including the headless default "null", keeps control, events
// and metrics working without a drawable.
const renderVideoOutput = "libmpv"

// renderingEnabled reports whether cfg asks for on-screen video. It is
// deliberately strict: rendering into a native surface requires vo=libmpv, and
// a mismatched configuration must fail before any native object exists.
func renderingEnabled(cfg Config) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.VideoOutput), renderVideoOutput)
}
