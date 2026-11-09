package playlist

import "mumu/internal/catalog"

const (
	defaultMaxChannels = 20000
	defaultMaxWarnings = 100
	defaultMaxLine     = 8192
	maxSkippedEntries  = 20
)

// Options bounds a parse and decides which destinations may appear in it.
//
// AllowLocalPaths is for a playlist the user opened from disk. AllowPrivateHosts
// is the stored consent to play streams on private or local addresses.
// AllowedSchemes is the per-source scheme allowlist: empty means http and
// https only, and file paths stay governed by AllowLocalPaths alone.
type Options struct {
	MaxChannels       int
	MaxWarnings       int
	MaxLineBytes      int
	DefaultCountry    string
	BaseDir           string
	AllowLocalPaths   bool
	AllowPrivateHosts bool
	AllowedSchemes    []string
}

// WithDefaults returns a copy with zero limits replaced by safe defaults and
// the scheme allowlist normalized.
func (o Options) WithDefaults() Options {
	if o.MaxChannels <= 0 {
		o.MaxChannels = defaultMaxChannels
	}
	if o.MaxWarnings <= 0 {
		o.MaxWarnings = defaultMaxWarnings
	}
	if o.MaxLineBytes <= 0 {
		o.MaxLineBytes = defaultMaxLine
	}
	o.AllowedSchemes = catalog.NormalizeSchemes(o.AllowedSchemes)

	return o
}
