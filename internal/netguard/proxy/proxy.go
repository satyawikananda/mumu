// Package proxy is a loopback HTTP proxy that enforces a netguard.Policy on
// every destination the media engine opens.
//
// The media engine (libmpv/FFmpeg) resolves DNS, follows redirects and fetches
// HLS segments itself, outside netguard. Pointing it at this proxy makes every
// request and tunnel pass the same scheme, credential and resolved-address
// checks mumu applies to its own network calls. The listener is bound to
// 127.0.0.1 with an ephemeral port and is intended to live for one engine.
//
// The per-request policy is selected with SetAllowPrivate: a strict source
// refuses private, loopback and link-local destinations, while a source whose
// stored policy carries explicit user consent may reach them.
package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"mumu/internal/netguard"
)

const (
	dialTimeout           = 10 * time.Second
	headerTimeout         = 15 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	idleConnectionTimeout = 60 * time.Second
)

// Proxy is a running loopback proxy with a strict and a consented policy.
type Proxy struct {
	strict     netguard.Policy
	permissive netguard.Policy
	override   atomic.Bool

	client   *http.Client
	listener net.Listener
	server   *http.Server
}

// Start listens on a loopback ephemeral port and serves until Close.
//
// The proxy starts strict; SetAllowPrivate opts a single consented source into
// private destinations.
func Start(policy netguard.Policy) (*Proxy, error) {
	strict := policy.WithDefaults()
	strict.AllowPrivate = false
	permissive := strict
	permissive.AllowPrivate = true

	p := &Proxy{strict: strict, permissive: permissive}

	transport := &http.Transport{
		// A proxy must not itself use the environment proxy: that would route
		// around the address policy.
		Proxy:                 nil,
		DialContext:           p.dialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: headerTimeout,
		IdleConnTimeout:       idleConnectionTimeout,
		// Each request must dial under the policy in force for its source.
		// A pooled connection could otherwise carry a consented private
		// route into a later strict request for the same host.
		DisableKeepAlives: true,
	}
	p.client = &http.Client{
		Transport: transport,
		// The media engine follows redirects itself, so every hop comes back
		// through the proxy and is validated again.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start playback proxy: %w", err)
	}
	p.listener = listener
	p.server = &http.Server{
		Handler:           p,
		ReadHeaderTimeout: headerTimeout,
		IdleTimeout:       idleConnectionTimeout,
	}

	go func() {
		_ = p.server.Serve(listener)
	}()

	return p, nil
}

// SetAllowPrivate selects the policy used for the current source.
func (p *Proxy) SetAllowPrivate(allow bool) {
	if p == nil {
		return
	}

	p.override.Store(allow)
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
}

// policy returns the policy for the current source.
func (p *Proxy) policy() netguard.Policy {
	if p.override.Load() {
		return p.permissive
	}

	return p.strict
}

func (p *Proxy) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return p.policy().Dialer().DialContext(ctx, network, address)
}

// URL is the proxy address to hand to the media engine.
func (p *Proxy) URL() string {
	if p == nil || p.listener == nil {
		return ""
	}

	return "http://" + p.listener.Addr().String()
}

// Close stops the listener.
func (p *Proxy) Close() error {
	if p == nil || p.server == nil {
		return nil
	}

	return p.server.Close()
}

// ServeHTTP handles both CONNECT tunnels and absolute-form proxy requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)

		return
	}

	p.forward(w, r)
}

// tunnel validates a CONNECT destination and pipes bytes to it.
func (p *Proxy) tunnel(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, "443"
	}
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")

	policy := p.policy()
	if err := policy.ValidateHost(host); err != nil {
		http.Error(w, "destination is not allowed", http.StatusForbidden)

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dialTimeout)
	defer cancel()

	upstream, err := policy.Dialer().DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		http.Error(w, "destination is not reachable", http.StatusBadGateway)

		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "proxy is unsupported", http.StatusInternalServerError)

		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()

		return
	}
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = client.Close()
		_ = upstream.Close()

		return
	}
	if err := buffered.Flush(); err != nil {
		_ = client.Close()
		_ = upstream.Close()

		return
	}

	// A client may pipeline the TLS ClientHello immediately after CONNECT.
	// Drain bytes already buffered by net/http before the general pipe.
	if pending := buffered.Reader.Buffered(); pending > 0 {
		if _, err := io.CopyN(upstream, buffered, int64(pending)); err != nil {
			_ = client.Close()
			_ = upstream.Close()

			return
		}
	}

	go pipe(upstream, client)
	pipe(client, upstream)
}

// forward validates an absolute-form request and streams the response back.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request) {
	target, err := p.policy().ValidateURL(r.URL.String())
	if err != nil {
		http.Error(w, "destination is not allowed", http.StatusForbidden)

		return
	}

	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.URL = target
	out.Host = target.Host
	removeHopByHop(out.Header)

	response, err := p.client.Do(out)
	if err != nil {
		http.Error(w, "destination is not reachable", http.StatusBadGateway)

		return
	}
	defer func() { _ = response.Body.Close() }()

	removeHopByHop(response.Header)
	for name, values := range response.Header {
		w.Header()[name] = values
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

// pipe copies one direction and closes both sides when it ends.
func pipe(dst io.WriteCloser, src io.ReadCloser) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
}

// removeHopByHop strips headers that must not be forwarded by a proxy.
func removeHopByHop(header http.Header) {
	for _, name := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(name)
	}
}

var _ http.Handler = (*Proxy)(nil)
