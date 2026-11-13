//go:build darwin && arm64

package libmpv_test

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

// TestIntegrationSoak plays one source for MUMU_SOAK_SECONDS and fails when
// the controller leaves the playing/buffering states. It exercises the engine
// and its event pump for a long run without a window. It does not measure the
// app's RSS or GPU use; packaging/qa/sample.sh does that against the real app.
//
// Set MUMU_SOAK_URL to an authorized HLS URL and MUMU_SOAK_SECONDS to the
// duration. A skip is not a pass.
func TestIntegrationSoak(t *testing.T) {
	sourceURL := os.Getenv("MUMU_SOAK_URL")
	if sourceURL == "" {
		t.Skip("set MUMU_SOAK_URL to an authorized HLS URL")
	}
	seconds := 60
	if value := os.Getenv("MUMU_SOAK_SECONDS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			t.Fatalf("MUMU_SOAK_SECONDS=%q is not a positive integer", value)
		}
		seconds = parsed
	}

	engine, err := libmpv.New(libmpv.Config{
		VideoOutput: "null",
		AudioOutput: "null",
		Muted:       true,
		Logger:      slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("libmpv.New: %v", err)
	}

	controller := player.New(
		engine,
		player.WithStallTimeout(20*time.Second),
		player.WithInitialMuted(true),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := controller.Close(ctx); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if err := controller.Play(player.Source{URL: sourceURL, Scheme: "http"}); err != nil {
		t.Fatalf("Play: %v", err)
	}

	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	start := time.Now()
	playing := false
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		switch snapshot.State {
		case player.StatePlaying:
			playing = true
		case player.StateBuffering:
		case player.StateError:
			t.Fatalf("playback failed after %s: %s", time.Since(start).Round(time.Second), snapshot.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !playing {
		t.Fatalf("playback never reached playing; last snapshot %+v", controller.Snapshot())
	}
}
