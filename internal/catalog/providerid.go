package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	maxProviderIDLength = 512
	maxURLLength        = 8192
	providerIDBytes     = 16
)

// ProviderID returns a stable channel identity. An explicit provider ID wins;
// otherwise the canonical stream URL is hashed. Display names are never used.
func ProviderID(explicit, streamURL string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if len(explicit) > maxProviderIDLength {
			return "", invalid("provider_id", CodeInvalidRecord, nil, "provider ID is too long")
		}
		if hasControl(explicit) {
			return "", invalid("provider_id", CodeInvalidRecord, nil, "provider ID has control characters")
		}

		return explicit, nil
	}

	canonical, err := CanonicalStreamURL(streamURL)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(canonical))

	return "url:" + hex.EncodeToString(sum[:providerIDBytes]), nil
}

// CanonicalStreamURL normalizes an HTTP(S) URL, file URL or absolute local
// path for hashing and comparison.
func CanonicalStreamURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", invalid("url", CodeMissingStream, nil, "stream URL is empty")
	}
	if len(raw) > maxURLLength {
		return "", invalid("url", CodeInvalidURL, nil, "stream URL is too long")
	}
	if hasControl(raw) {
		return "", invalid("url", CodeInvalidURL, nil, "stream URL has control characters")
	}

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", invalid("url", CodeInvalidURL, err, "stream URL is invalid")
		}

		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			if parsed.Host == "" {
				return "", invalid("url", CodeInvalidURL, nil, "stream URL has no host")
			}
			if parsed.User != nil {
				return "", invalid("url", CodeCredentials, ErrCredentials, "stream URL embeds credentials")
			}

			parsed.Fragment = ""
			parsed.Scheme = strings.ToLower(parsed.Scheme)
			parsed.Host = strings.ToLower(parsed.Host)

			return parsed.String(), nil
		case "file":
			if parsed.User != nil {
				return "", invalid("url", CodeCredentials, ErrCredentials, "file URL embeds credentials")
			}

			return canonicalPath(FileURLPath(parsed))
		default:
			return "", invalid(
				"url",
				CodeUnsupportedScheme,
				ErrUnsupportedScheme,
				fmt.Sprintf("scheme %q is not supported", parsed.Scheme),
			)
		}
	}

	return canonicalPath(raw)
}

// FileURLPath converts a parsed file URL to a host filesystem path. Windows
// drive URLs use /C:/path and UNC URLs use a host, so both need conversion
// before filepath.Abs sees them.
func FileURLPath(location *url.URL) string {
	return fileURLPath(location, runtime.GOOS)
}

func fileURLPath(location *url.URL, goos string) string {
	if location == nil {
		return ""
	}

	host, path := location.Host, location.Path
	if goos == "windows" {
		toWindowsPath := func(value string) string {
			return strings.ReplaceAll(value, "/", `\`)
		}
		if len(host) == 2 && host[1] == ':' {
			return toWindowsPath(host + path)
		}
		if host != "" {
			return `\\` + host + toWindowsPath(path)
		}
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}

		return toWindowsPath(path)
	}

	return filepath.FromSlash(path)
}

func canonicalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", invalid("path", CodeInvalidURL, nil, "local path is empty")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", invalid("path", CodeInvalidURL, err, "local path is invalid")
	}

	return filepath.Clean(absolute), nil
}
