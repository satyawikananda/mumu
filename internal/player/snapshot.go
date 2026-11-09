package player

import "time"

// Source is one stream to play. Credentials must not be embedded in logs.
type Source struct {
	URL string
}

// Metadata describes the media and the decoder selected by the engine.
type Metadata struct {
	Title           string
	Width           int
	Height          int
	VideoCodec      string
	AudioCodec      string
	HardwareDecoder string
	VideoOutput     string
	AudioOutput     string
	Duration        time.Duration
	Seekable        bool
	Live            bool
}

// Capabilities tells the UI which controls are meaningful.
type Capabilities struct {
	HasAudio bool
	HasVideo bool
	Seekable bool
	Live     bool
}

// Metrics is periodically sampled playback telemetry.
type Metrics struct {
	Generation     uint64
	TimePos        time.Duration
	PausedForCache bool
}

// Snapshot is the complete observable controller state. It is comparable and
// safe to copy.
type Snapshot struct {
	State        State
	Generation   uint64
	Source       Source
	Detail       string
	Error        string
	Volume       int
	Muted        bool
	Metadata     Metadata
	Metrics      Metrics
	Capabilities Capabilities
}
