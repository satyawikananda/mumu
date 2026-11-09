//go:build darwin && arm64 && mumu_mpv

package libmpv_test

import (
	"context"
	"os"
	"testing"
	"time"

	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

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
