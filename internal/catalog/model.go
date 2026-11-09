package catalog

import (
	"fmt"
	"strings"
	"time"
)

// SourceKind classifies where a set of channels comes from.
type SourceKind string

// Supported source kinds.
const (
	SourceBuiltin        SourceKind = "builtin"
	SourceRemotePlaylist SourceKind = "remote_playlist"
	SourceLocalPlaylist  SourceKind = "local_playlist"
	SourceCustomStream   SourceKind = "custom_stream"
)

// Valid reports whether k is a supported source kind.
func (k SourceKind) Valid() bool {
	switch k {
	case SourceBuiltin, SourceRemotePlaylist, SourceLocalPlaylist, SourceCustomStream:
		return true
	default:
		return false
	}
}

// Source is a playlist, a single stream or the built-in community catalog.
type Source struct {
	ID            string
	Kind          SourceKind
	Identity      string
	Location      string
	DisplayName   string
	Enabled       bool
	LastRefreshAt time.Time
	LastSuccessAt time.Time
	RefreshETag   string
	RefreshError  string
	ItemCount     int
}

// Channel is one playable entry scoped to a source.
//
// ProviderID is an explicit provider identifier when the playlist supplies
// one, otherwise a hash of the canonical stream URL. A display name is never
// an identity, and the pair (SourceID, ProviderID) is unique.
type Channel struct {
	SourceID   string
	ProviderID string
	Name       string
	Group      string
	Country    string
	Language   string
	LogoURL    string
	EPGID      string
	Streams    []Stream
}

// Stream is one playable variant of a channel.
type Stream struct {
	ProviderID string
	URL        string
	Quality    string
	HLS        bool
	Live       bool
	Headers    Headers
}

// Headers holds the small set of playback headers mumu may keep. It never
// contains credentials.
type Headers map[string]string

// Get returns the value of key, matching names case-insensitively.
func (h Headers) Get(key string) string {
	for name, value := range h {
		if strings.EqualFold(name, key) {
			return value
		}
	}

	return ""
}

// Clone returns a copy that callers can mutate.
func (h Headers) Clone() Headers {
	if len(h) == 0 {
		return nil
	}

	out := make(Headers, len(h))
	for name, value := range h {
		out[name] = value
	}

	return out
}

// Validate reports whether the source can be stored and presented.
func (s Source) Validate() error {
	if !s.Kind.Valid() {
		return invalid("kind", CodeInvalidRecord, nil, fmt.Sprintf("unknown source kind %q", s.Kind))
	}
	if strings.TrimSpace(s.Location) == "" {
		return invalid("location", CodeInvalidURL, nil, "source has no location")
	}
	if len(s.DisplayName) > 200 {
		return invalid("display_name", CodeInvalidRecord, nil, "display name is too long")
	}

	return nil
}

// Validate reports whether the channel has a name and at least one stream.
func (c Channel) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return invalid("name", CodeMissingName, nil, "channel has no name")
	}
	if len(c.Name) > 400 {
		return invalid("name", CodeInvalidRecord, nil, "channel name is too long")
	}
	if len(c.Streams) == 0 {
		return invalid("streams", CodeMissingStream, nil, "channel has no streams")
	}
	for i := range c.Streams {
		if err := c.Streams[i].Validate(); err != nil {
			return fmt.Errorf("stream %d: %w", i, err)
		}
	}

	return nil
}

// Validate reports whether the stream URL is usable.
func (s Stream) Validate() error {
	if strings.TrimSpace(s.URL) == "" {
		return invalid("url", CodeMissingStream, nil, "stream has no URL")
	}
	if len(s.URL) > maxURLLength {
		return invalid("url", CodeInvalidURL, nil, "stream URL is too long")
	}
	if hasControl(s.URL) {
		return invalid("url", CodeInvalidURL, nil, "stream URL has control characters")
	}

	return nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}

	return false
}
