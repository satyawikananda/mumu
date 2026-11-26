//go:build (darwin && arm64) || ((windows || linux) && amd64)

package libmpv_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"mumu/internal/player"
	"mumu/internal/player/libmpv"
)

const integrationGate = "MUMU_MPV_TEST"

// TestIntegrationEngineCreateLoadStop exercises real decoding and client
// control with an original synthetic HLS fixture. This proves headless only.
func TestIntegrationEngineCreateLoadStop(t *testing.T) {
	requireIntegration(t, integrationGate)
	server := newSyntheticServer(t, nil)
	engine := newHeadlessEngine(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := engine.Load(ctx, 1, player.Source{URL: server.URL + "/live.m3u8", Scheme: "http", AllowPrivate: true}); err != nil {
		t.Fatal(err)
	}
	event := waitEngineEvent(t, engine, func(event player.Event) bool {
		return event.Kind == player.EventMetrics && event.Metrics.TimePos > 0 && event.Metadata.Width > 0
	})
	t.Logf("HEADLESS ONLY: video=%s audio=%s dimensions=%dx%d vo=%s ao=%s", event.Metadata.VideoCodec, event.Metadata.AudioCodec, event.Metadata.Width, event.Metadata.Height, event.Metadata.VideoOutput, event.Metadata.AudioOutput)
	for _, volume := range []int{20, 0, 65} {
		if err := engine.SetVolume(ctx, volume); err != nil {
			t.Fatal(err)
		}
	}
	for _, muted := range []bool{true, false} {
		if err := engine.SetMuted(ctx, muted); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.SetPaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	// libmpv applies pause asynchronously. Allow pending frames to settle, then
	// require a held position for several consecutive telemetry samples.
	held := time.Duration(-1)
	stable := 0
	deadline := time.Now().Add(3 * time.Second)
	for stable < 4 && time.Now().Before(deadline) {
		position := waitEngineEvent(t, engine, isMetrics).Metrics.TimePos
		if position == held {
			stable++
		} else {
			stable = 0
			held = position
		}
	}
	if stable < 4 {
		t.Fatal("pause did not hold the playback position")
	}
	later := held

	if err := engine.SetPaused(ctx, false); err != nil {
		t.Fatal(err)
	}
	waitEngineEvent(t, engine, func(event player.Event) bool { return isMetrics(event) && event.Metrics.TimePos > later })
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := engine.Load(canceled, 2, player.Source{URL: server.URL}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled load: %v", err)
	}
	if err := engine.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(ctx); err != nil {
		t.Fatal(err)
	}
	for range engine.Events() {
	}
	if err := engine.SetVolume(ctx, 50); !errors.Is(err, player.ErrClosed) {
		t.Fatalf("closed engine: %v", err)
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
		player.WithStallTimeout(20*time.Second),
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
			if err := controller.Pause(); err != nil {
				t.Fatalf("Pause: %v", err)
			}
			waitForState(t, controller, player.StatePaused, 5*time.Second)
			time.Sleep(1200 * time.Millisecond)
			held := controller.Snapshot()
			time.Sleep(1200 * time.Millisecond)
			later := controller.Snapshot()
			if later.Metrics.TimePos != held.Metrics.TimePos {
				t.Fatalf("paused time-pos moved from %s to %s; the frame is not held", held.Metrics.TimePos, later.Metrics.TimePos)
			}
			if err := controller.Resume(); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			waitForState(t, controller, player.StatePlaying, 25*time.Second)

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

// waitForState waits for one real engine transition within the timeout and
// returns the snapshot that showed it.
func waitForState(t *testing.T, controller *player.Controller, want player.State, timeout time.Duration) player.Snapshot {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		if snapshot.State == want {
			return snapshot
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("state %s, want %s; last snapshot %+v", controller.Snapshot().State, want, controller.Snapshot())

	return player.Snapshot{}
}
