package player

import "time"

// Source is one stream to play.
//
// Scheme is the declared URL scheme; an empty value is read from URL. Only
// http and https are accepted; the source policy decides whether a private
// host or a playback header is allowed, and the controller never fetches.
//
// AllowPrivate carries the stored source policy to the playback boundary: the
// controller re-validates the URL against it, and the engine routes strict
// sources through a validating proxy so DNS rebinding cannot reach a private
// address. A source that never passed a source policy stays strict.
//
// Headers holds the playback headers the engine may forward. Only
// User-Agent and Referer reach libmpv, and Header values must never be logged.
type Source struct {
	URL          string
	Scheme       string
	AllowPrivate bool
	Headers      map[string]string
}

// Variant is one playable stream of a channel.
//
// Quality is the label the playlist declared, never a measured resolution. An
// empty label is unknown and must not be filled with a guessed value.
type Variant struct {
	Source  Source
	Quality string
}

// Request is one playback episode: the variants of a single channel, in
// preference order, and the quality the viewer selected.
//
// Quality is "auto", "Auto", or an exact Variant.Quality label. Auto keeps the
// supplied order, which is the supported-stream choice the app already made. An
// unavailable label falls back to Auto so the snapshot never claims a variant
// the channel does not offer.
type Request struct {
	Variants []Variant
	Quality  string
}

// Quality reports the selected and actual stream variant.
type Quality struct {
	// Selected is "auto" or the exact label of the chosen variant.
	Selected string
	// Current is the label of the variant actually being attempted, empty
	// when the playlist declared none.
	Current string
	// Known reports that Current carries a real playlist label.
	Known bool
	// Fallback reports that the current unplanned variant is not the selected
	// one, as after a recoverable failure.
	Fallback bool
}

// Recovery reports the bounded reconnect episode.
type Recovery struct {
	// Attempt is the attempt being made, or the next one while Pending, from
	// one through MaxAttempts.
	Attempt int
	// MaxAttempts is three for every episode.
	MaxAttempts int
	// Pending reports that the next attempt waits for its retry floor.
	Pending bool
	// RetryIn is the remaining wait while Pending, for a countdown.
	RetryIn time.Duration
	// Terminal reports exhaustion: no further attempt is scheduled.
	Terminal bool
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
	State      State
	Generation uint64
	// Episode identifies one viewing request. Recovery attempts keep it, so
	// the app records history once per episode rather than per attempt.
	Episode      uint64
	Source       Source
	Detail       string
	Error        string
	Volume       int
	Muted        bool
	Quality      Quality
	Recovery     Recovery
	Metadata     Metadata
	Metrics      Metrics
	Capabilities Capabilities
}

// equal reports whether both snapshots hold the same observable state. The
// struct is no longer comparable because Source.Headers is a map.
func (s Snapshot) equal(other Snapshot) bool {
	return s.State == other.State &&
		s.Generation == other.Generation &&
		s.Episode == other.Episode &&
		s.Detail == other.Detail &&
		s.Error == other.Error &&
		s.Volume == other.Volume &&
		s.Muted == other.Muted &&
		s.Quality == other.Quality &&
		s.Recovery == other.Recovery &&
		s.Metadata == other.Metadata &&
		s.Metrics == other.Metrics &&
		s.Capabilities == other.Capabilities &&
		s.Source.URL == other.Source.URL &&
		s.Source.Scheme == other.Source.Scheme &&
		s.Source.AllowPrivate == other.Source.AllowPrivate &&
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
