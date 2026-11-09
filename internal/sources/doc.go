// Package sources maps the catalog domain to local storage and coordinates
// the built-in community catalog and user source management.
//
// It owns no network policy of its own (playlist and netguard do) and no UI.
// Every stored refresh goes through storage.ReplaceForSource, which keeps the
// last known good rows when a network or parse step fails.
package sources
