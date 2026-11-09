package iptvorg

import "errors"

// Sentinel errors for catalog metadata and regions.
var (
	// ErrInvalidCountry reports a region code outside the two-letter form.
	ErrInvalidCountry = errors.New("invalid country code")
	// ErrInvalidMetadata reports an unusable iptv-org metadata document.
	ErrInvalidMetadata = errors.New("invalid iptv-org metadata")
)
