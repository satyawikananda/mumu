package playlist

const (
	defaultMaxChannels = 20000
	defaultMaxWarnings = 100
	defaultMaxLine     = 8192
	maxSkippedEntries  = 20
)

// Options bounds a parse and decides which destinations may appear in it.
//
// AllowLocalPaths is for a playlist the user opened from disk. AllowPrivateHosts
// is for sources the user typed or chose; a remote catalog keeps it off.
type Options struct {
	MaxChannels       int
	MaxWarnings       int
	MaxLineBytes      int
	DefaultCountry    string
	BaseDir           string
	AllowLocalPaths   bool
	AllowPrivateHosts bool
}

// WithDefaults returns a copy with zero limits replaced by safe defaults.
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

	return o
}
