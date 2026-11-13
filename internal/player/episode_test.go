package player

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func pauseCalls(engine *fakeEngine) []bool {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	out := make([]bool, len(engine.pauses))
	copy(out, engine.pauses)

	return out
}

func loadAt(engine *fakeEngine, index int) loadCall {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	if index < 0 || index >= len(engine.loads) {
		return loadCall{}
	}

	return engine.loads[index]
}

func waitVirtual(t *testing.T, controller *Controller, label string, done func(Snapshot) bool) Snapshot {
	t.Helper()

	for range 50 {
		synctest.Wait()
		snapshot := controller.Snapshot()
		if done(snapshot) {
			return snapshot
		}
	}

	t.Fatalf("timed out waiting for %s; last snapshot %+v", label, controller.Snapshot())

	return Snapshot{}
}

func TestPauseHoldsSessionAndResumeReloads(t *testing.T) {
	controller, engine := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)

	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitState(t, controller, StatePaused)
	if calls := pauseCalls(engine); len(calls) != 1 || !calls[0] {
		t.Fatalf("pause calls %v, want one true", calls)
	}
	if got := engine.loadCount(); got != 1 {
		t.Fatalf("loads after pause %d, want 1", got)
	}

	if err := controller.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitState(t, controller, StateBuffering)
	if got := engine.loadCount(); got != 2 {
		t.Fatalf("loads after resume %d, want 2", got)
	}
	if load := loadAt(engine, 1); load.source.URL != "https://stream.example/live.m3u8" {
		t.Fatalf("resume loaded %q, want the same source", load.source.URL)
	}

	engine.emit(Event{Generation: 2, Kind: EventLoaded})
	waitSnapshot(t, controller, "unpause after load", func(Snapshot) bool { return len(pauseCalls(engine)) == 2 })
	if calls := pauseCalls(engine); calls[1] {
		t.Fatalf("pause calls after load %v, want a final false", calls)
	}

	engine.emit(Event{Generation: 2, Kind: EventPlaying})
	snapshot := waitState(t, controller, StatePlaying)
	if snapshot.Recovery.Attempt != 0 {
		t.Fatalf("recovery %+v, want cleared after playing", snapshot.Recovery)
	}
}

func TestPauseIgnoresEngineFailure(t *testing.T) {
	controller, engine := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)

	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitState(t, controller, StatePaused)

	engine.emit(Event{Generation: 1, Kind: EventFailed, Err: errors.New("network gone")})
	engine.emit(Event{Generation: 1, Kind: EventPlaying})

	snapshot := controller.Snapshot()
	if snapshot.State != StatePaused || snapshot.Error != "" {
		t.Fatalf("snapshot %+v after stale events, want a clean paused session", snapshot)
	}
}

func TestSwitchChannelWhilePausedUnpausesAfterLoad(t *testing.T) {
	controller, engine := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/a.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)
	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitState(t, controller, StatePaused)

	if err := controller.Play(Source{URL: "https://stream.example/b.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	if got := engine.loadCount(); got != 2 {
		t.Fatalf("loads %d, want 2", got)
	}
	engine.emit(Event{Generation: 2, Kind: EventLoaded})
	waitSnapshot(t, controller, "unpause after load", func(Snapshot) bool { return len(pauseCalls(engine)) == 2 })
	if calls := pauseCalls(engine); calls[1] {
		t.Fatalf("pause calls %v, want the new channel unpaused", calls)
	}
}

func TestRecoveryStopsAfterThreeAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		controller, engine := newTestController(t,
			WithStallTimeout(8*time.Second),
			WithRetryFloor(8*time.Second),
		)
		if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
			t.Fatalf("Play: %v", err)
		}
		waitVirtual(t, controller, "loading", func(s Snapshot) bool { return s.State == StateLoading })

		engine.emit(Event{Generation: 1, Kind: EventFailed, Err: errors.New("first failure")})
		waitVirtual(t, controller, "scheduled retry", func(s Snapshot) bool {
			return s.Recovery.Pending && s.Recovery.Attempt == 2
		})
		if got := engine.loadCount(); got != 1 {
			t.Fatalf("loads %d while waiting for the retry floor, want 1", got)
		}

		time.Sleep(3 * time.Second)
		waitVirtual(t, controller, "countdown tick", func(s Snapshot) bool {
			return s.Recovery.Pending && s.Recovery.RetryIn <= 5*time.Second && s.Recovery.RetryIn > 0
		})

		time.Sleep(5 * time.Second)
		waitVirtual(t, controller, "second attempt", func(s Snapshot) bool { return s.Generation == 2 })
		if got := engine.loadCount(); got != 2 {
			t.Fatalf("loads %d after the floor, want 2", got)
		}

		engine.emit(Event{Generation: 2, Kind: EventFailed, Err: errors.New("second failure")})
		time.Sleep(8 * time.Second)
		waitVirtual(t, controller, "third attempt", func(s Snapshot) bool { return s.Generation == 3 })
		if got := engine.loadCount(); got != 3 {
			t.Fatalf("loads %d, want 3", got)
		}

		engine.emit(Event{Generation: 3, Kind: EventFailed, Err: errors.New("third failure")})
		snapshot := waitVirtual(t, controller, "terminal", func(s Snapshot) bool {
			return s.State == StateError
		})
		if !snapshot.Recovery.Terminal || snapshot.Recovery.Attempt != 3 {
			t.Fatalf("recovery %+v, want terminal attempt 3", snapshot.Recovery)
		}

		time.Sleep(60 * time.Second)
		synctest.Wait()
		if got := engine.loadCount(); got != 3 {
			t.Fatalf("loads %d after exhaustion, want 3", got)
		}
	})
}

func TestRecoveryAdvancesVariantsWithinChannel(t *testing.T) {
	controller, engine := newTestController(t, WithRetryFloor(0))
	request := Request{
		Variants: []Variant{
			{Source: Source{URL: "https://stream.example/a.m3u8"}, Quality: "480p"},
			{Source: Source{URL: "https://stream.example/b.m3u8"}, Quality: "720p"},
		},
		Quality: "auto",
	}
	if err := controller.PlayRequest(request); err != nil {
		t.Fatalf("PlayRequest: %v", err)
	}
	waitState(t, controller, StateLoading)

	engine.emit(Event{Generation: 1, Kind: EventFailed, Err: errors.New("variant a failed")})
	waitSnapshot(t, controller, "second variant", func(s Snapshot) bool { return s.Generation == 2 })
	if got := loadAt(engine, 1).source.URL; got != "https://stream.example/b.m3u8" {
		t.Fatalf("fallback loaded %q, want the second variant", got)
	}
	if quality := controller.Snapshot().Quality; quality.Current != "720p" || !quality.Known {
		t.Fatalf("quality %+v, want the fallback variant reported", quality)
	}
}

func TestQualitySelectionStartsChosenVariant(t *testing.T) {
	controller, engine := newTestController(t)
	request := Request{
		Variants: []Variant{
			{Source: Source{URL: "https://stream.example/a.m3u8"}, Quality: "480p"},
			{Source: Source{URL: "https://stream.example/b.m3u8"}, Quality: "720p"},
		},
		Quality: "720p",
	}
	if err := controller.PlayRequest(request); err != nil {
		t.Fatalf("PlayRequest: %v", err)
	}
	waitState(t, controller, StateLoading)
	if got := loadAt(engine, 0).source.URL; got != "https://stream.example/b.m3u8" {
		t.Fatalf("first load %q, want the selected variant", got)
	}
	snapshot := controller.Snapshot()
	if snapshot.Quality.Selected != "720p" || snapshot.Quality.Current != "720p" || !snapshot.Quality.Known {
		t.Fatalf("quality %+v, want a known 720p selection", snapshot.Quality)
	}
}

func TestUnavailableQualityFallsBackToAuto(t *testing.T) {
	controller, engine := newTestController(t)
	request := Request{
		Variants: []Variant{{Source: Source{URL: "https://stream.example/a.m3u8"}, Quality: "480p"}},
		Quality:  "1080p",
	}
	if err := controller.PlayRequest(request); err != nil {
		t.Fatalf("PlayRequest: %v", err)
	}
	waitState(t, controller, StateLoading)
	if got := loadAt(engine, 0).source.URL; got != "https://stream.example/a.m3u8" {
		t.Fatalf("loaded %q, want the available variant", got)
	}
	quality := controller.Snapshot().Quality
	if quality.Selected != qualityAuto || quality.Fallback || quality.Current != "480p" {
		t.Fatalf("quality %+v, want Auto with the real variant", quality)
	}
}

