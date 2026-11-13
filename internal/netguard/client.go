package netguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

const (
	dialTimeout           = 5 * time.Second
	tlsHandshakeTimeout   = 5 * time.Second
	responseHeaderTimeout = 8 * time.Second
	idleConnectionTimeout = 30 * time.Second
	maxResponseHeaderSize = 64 << 10
)

// Client fetches untrusted URLs under one Policy. It is safe for concurrent
// use and never follows a redirect without revalidating it.
type Client struct {
	policy Policy
	http   *http.Client
}

// Request describes one bounded HTTP read.
type Request struct {
	URL      string
	MaxBytes int64
	ETag     string
	Range    string
	Accept   string
}

// Response is a fully read, bounded HTTP response.
type Response struct {
	FinalURL     string
	StatusCode   int
	ContentType  string
	ETag         string
	LastModified string
	Body         []byte
	NotModified  bool
}

// New returns a client for policy, with defaults for zero fields.
func New(policy Policy) *Client {
	policy = policy.WithDefaults()

	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: idleConnectionTimeout,
		Control:   policy.control,
	}
	transport := &http.Transport{
		// Environment proxies would route around the address policy.
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           8,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        idleConnectionTimeout,
		TLSHandshakeTimeout:    tlsHandshakeTimeout,
		ResponseHeaderTimeout:  responseHeaderTimeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: maxResponseHeaderSize,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > policy.MaxRedirects {
				return fmt.Errorf("%w: %d", ErrTooManyRedirects, len(via))
			}

			_, err := policy.ValidateURL(req.URL.String())

			return err
		},
	}

	return &Client{policy: policy, http: client}
}

// Fetch reads at most the requested number of bytes. It returns
// NotModified for a 304 response and no body for any response.
func (c *Client) Fetch(ctx context.Context, request Request) (Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	parsed, err := c.policy.ValidateURL(request.URL)
	if err != nil {
		return Response{}, err
	}

	limit := request.MaxBytes
	if limit <= 0 || limit > c.policy.MaxResponseBytes {
		limit = c.policy.MaxResponseBytes
	}

	ctx, cancel := context.WithTimeout(ctx, c.policy.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	req.Header.Set("User-Agent", c.policy.UserAgent)
	req.Header.Set("Accept", acceptOr(request.Accept))
	if request.ETag != "" {
		req.Header.Set("If-None-Match", request.ETag)
	}
	if request.Range != "" {
		req.Header.Set("Range", request.Range)
	}

	resp, err := c.http.Do(req) //nolint:bodyclose // closed below once the body is read or rejected
	if err != nil {
		if errors.Is(err, ErrPrivateAddress) ||
			errors.Is(err, ErrUnsupportedScheme) ||
			errors.Is(err, ErrTooManyRedirects) ||
			errors.Is(err, ErrCredentials) {
			return Response{}, err
		}
		if ctx.Err() != nil {
			return Response{}, fmt.Errorf("fetch: %w", ctx.Err())
		}

		return Response{}, fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	response := Response{
		FinalURL:     resp.Request.URL.String(),
		StatusCode:   resp.StatusCode,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if resp.StatusCode == http.StatusNotModified {
		response.NotModified = true

		return response, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Response{}, &StatusError{Code: resp.StatusCode, Header: resp.Header.Clone()}
	}
	if resp.ContentLength > limit {
		return Response{}, fmt.Errorf("%w: content length %d", ErrResponseTooLarge, resp.ContentLength)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > limit {
		return Response{}, fmt.Errorf("%w: limit %d", ErrResponseTooLarge, limit)
	}

	response.Body = body

	return response, nil
}

// control runs after DNS resolution, before the socket connects, so a host
// name that resolves to a private address is refused too.
func (p Policy) control(network, address string, _ syscall.RawConn) error {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil
	}

	host, _, splitErr := net.SplitHostPort(address)
	if splitErr == nil {
		parsed, parseErr := netip.ParseAddr(host)
		if parseErr == nil {
			return p.CheckAddress(parsed)
		}
	}

	return nil
}

func acceptOr(value string) string {
	if strings.TrimSpace(value) == "" {
		return "*/*"
	}

	return value
}
