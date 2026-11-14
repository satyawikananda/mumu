//go:build darwin && arm64

package libmpv_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"mumu/internal/player"
	"mumu/internal/player/libmpv"
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
	if os.Getenv("MUMU_MPV_HEADER_TEST") == "" {
		t.Skip("set MUMU_MPV_HEADER_TEST=1 to run the libmpv playback-header test")
	}

	requests := make(chan http.Header, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r.Header.Clone():
		default:
		}

		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nsegment.ts\n"))
	}))
	t.Cleanup(server.Close)

	engine, err := libmpv.New(libmpv.Config{
		VideoOutput: "null",
		AudioOutput: "null",
		Muted:       true,
		Logger:      slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("libmpv.New: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := engine.Close(ctx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := engine.Load(ctx, 1, player.Source{
		URL:    server.URL + "/live.m3u8",
		Scheme: "http",
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
