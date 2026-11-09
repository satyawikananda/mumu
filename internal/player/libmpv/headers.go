package libmpv

import (
	"slices"
	"strings"
)

const (
	maxHeaderFieldValue = 1024
	userAgentField      = "User-Agent"
	refererField        = "Referer"
)

// headerOptionValue builds the value for libmpv's http-header-fields option.
//
// Only User-Agent and Referer are forwarded, matched case-insensitively, and
// they always appear in that order. A value with a control character, an empty
// value, a value longer than 1024 bytes, or a value containing a comma or a
// backslash is dropped: mpv parses the option as a comma-separated list
// escaped with backslash, so forwarding such a value would split it into other
// fields. An empty result means "send no custom field".
//
// The result contains header values, so it must never be logged.
func headerOptionValue(headers map[string]string) string {
	fields := canonicalFields(headers)
	if len(fields) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(fields))
	for _, name := range []string{userAgentField, refererField} {
		value, ok := fields[name]
		if !ok {
			continue
		}
		pairs = append(pairs, name+": "+value)
	}

	return strings.Join(pairs, ",")
}

// canonicalFields maps each accepted header to its sanitized value. Keys are
// visited in sorted order so a duplicate spelling resolves deterministically.
func canonicalFields(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	fields := make(map[string]string, 2)
	for _, key := range keys {
		name, ok := acceptedField(key)
		if !ok {
			continue
		}
		if _, exists := fields[name]; exists {
			continue
		}
		if value := strings.TrimSpace(headers[key]); usableHeaderValue(value) {
			fields[name] = value
		}
	}

	return fields
}

// acceptedField returns the canonical name mumu forwards for a header.
func acceptedField(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "user-agent", "http-user-agent":
		return userAgentField, true
	case "referer", "referrer", "http-referer", "http-referrer":
		return refererField, true
	default:
		return "", false
	}
}

func usableHeaderValue(value string) bool {
	if value == "" || len(value) > maxHeaderFieldValue {
		return false
	}
	if strings.ContainsAny(value, ",\\") {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}

	return true
}
