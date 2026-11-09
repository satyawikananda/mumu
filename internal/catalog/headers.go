package catalog

import (
	"fmt"
	"strings"
)

const maxHeaderValueLength = 1024

// acceptedHeaders maps lower-case playlist header names to the canonical
// names mumu keeps. Credentials and cookies are deliberately absent.
var acceptedHeaders = map[string]string{
	"user-agent":      "User-Agent",
	"http-user-agent": "User-Agent",
	"referer":         "Referer",
	"referrer":        "Referer",
	"http-referer":    "Referer",
	"http-referrer":   "Referer",
}

// HeaderName returns the canonical name for an accepted playback header.
func HeaderName(name string) (string, bool) {
	canonical, ok := acceptedHeaders[strings.ToLower(strings.TrimSpace(name))]

	return canonical, ok
}

// SanitizeHeaders keeps only accepted playback headers. Unknown names are
// ignored; values that break the header syntax are rejected.
func SanitizeHeaders(headers map[string]string) (Headers, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	clean := Headers{}
	for name, value := range headers {
		canonical, ok := HeaderName(name)
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > maxHeaderValueLength {
			return nil, invalid("headers", CodeInvalidHeader, nil, fmt.Sprintf("%s is too long", canonical))
		}
		if hasControl(value) {
			return nil, invalid("headers", CodeInvalidHeader, nil, fmt.Sprintf("%s has control characters", canonical))
		}

		clean[canonical] = value
	}
	if len(clean) == 0 {
		return nil, nil
	}

	return clean, nil
}
