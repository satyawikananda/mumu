package proxy_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mumu/internal/netguard"
	"mumu/internal/netguard/proxy"
)

func clientFor(t *testing.T, p *proxy.Proxy) *http.Client {
	t.Helper()

	parsed, err := url.Parse(p.URL())
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}

	return &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(parsed),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test server certificate
	}}
}

// A strict proxy refuses an absolute-form request to a loopback server.
func TestForwardRejectsPrivateWithoutConsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "reached")
	}))
	defer server.Close()

	p, err := proxy.Start(netguard.Policy{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	response, err := clientFor(t, p).Get(server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", response.StatusCode)
	}
}

// A proxy that has been told the current source consents forwards the request.
func TestForwardAllowsPrivateWithConsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "reached")
	}))
	defer server.Close()

	p, err := proxy.Start(netguard.Policy{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()
	p.SetAllowPrivate(true)

	response, err := clientFor(t, p).Get(server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "reached") {
		t.Fatalf("status %d body %q, want the loopback server", response.StatusCode, body)
	}
}

// A strict proxy refuses a CONNECT tunnel to a loopback TLS server.
func TestConnectRejectsPrivateWithoutConsent(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "tunneled")
	}))
	defer server.Close()

	p, err := proxy.Start(netguard.Policy{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	if _, err := clientFor(t, p).Get(server.URL); err == nil {
		t.Fatal("Get succeeded, want a refused tunnel")
	}
}

// A proxy told the current source consents tunnels to a loopback TLS server.
func TestConnectAllowsPrivateWithConsent(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "tunneled")
	}))
	defer server.Close()

	p, err := proxy.Start(netguard.Policy{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()
	p.SetAllowPrivate(true)

	response, err := clientFor(t, p).Get(server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "tunneled") {
		t.Fatalf("status %d body %q, want the tunneled response", response.StatusCode, body)
	}
}

// Turning consent back off restores the strict policy.
func TestSetAllowPrivateRestoresStrict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "reached")
	}))
	defer server.Close()

	p, err := proxy.Start(netguard.Policy{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	p.SetAllowPrivate(true)
	response, err := clientFor(t, p).Get(server.URL)
	if err != nil {
		t.Fatalf("consented Get: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("consented status %d, want 200", response.StatusCode)
	}

	p.SetAllowPrivate(false)
	response, err = clientFor(t, p).Get(server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403 after consent is cleared", response.StatusCode)
	}
}

// The proxy URL is always a loopback address.
func TestURLIsLoopback(t *testing.T) {
	p, err := proxy.Start(netguard.Policy{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	parsed, err := url.Parse(p.URL())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("host %q, want 127.0.0.1", parsed.Hostname())
	}
	if parsed.Port() == "" {
		t.Fatal("proxy URL has no port")
	}
}
