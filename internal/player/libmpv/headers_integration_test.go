//go:build (darwin && arm64) || ((windows || linux) && amd64)

package libmpv_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mumu/internal/player"
)

const (
	headerTestUserAgent = "mumu-header-test/0.1"
	headerTestReferer   = "https://example.com/watch"
)

// TestIntegrationPlaybackHeaders proves that the sanitized User-Agent and
// Referer reach a real HTTP server through libmpv, and that no other header is
// forwarded.
//
// It needs a machine with libmpv, which the adapter discovers, and
// MUMU_MPV_HEADER_TEST=1. The environment recorded in
// docs/architecture/sources.md is the only evidence
// this test provides; a skip is not a passing result.
func TestIntegrationPlaybackHeaders(t *testing.T) {
	requireIntegration(t, "MUMU_MPV_HEADER_TEST")

	requests := make(chan http.Header, 8)
	server := newSyntheticServer(t, func(r *http.Request) {
		select {
		case requests <- r.Header.Clone():
		default:
		}
	})
	engine := newHeadlessEngine(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := engine.Load(ctx, 1, player.Source{
		URL:          server.URL + "/live.m3u8",
		Scheme:       "http",
		AllowPrivate: true,
		Headers: map[string]string{
			"User-Agent": headerTestUserAgent,
			"Referer":    headerTestReferer,
			"Cookie":     "session=must-not-leak",
		},
	}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	deadline := time.After(20 * time.Second)
	for {
		select {
		case header := <-requests:
			if header.Get("User-Agent") != headerTestUserAgent {
				continue
			}
			if got := header.Get("Referer"); got != headerTestReferer {
				t.Fatalf("Referer = %q, want %q", got, headerTestReferer)
			}
			if cookie := header.Get("Cookie"); cookie != "" {
				t.Fatalf("Cookie reached the server: %q", cookie)
			}

			if err := engine.Stop(ctx); err != nil {
				t.Errorf("Stop: %v", err)
			}

			return
		case <-deadline:
			t.Fatalf("no request carried User-Agent %q", headerTestUserAgent)
		}
	}
}
