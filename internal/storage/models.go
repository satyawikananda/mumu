package storage

import "time"

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

// Source is a user-owned playlist, stream or built-in catalog.
//
// Identity is the stable, credential-free form of Location. ID is derived
// from Kind and Identity, so re-importing the same source keeps favorites
// and recent channels.
type Source struct {
	ID            string
	Kind          SourceKind
	Identity      string
	Location      string
	DisplayName   string
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastRefreshAt time.Time
	LastSuccessAt time.Time
	RefreshETag   string
	RefreshError  string
	ItemCount     int
}

// Channel is one playable entry scoped to a Source.
//
// ProviderID is never the display name. It is an explicit provider ID when
// available, otherwise a hash of the canonical stream URL. The pair
// (SourceID, ProviderID) is unique.
type Channel struct {
	ID         int64
	SourceID   string
	ProviderID string
	Name       string
	StreamURL  string
	LogoURL    string
	Country    string
	Category   string
	Language   string
	EPGID      string
	Live       bool
	UpdatedAt  time.Time
}

// Stream is one playable variant of a channel.
//
// ProviderID is unique within the channel. Headers holds only the playback
// headers mumu keeps, never credentials.
type Stream struct {
	ID         int64
	ChannelID  int64
	ProviderID string
	URL        string
	Quality    string
	HLS        bool
	Live       bool
	Position   int
	Headers    map[string]string
}

// Recent is a channel the user genuinely played, most recent first.
type Recent struct {
	Channel   Channel
	PlayedAt  time.Time
	PlayCount int
}
