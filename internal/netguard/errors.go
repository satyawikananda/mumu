// Package netguard fetches untrusted URLs with a fixed scheme allowlist,
// bounded responses and a dialer that refuses loopback and private networks.
package netguard

import "errors"

// Sentinel errors identify blocked requests without exposing their URLs.
var (
	// ErrInvalidURL reports a URL that cannot be parsed or used.
	ErrInvalidURL = errors.New("invalid URL")
	// ErrUnsupportedScheme reports a scheme outside the HTTP allowlist.
	ErrUnsupportedScheme = errors.New("unsupported URL scheme")
	// ErrCredentials reports an embedded user name or password.
	ErrCredentials = errors.New("URL credentials are not supported")
	// ErrPrivateAddress reports a loopback, private or link-local destination.
	ErrPrivateAddress = errors.New("private network address is not allowed")
	// ErrTooManyRedirects reports a redirect chain longer than the policy.
	ErrTooManyRedirects = errors.New("too many redirects")
	// ErrResponseTooLarge reports a body over the configured limit.
	ErrResponseTooLarge = errors.New("response is too large")
	// ErrStatus reports a non-success HTTP status.
	ErrStatus = errors.New("unexpected HTTP status")
)
