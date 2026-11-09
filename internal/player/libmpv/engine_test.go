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

const integrationGate = "MUMU_MPV_TEST"

// TestIntegrationEngineCreateLoadStop proves that the default darwin/arm64
// build creates a real libmpv client, loads a stream and stops it.
//
// It needs a machine with libmpv, which the loader discovers, and
// MUMU_MPV_TEST=1. A skip is not a passing result.
func TestIntegrationEngineCreateLoadStop(t *testing.T) {
	if os.Getenv(integrationGate) == "" {
		t.Skip("set " + integrationGate + "=1 to run the libmpv engine test")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Cleanup(func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		if err := engine.Close(closeCtx); err != nil {
			t.Errorf("engine.Close: %v", err)
		}
	})

	if err := engine.Load(ctx, 1, player.Source{URL: server.URL + "/live.m3u8", Scheme: "http"}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	deadline := time.After(15 * time.Second)
	for {
		select {
		case event, ok := <-engine.Events():
			if !ok {
				t.Fatal("events closed before a load result")
			}
			if event.Generation != 1 {
				continue
			}
			if event.Kind == player.EventFailed && event.Err != nil {
				t.Fatalf("load failed: %v", event.Err)
			}
			if event.Kind == player.EventLoaded || event.Kind == player.EventStarted || event.Kind == player.EventPlaying {
				if err := engine.Stop(ctx); err != nil {
					t.Fatalf("Stop: %v", err)
				}

				return
			}
		case <-deadline:
			t.Fatal("no load event from the real libmpv runtime")
		}
	}
}

func TestIntegrationHLS(t *testing.T) {
	sourceURL := os.Getenv("MUMU_HLS_TEST_URL")
	if sourceURL == "" {
		t.Skip("set MUMU_HLS_TEST_URL to an authorized HLS URL")
	}

	engine, err := libmpv.New(libmpv.Config{
		VideoOutput: "null",
		AudioOutput: "null",
		Muted:       true,
	})
	if err != nil {
		t.Fatalf("libmpv.New: %v", err)
	}

	controller := player.New(
		engine,
		player.WithLoadingTimeout(20*time.Second),
		player.WithInitialMuted(true),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := controller.Close(ctx); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if err := controller.Play(player.Source{URL: sourceURL}); err != nil {
		t.Fatalf("Play: %v", err)
	}

	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		switch snapshot.State {
		case player.StatePlaying:
			if err := controller.SetVolume(20); err != nil {
				t.Fatalf("SetVolume: %v", err)
			}
			if err := controller.SetMuted(true); err != nil {
				t.Fatalf("SetMuted: %v", err)
			}
			if err := controller.Stop(); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			waitStopped(t, controller)

			return
		case player.StateError:
			t.Fatalf("playback failed: %s", snapshot.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("playback did not reach playing; last snapshot %+v", controller.Snapshot())
}

func waitStopped(t *testing.T, controller *player.Controller) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if controller.Snapshot().State == player.StateStopped {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("playback did not stop; last snapshot %+v", controller.Snapshot())
}
