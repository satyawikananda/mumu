// Package storage persists mumu's local user data in SQLite.
//
// It owns user intent only: playlist and stream sources, normalized channels,
// favorites, recent playback and settings. Network catalog payload caching is
// intentionally out of scope for this package. A later issue may add a bounded
// cache with a new additive migration; the migration runner supports that.
package storage
