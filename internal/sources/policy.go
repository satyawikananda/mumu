package sources

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
)

// policyHosts is the host policy used to recognise a private destination
// without any network access. It never allows a private address, so its result
// is exactly the consent question.
var policyHosts = netguard.Policy{}

// analyzeStreams reports whether any accepted stream needs the user's consent
// and which schemes the channels use. It never returns a URL or a credential.
func analyzeStreams(channels []catalog.Channel) (needsPrivate bool, schemes []string) {
	seen := make(map[string]struct{}, 2)
	for _, channel := range channels {
		for _, stream := range channel.Streams {
			scheme, private := describeStream(stream.URL)
			if scheme != "" {
				seen[scheme] = struct{}{}
			}
			if private {
				needsPrivate = true
			}
		}
	}

	return needsPrivate, sortedSchemes(seen)
}

// describeStream returns the lower-case scheme of rawURL and whether its host
// is private, loopback or link-local. A filesystem path has no scheme.
func describeStream(rawURL string) (scheme string, private bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || !strings.Contains(rawURL, "://") {
		return "", false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}

	return strings.ToLower(parsed.Scheme), isPrivateHost(parsed.Hostname())
}

func isPrivateHost(hostname string) bool {
	if strings.TrimSpace(hostname) == "" {
		return false
	}

	return errors.Is(policyHosts.ValidateHost(hostname), netguard.ErrPrivateAddress)
}

func sortedSchemes(seen map[string]struct{}) []string {
	schemes := make([]string, 0, len(seen))
	for scheme := range seen {
		schemes = append(schemes, scheme)
	}
	slices.Sort(schemes)

	return schemes
}

// previewPolicy is the policy Save may store. Consent is honoured only for a
// source that reported private hosts, and a built-in source is always strict.
func previewPolicy(preview Preview) catalog.Policy {
	if preview.Kind == catalog.SourceBuiltin {
		return catalog.StrictPolicy()
	}

	return catalog.NewPolicy(preview.AllowPrivate && preview.NeedsPrivate, preview.Schemes)
}

// applyPolicy keeps only the channels and streams the policy allows and
// reports how many channels were dropped because nothing in them survived.
func applyPolicy(
	kind catalog.SourceKind,
	policy catalog.Policy,
	channels []catalog.Channel,
) (kept []catalog.Channel, dropped int) {
	kept = make([]catalog.Channel, 0, len(channels))

	for _, channel := range channels {
		streams := make([]catalog.Stream, 0, len(channel.Streams))
		for _, stream := range channel.Streams {
			if streamAllowed(kind, policy, stream.URL) {
				streams = append(streams, stream)
			}
		}
		if len(streams) == 0 {
			dropped++

			continue
		}
		channel.Streams = streams
		kept = append(kept, channel)
	}

	return kept, dropped
}

// streamAllowed reports whether one stream URL is inside a source policy.
//
// A filesystem path is a local-playlist choice and never a remote one. A
// network stream needs an allowed scheme and, without consent, a public host.
func streamAllowed(kind catalog.SourceKind, policy catalog.Policy, rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	if !strings.Contains(rawURL, "://") {
		return kind == catalog.SourceLocalPlaylist || kind == catalog.SourceCustomStream
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if parsed.User != nil {
		return false
	}
	if !policy.AllowsScheme(parsed.Scheme) {
		return false
	}
	if policy.AllowPrivate {
		return true
	}

	return policyHosts.ValidateHost(parsed.Hostname()) == nil
}
