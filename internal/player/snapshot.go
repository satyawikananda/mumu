package player

import "time"

// Source is one stream to play.
//
// Scheme is the declared URL scheme; an empty value is read from URL. Only
// http and https are accepted; the source policy decides whether a private
// host or a playback header is allowed, and the controller never fetches.
//
// Headers holds the playback headers the engine may forward. Only
// User-Agent and Referer reach libmpv, and Header values must never be logged.
type Source struct {
	URL     string
	Scheme  string
	Headers map[string]string
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

// Snapshot is the complete observable controller state. It is safe to copy;
// compare two snapshots with equal because Source carries a header map.
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

// equal reports whether both snapshots hold the same observable state. The
// struct is no longer comparable because Source.Headers is a map.
func (s Snapshot) equal(other Snapshot) bool {
	return s.State == other.State &&
		s.Generation == other.Generation &&
		s.Detail == other.Detail &&
		s.Error == other.Error &&
		s.Volume == other.Volume &&
		s.Muted == other.Muted &&
		s.Metadata == other.Metadata &&
		s.Metrics == other.Metrics &&
		s.Capabilities == other.Capabilities &&
		s.Source.URL == other.Source.URL &&
		s.Source.Scheme == other.Source.Scheme &&
		equalHeaders(s.Source.Headers, other.Source.Headers)
}

func equalHeaders(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for name, value := range a {
		other, ok := b[name]
		if !ok || other != value {
			return false
		}
	}

	return true
}