func TestQualityChangeWhilePausedStaysPaused(t *testing.T) {
	controller, engine := newTestController(t)
	request := Request{
		Variants: []Variant{
			{Source: Source{URL: "https://stream.example/a.m3u8"}, Quality: "480p"},
			{Source: Source{URL: "https://stream.example/b.m3u8"}, Quality: "720p"},
		},
		Quality: "auto",
	}
	if err := controller.PlayRequest(request); err != nil {
		t.Fatalf("PlayRequest: %v", err)
	}
	waitState(t, controller, StateLoading)
	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)
	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitState(t, controller, StatePaused)

	if err := controller.SelectQuality("720p"); err != nil {
		t.Fatalf("SelectQuality: %v", err)
	}
	waitSnapshot(t, controller, "paused quality load", func(s Snapshot) bool { return s.Generation == 2 })
	if state := controller.Snapshot().State; state != StatePaused {
		t.Fatalf("state %s, want paused after a quality change", state)
	}
	if got := loadAt(engine, 1).source.URL; got != "https://stream.example/b.m3u8" {
		t.Fatalf("loaded %q, want the selected variant", got)
	}

	engine.emit(Event{Generation: 2, Kind: EventLoaded})
	waitState(t, controller, StatePaused)
	if calls := pauseCalls(engine); len(calls) != 1 || !calls[0] {
		t.Fatalf("pause calls %v, want the paused load to stay paused", calls)
	}
}

func TestRuntimeFailureDoesNotRetry(t *testing.T) {
	controller, engine := newTestController(t)
	engine.loadErr = errors.Join(ErrRuntimeUnavailable, errors.New("no library"))

	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	snapshot := waitState(t, controller, StateError)
	if !snapshot.Recovery.Terminal || engine.loadCount() != 1 {
		t.Fatalf("recovery %+v loads %d, want one terminal attempt", snapshot.Recovery, engine.loadCount())
	}
	if snapshot.Recovery.Attempt != 1 {
		t.Fatalf("terminal attempt %d, want the real first attempt", snapshot.Recovery.Attempt)
	}
}

func TestPauseCancelsScheduledRecovery(t *testing.T) {
	controller, engine := newTestController(t, WithRetryFloor(time.Hour))
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)

	engine.emit(Event{Generation: 1, Kind: EventFailed, Err: errors.New("drop")})
	waitSnapshot(t, controller, "pending retry", func(s Snapshot) bool { return s.Recovery.Pending })
	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	snapshot := waitState(t, controller, StatePaused)
	if snapshot.Recovery.Pending || snapshot.Recovery.Terminal {
		t.Fatalf("recovery %+v, want cancelled by pause", snapshot.Recovery)
	}
}

func TestEpisodeSurvivesRecoveryAndChangesWithQuality(t *testing.T) {
	controller, engine := newTestController(t, WithRetryFloor(0))
	request := Request{
		Variants: []Variant{
			{Source: Source{URL: "https://stream.example/a.m3u8"}, Quality: "480p"},
			{Source: Source{URL: "https://stream.example/b.m3u8"}, Quality: "720p"},
		},
		Quality: "auto",
	}
	if err := controller.PlayRequest(request); err != nil {
		t.Fatalf("PlayRequest: %v", err)
	}
	waitState(t, controller, StateLoading)
	episode := controller.Snapshot().Episode
	engine.emit(Event{Generation: 1, Kind: EventFailed, Err: errors.New("retry")})
	waitSnapshot(t, controller, "retry", func(s Snapshot) bool { return s.Generation == 2 })
	if got := controller.Snapshot().Episode; got != episode {
		t.Fatalf("episode %d, want %d across one recovery", got, episode)
	}

	engine.emit(Event{Generation: 2, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)
	if err := controller.SelectQuality("720p"); err != nil {
		t.Fatalf("SelectQuality: %v", err)
	}
	waitSnapshot(t, controller, "quality episode", func(s Snapshot) bool { return s.Episode > episode })
}

func TestPauseWithoutFrameIsIgnored(t *testing.T) {
	controller, _ := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if state := controller.Snapshot().State; state != StateLoading {
		t.Fatalf("state %s, want loading without a frame", state)
	}
}

func TestQualityRequiresSession(t *testing.T) {
	controller, _ := newTestController(t)
	if err := controller.SelectQuality("720p"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("SelectQuality error %v, want ErrNoSession", err)
	}
	if err := controller.Pause(); err != nil {
		t.Fatalf("Pause without a session should be a no-op, got %v", err)
	}
	if err := controller.Resume(); err != nil {
		t.Fatalf("Resume without a paused session should be a no-op, got %v", err)
	}
}

func TestCloseDuringRecovery(t *testing.T) {
	controller, engine := newTestController(t, WithRetryFloor(time.Hour))
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)
	engine.emit(Event{Generation: 1, Kind: EventFailed, Err: errors.New("drop")})
	waitSnapshot(t, controller, "pending retry", func(s Snapshot) bool { return s.Recovery.Pending })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
