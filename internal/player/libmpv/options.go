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
type Config struct {
	Volume           int
	Muted            bool
	HardwareDecoding string
	VideoOutput      string
	AudioOutput      string
	NetworkTimeout   time.Duration
	DemuxerMaxBytes  string
	MetricsInterval  time.Duration
	Logger           *slog.Logger
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
