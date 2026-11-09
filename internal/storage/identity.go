package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	maxProviderIDLength = 512
	sourceIDBytes       = 16
	providerIDBytes     = 16
)

// NewSource validates location and returns a Source with deterministic
// Identity and ID. DisplayName may be empty; callers fall back to the
// location when they present the source.
func NewSource(kind SourceKind, location, displayName string) (Source, error) {
	if !kind.Valid() {
		return Source{}, fmt.Errorf("%w: unsupported source kind %q", ErrInvalid, kind)
	}

	identity, normalized, err := normalizeLocation(kind, location)
	if err != nil {
		return Source{}, err
	}

	return Source{
		ID:          sourceID(kind, identity),
		Kind:        kind,
		Identity:    identity,
		Location:    normalized,
		DisplayName: displayName,
		Enabled:     true,
	}, nil
}

// ProviderID returns a stable channel identity. Explicit provider IDs win.
// Otherwise the canonical stream URL is hashed, never the display name.
func ProviderID(explicit, streamURL string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if len(explicit) > maxProviderIDLength {
			return "", fmt.Errorf("%w: provider ID is too long", ErrInvalid)
		}

		return explicit, nil
	}

	streamURL = strings.TrimSpace(streamURL)
	if streamURL == "" {
		return "", fmt.Errorf("%w: channel has no stream URL", ErrInvalid)
	}

	canonical, err := canonicalStreamURL(streamURL)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(canonical))

	return "url:" + hex.EncodeToString(sum[:providerIDBytes]), nil
}

func sourceID(kind SourceKind, identity string) string {
	sum := sha256.Sum256([]byte(string(kind) + "\x00" + identity))

	return string(kind) + ":" + hex.EncodeToString(sum[:sourceIDBytes])
}

func normalizeLocation(kind SourceKind, location string) (identity, normalized string, err error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", "", fmt.Errorf("%w: source has no location", ErrInvalid)
	}

	switch kind {
	case SourceBuiltin:
		return location, location, nil
	case SourceRemotePlaylist:
		return httpIdentity(location)
	case SourceLocalPlaylist:
		return localIdentity(location)
	case SourceCustomStream:
		if isHTTPURL(location) {
			return httpIdentity(location)
		}
		if strings.Contains(location, "://") {
			return "", "", fmt.Errorf("%w: stream URL must use http or https", ErrInvalid)
		}

		return localIdentity(location)
	default:
		return "", "", fmt.Errorf("%w: unsupported source kind %q", ErrInvalid, kind)
	}
}

func httpIdentity(location string) (identity, normalized string, err error) {
	parsed, err := url.Parse(location)
	if err != nil {
		return "", "", fmt.Errorf("%w: source URL is invalid", ErrInvalid)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("%w: source URL must use http or https", ErrInvalid)
	}
	if parsed.Host == "" {
		return "", "", fmt.Errorf("%w: source URL has no host", ErrInvalid)
	}
	if parsed.User != nil {
		return "", "", ErrCredentialsUnsupported
	}

	parsed.Fragment = ""
	normalized = parsed.String()

	identityURL := *parsed
	identityURL.RawQuery = ""
	identityURL.ForceQuery = false
	identityURL.Scheme = strings.ToLower(identityURL.Scheme)
	identityURL.Host = strings.ToLower(identityURL.Host)
	identity = identityURL.String()

	return identity, normalized, nil
}

func localIdentity(location string) (identity, normalized string, err error) {
	if strings.HasPrefix(location, "file://") {
		parsed, parseErr := url.Parse(location)
		if parseErr != nil {
			return "", "", fmt.Errorf("%w: source path is invalid", ErrInvalid)
		}
		if parsed.User != nil {
			return "", "", ErrCredentialsUnsupported
		}
		location = parsed.Path
	}

	absolute, err := filepath.Abs(location)
	if err != nil {
		return "", "", fmt.Errorf("%w: source path is invalid", ErrInvalid)
	}

	normalized = filepath.Clean(absolute)

	return normalized, normalized, nil
}

func canonicalStreamURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: stream URL is invalid", ErrInvalid)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: stream URL must be absolute", ErrInvalid)
	}
	if parsed.User != nil {
		return "", ErrCredentialsUnsupported
	}

	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	return parsed.String(), nil
}

func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}
