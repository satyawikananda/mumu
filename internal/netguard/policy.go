package netguard

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	defaultMaxRedirects     = 5
	defaultRequestTimeout   = 10 * time.Second
	defaultMaxResponseBytes = 8 << 20
	defaultUserAgent        = "mumu/0.1 (+https://github.com/satyawikananda/mumu)"
)

// blockedHostSuffixes are local names that never resolve to a public service.
var blockedHostSuffixes = []string{".localhost", ".local", ".internal", ".home.arpa"}

// Policy describes which destinations a client may reach.
//
// AllowPrivate is opt-in for destinations the user typed or chose in a file
// dialog. Catalog-provided URLs keep it off.
type Policy struct {
	AllowPrivate     bool
	MaxRedirects     int
	RequestTimeout   time.Duration
	MaxResponseBytes int64
	UserAgent        string
}

// WithDefaults returns a copy with zero values replaced by safe defaults.
func (p Policy) WithDefaults() Policy {
	if p.MaxRedirects <= 0 {
		p.MaxRedirects = defaultMaxRedirects
	}
	if p.RequestTimeout <= 0 {
		p.RequestTimeout = defaultRequestTimeout
	}
	if p.MaxResponseBytes <= 0 {
		p.MaxResponseBytes = defaultMaxResponseBytes
	}
	if strings.TrimSpace(p.UserAgent) == "" {
		p.UserAgent = defaultUserAgent
	}

	return p
}

// ValidateURL parses raw and applies the scheme, credential and host policy.
func (p Policy) ValidateURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8192 || hasControl(raw) {
		return nil, fmt.Errorf("%w: empty or malformed", ErrInvalidURL)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedScheme, scheme)
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	if parsed.User != nil {
		return nil, ErrCredentials
	}
	if err := p.ValidateHost(parsed.Hostname()); err != nil {
		return nil, err
	}

	parsed.Scheme = scheme

	return parsed, nil
}

// ValidateHost refuses local names and, unless AllowPrivate is set, any
// literal loopback or private address.
func (p Policy) ValidateHost(host string) error {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	if address, err := netip.ParseAddr(host); err == nil {
		return p.CheckAddress(address)
	}

	if host == "localhost" {
		return fmt.Errorf("%w: localhost", ErrPrivateAddress)
	}
	for _, suffix := range blockedHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return fmt.Errorf("%w: local name", ErrPrivateAddress)
		}
	}

	return nil
}

// CheckAddress refuses loopback, private, link-local, multicast and
// unspecified addresses unless AllowPrivate is set.
func (p Policy) CheckAddress(address netip.Addr) error {
	if !address.IsValid() {
		return fmt.Errorf("%w: invalid address", ErrInvalidURL)
	}
	if p.AllowPrivate {
		return nil
	}

	address = address.Unmap()

	private := address.IsLoopback() ||
		address.IsPrivate() ||
		address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() ||
		address.IsInterfaceLocalMulticast() ||
		address.IsMulticast() ||
		address.IsUnspecified()
	if private {
		return fmt.Errorf("%w: %s", ErrPrivateAddress, address)
	}

	return nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}

	return false
}
