package libmpv

import (
	"log/slog"
	"time"
)

const (
	defaultVolume          = 30
	defaultNetworkTimeout  = 10 * time.Second
	defaultMetricsInterval = time.Second
	defaultDemuxerMaxBytes = "32MiB"
)

// Config controls the libmpv client. The zero value is usable.
//
// VideoOutput "null" (the default) keeps playback headless. On-screen frames
// require "libmpv"; the engine only then provides a render context.
//
// BlockPrivateMedia routes libmpv's HTTP(S) traffic through a loopback proxy
// that refuses private, loopback and link-local destinations. Sources whose
// stored policy carries explicit user consent bypass the proxy, which is how
// the documented private-host consent survives at the playback boundary.
type Config struct {
	Volume            int
	Muted             bool
	HardwareDecoding  string
	VideoOutput       string
	AudioOutput       string
	NetworkTimeout    time.Duration
	DemuxerMaxBytes   string
	MetricsInterval   time.Duration
	BlockPrivateMedia bool
	Logger            *slog.Logger
}

// DefaultConfig returns the M0-derived defaults.
func DefaultConfig() Config {
	return Config{
		Volume:           defaultVolume,
		Muted:            false,
		HardwareDecoding: "auto-safe",
		VideoOutput:      "null",
		AudioOutput:      "",
		NetworkTimeout:   defaultNetworkTimeout,
		DemuxerMaxBytes:  defaultDemuxerMaxBytes,
		MetricsInterval:  defaultMetricsInterval,
		Logger:           slog.Default(),
	}
}
