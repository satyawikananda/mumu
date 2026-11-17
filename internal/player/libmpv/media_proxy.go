//go:build darwin && arm64

package libmpv

import (
	"fmt"
	"os"

	"mumu/internal/netguard"
	"mumu/internal/netguard/proxy"
)

// mediaProxy routes libmpv's HTTP(S) traffic through a loopback proxy that
// applies the same address policy as the rest of the app. Without it, libmpv
// resolves DNS, follows redirects and fetches HLS segments itself, outside
// netguard.
type mediaProxy struct {
	server *proxy.Proxy
	url    string
}

// startMediaProxy binds a strict proxy and installs it as a libmpv option.
func startMediaProxy() (*mediaProxy, error) {
	// mpv still honors no_proxy when --http-proxy is set, so clear it to keep
	// the validating proxy from being bypassed for a destination. netguard
	// disables environment proxies entirely, so this only affects libmpv.
	_ = os.Unsetenv("no_proxy")
	_ = os.Unsetenv("NO_PROXY")

	server, err := proxy.Start(netguard.Policy{})
	if err != nil {
		return nil, fmt.Errorf("start media proxy: %w", err)
	}

	return &mediaProxy{server: server, url: server.URL()}, nil
}

func (p *mediaProxy) close() {
	if p == nil || p.server == nil {
		return
	}

	_ = p.server.Close()
}

// setPlaybackPolicy selects the proxy policy for the source about to load. A
// strict source keeps the private block; a consented source may reach private
// hosts. The engine installs the proxy itself, so this switch cannot fail.
func (e *Engine) setPlaybackPolicy(allowPrivate bool) {
	if e.proxy == nil {
		return
	}

	e.proxy.server.SetAllowPrivate(allowPrivate)
}
