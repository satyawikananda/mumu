package library

import (
	"net/url"
	"strings"
	"time"

	"mumu/internal/catalog"
)

// Entry is one favorite or recently watched channel with its source.
//
// Key is the stable identity shared by Discover, Watch and the Library.
// PlayedAt and PlayCount are zero for a favorite. Playable reports whether the
// source is enabled and at least one stream variant is an HTTP(S) URL with a
// host; it is a local policy check, never a reachability test.
type Entry struct {
	Key       string
	Channel   catalog.Channel
	Source    catalog.Source
	PlayedAt  time.Time
	PlayCount int
	Playable  bool
}

// Key returns the stable identity used across Discover, Watch and Library.
func Key(sourceID, providerID string) string {
	return sourceID + ":" + providerID
}

// playable reports whether an entry's user-visible state allows playback.
func playable(source catalog.Source, streams []catalog.Stream) bool {
	if !source.Enabled {
		return false
	}

	for _, stream := range streams {
		if playableURL(stream.URL) {
			return true
		}
	}

	return false
}

// playableURL reports whether raw is an HTTP(S) URL with a host. This mirrors
// the schemes catalog.SchemeSupported enables, without any network work.
func playableURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}

	switch strings.ToLower(parsed.Scheme) {
	case catalog.SchemeHTTP, catalog.SchemeHTTPS:
		return true
	default:
		return false
	}
}
