//go:build (darwin && arm64) || ((windows || linux) && amd64)

package libmpv_test

import (
	"context"
	_ "embed"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

//go:embed testdata/synthetic.mpegts
var syntheticSegment []byte

func requireIntegration(t *testing.T, gate string) {
	t.Helper()
	if os.Getenv(gate) == "" {
		t.Skip("set " + gate + "=1 to exercise the real headless engine")
	}
	t.Logf("native runner: %s/%s, %s; fixture: synthetic MPEG-2/MP2 HLS", runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func newSyntheticServer(t *testing.T, observe func(*http.Request)) *httptest.Server {
	t.Helper()
	manifest := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n"
	manifest += strings.Repeat("#EXTINF:2,\nsegment.ts\n#EXT-X-DISCONTINUITY\n", 60) + "#EXT-X-ENDLIST\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if observe != nil {
			observe(r)
		}
		switch r.URL.Path {
		case "/live.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte(manifest))
		case "/segment.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(syntheticSegment)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newHeadlessEngine(t *testing.T) *libmpv.Engine {
	t.Helper()
	engine, err := libmpv.New(libmpv.Config{
		VideoOutput: "null", AudioOutput: "null", HardwareDecoding: "no",
		Muted: true, BlockPrivateMedia: true, MetricsInterval: 100 * time.Millisecond,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := engine.Close(ctx); err != nil {
			t.Errorf("close engine: %v", err)
		}
	})
	return engine
}

func isMetrics(event player.Event) bool { return event.Kind == player.EventMetrics }

func waitEngineEvent(t *testing.T, engine *libmpv.Engine, accept func(player.Event) bool) player.Event {
	t.Helper()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-engine.Events():
			if !ok {
				t.Fatal("engine events closed early")
			}
			if accept(event) {
				return event
			}
			if event.Kind == player.EventFailed {
				t.Fatalf("engine failed: %v", event.Err)
			}
		case <-timer.C:
			t.Fatal("timed out waiting for engine event")
		}
	}
}

func TestIntegrationPlaybackAddressPolicy(t *testing.T) {
	requireIntegration(t, integrationGate)
	var requests atomic.Int64
	server := newSyntheticServer(t, func(*http.Request) { requests.Add(1) })
	engine := newHeadlessEngine(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := player.Source{URL: server.URL + "/live.m3u8", Scheme: "http"}
	if err := engine.Load(ctx, 1, source); err != nil {
		t.Fatal(err)
	}
	waitEngineEvent(t, engine, playbackFailure)
	if requests.Load() != 0 {
		t.Fatal("strict playback reached loopback")
	}
	source.AllowPrivate = true
	if err := engine.Load(ctx, 2, source); err != nil {
		t.Fatal(err)
	}
	waitEngineEvent(t, engine, func(event player.Event) bool { return isMetrics(event) && event.Metrics.TimePos > 0 })
	if requests.Load() == 0 {
		t.Fatal("consented playback never reached fixture")
	}
	if err := engine.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	// A different server distinguishes lingering prior requests from a policy leak.
	var forbidden atomic.Int64
	strict := newSyntheticServer(t, func(*http.Request) { forbidden.Add(1) })
	if err := engine.Load(ctx, 3, player.Source{URL: strict.URL + "/live.m3u8", Scheme: "http"}); err != nil {
		t.Fatal(err)
	}
	waitEngineEvent(t, engine, func(event player.Event) bool { return event.Generation == 3 && playbackFailure(event) })
	if forbidden.Load() != 0 {
		t.Fatal("private consent leaked to the next source")
	}
}

func playbackFailure(event player.Event) bool {
	return event.Kind == player.EventFailed || (event.Kind == player.EventEnded && event.Err != nil)
}
