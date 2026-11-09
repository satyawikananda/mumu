// Package playlist parses M3U channel lists and classifies the difference
// between a playlist and an HLS media manifest.
//
// Parsing is pure and bounded; network reads go through netguard. Local files
// are read with the same size limits, and nothing is fetched for individual
// channels while a playlist is parsed.
package playlist
