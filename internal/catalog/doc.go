// Package catalog defines mumu's channel model, validation and provider
// boundary.
//
// The package owns no network, storage or UI. Playlists, built-in providers
// and user imports all normalize into these types, and adapters map them to
// SQLite or the player without changing their meaning.
package catalog
