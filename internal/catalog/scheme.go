package catalog

import (
	"slices"
	"strings"
)

// Playback schemes mumu has enabled. Only HTTP and HTTPS may reach libmpv in
// this release; see docs/architecture/adr/0007-per-source-stream-policy.md.
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// strictSchemes is the allowlist every source starts with.
var strictSchemes = []string{SchemeHTTP, SchemeHTTPS}

// blockedSchemes stay rejected even when a stored row names them, and even if a
// later change teaches SchemeSupported another transport without removing the
// entry here.
//
// UDP, RTP and multicast transports are unsupported, `file` is a local-path
// decision that only a playlist the user opened from disk may make, and RTSP,
// RTMP and SRT need a recorded macOS libmpv smoke test before they may appear.
var blockedSchemes = map[string]struct{}{
	"file":      {},
	"udp":       {},
	"rtp":       {},
	"rtsp":      {},
	"rtsps":     {},
	"rtmp":      {},
	"rtmps":     {},
	"rtmpt":     {},
	"rtmpe":     {},
	"srt":       {},
	"mms":       {},
	"mmsh":      {},
	"multicast": {},
}

// SchemeSupported reports whether this release can play scheme at all. Only
// http and https are enabled, so a stored policy cannot widen the surface.
func SchemeSupported(scheme string) bool {
	scheme = normalizeScheme(scheme)
	if _, blocked := blockedSchemes[scheme]; blocked {
		return false
	}

	switch scheme {
	case SchemeHTTP, SchemeHTTPS:
		return true
	default:
		return false
	}
}

// NormalizeSchemes returns the deterministic scheme allowlist: values are
// trimmed, lowercased, deduplicated and sorted, HTTP and HTTPS are always
// present, and anything this release cannot play is removed. A nil input yields
// the strict list.
func NormalizeSchemes(schemes []string) []string {
	seen := make(map[string]struct{}, len(schemes)+len(strictSchemes))
	for _, scheme := range schemes {
		if scheme = normalizeScheme(scheme); SchemeSupported(scheme) {
			seen[scheme] = struct{}{}
		}
	}
	for _, scheme := range strictSchemes {
		seen[scheme] = struct{}{}
	}

	normalized := make([]string, 0, len(seen))
	for scheme := range seen {
		normalized = append(normalized, scheme)
	}
	slices.Sort(normalized)

	return normalized
}

// SchemeAllowed reports whether an allowlist permits scheme. The list is
// normalized first, so a blocked or unpublished transport is never allowed.
func SchemeAllowed(schemes []string, scheme string) bool {
	if scheme = normalizeScheme(scheme); !SchemeSupported(scheme) {
		return false
	}
	// HTTP and HTTPS are in every normalized allowlist; keep this path
	// allocation-free for the scheme check every parsed URL makes.
	if scheme == SchemeHTTP || scheme == SchemeHTTPS {
		return true
	}

	return slices.Contains(NormalizeSchemes(schemes), scheme)
}

func normalizeScheme(scheme string) string {
	return strings.ToLower(strings.TrimSpace(scheme))
}

// Policy is the stream destination policy of one source: whether private and
// local hosts are reachable and which URL schemes may be played.
//
// The zero value is not usable on its own; use StrictPolicy or NewPolicy so
// Schemes is always normalized.
type Policy struct {
	AllowPrivate bool
	Schemes      []string
}

// StrictPolicy is the policy every built-in source and every new user source
// starts with: public hosts over http and https only.
func StrictPolicy() Policy {
	return Policy{Schemes: slices.Clone(strictSchemes)}
}

// NewPolicy returns a policy with a normalized, deterministic allowlist.
func NewPolicy(allowPrivate bool, schemes []string) Policy {
	return Policy{AllowPrivate: allowPrivate, Schemes: NormalizeSchemes(schemes)}
}

// AllowsScheme reports whether the policy permits scheme.
func (p Policy) AllowsScheme(scheme string) bool {
	return SchemeAllowed(p.Schemes, scheme)
}

// EffectivePolicy returns the stream policy the source may use. A built-in
// source is always strict, whatever a stored row claims, and every source is
// clamped to the schemes this release supports.
func (s Source) EffectivePolicy() Policy {
	if s.Kind == SourceBuiltin {
		return StrictPolicy()
	}

	return NewPolicy(s.AllowPrivate, s.AllowedSchemes)
}
