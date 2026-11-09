package playlist

import (
	"errors"
	"fmt"

	"mumu/internal/catalog"
)

// Sentinel errors identify failures callers can surface without parsing text.
var (
	// ErrMalformed reports a playlist the parser cannot use.
	ErrMalformed = errors.New("malformed playlist")
	// ErrNoChannels reports a playlist with no usable channel.
	ErrNoChannels = errors.New("playlist has no channels")
	// ErrUnsupported reports a body that is neither a playlist nor media.
	ErrUnsupported = errors.New("unsupported source")
	// ErrNotPlaylist reports a single stream where a playlist was expected.
	ErrNotPlaylist = errors.New("source is not a channel playlist")
	// ErrPlaylistNotStream reports a channel list where a stream was expected.
	ErrPlaylistNotStream = errors.New("source is a channel playlist, not a single stream")
	// ErrTooLarge reports a source over the configured byte limit.
	ErrTooLarge = errors.New("source is too large")
	// ErrUnreadable reports a local file that cannot be read.
	ErrUnreadable = errors.New("source cannot be read")
)

// Code is a stable reason for a warning or an error.
type Code string

// Playlist codes.
const (
	CodeMissingHeader     Code = "missing_header"
	CodeEncodingFallback  Code = "encoding_fallback"
	CodeMalformedLine     Code = "malformed_line"
	CodeMissingURL        Code = "missing_url"
	CodeMissingName       Code = "missing_name"
	CodeUnsupportedScheme Code = "unsupported_scheme"
	CodePrivateAddress    Code = "private_address"
	CodeCredentials       Code = "credentials_unsupported"
	CodeDuplicateProvider Code = "duplicate_provider"
	CodeTruncated         Code = "truncated"
	CodeEmptyPlaylist     Code = "empty_playlist"
	CodeStreamNotPlaylist Code = "stream_not_playlist"
	CodeUnsupported       Code = "unsupported_source"
	CodeTooLarge          Code = "source_too_large"
	CodeUnreadable        Code = "source_unreadable"
	CodeInvalidURL        Code = "invalid_url"
	CodeInvalidHeader     Code = "invalid_header"
)

// SkippedEntry explains one channel the parser could not use. It never holds
// a full URL, query string or credential.
type SkippedEntry struct {
	Line   int
	Name   string
	Reason Code
	Scheme string
	Host   string
}

// Error is a parse or classification failure with a stable code.
type Error struct {
	Code    Code
	Line    int
	Message string
	Err     error
}

// Error implements error.
func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s (%s)", e.Line, e.Message, e.Code)
	}

	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

// Unwrap returns the sentinel error.
func (e *Error) Unwrap() error {
	return e.Err
}

// Warning is a skipped or suspicious line that did not fail the playlist.
type Warning struct {
	Line    int
	Code    Code
	Message string
}

// Kind classifies what a source body contains.
type Kind uint8

// Source kinds.
const (
	KindUnknown Kind = iota
	KindChannelList
	KindHLSManifest
	KindDirectStream
)

// String returns a short name for logs and previews.
func (k Kind) String() string {
	switch k {
	case KindChannelList:
		return "channel list"
	case KindHLSManifest:
		return "HLS manifest"
	case KindDirectStream:
		return "direct stream"
	default:
		return "unknown"
	}
}

// Result is a parsed playlist or a classified stream body.
type Result struct {
	Kind            Kind
	Name            string
	Channels        []catalog.Channel
	Warnings        []Warning
	Skipped         int
	SkippedReasons  map[Code]int
	SkippedEntries  []SkippedEntry
	Truncated       bool
	OmittedWarnings int
}
