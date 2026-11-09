package player

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type loadCall struct {
	generation uint64
	source     Source
}

type fakeEngine struct {
	mu sync.Mutex

	loads      []loadCall
	stops      int
	volumes    []int
	mutes      []bool
	closeCalls int
	loadErr    error
	blockLoad  chan struct{}
	loadStart  chan struct{}

	events   chan Event
	closeOne sync.Once
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{
		events: make(chan Event, 64),
	}
}

func (f *fakeEngine) Load(ctx context.Context, generation uint64, source Source) error {
	f.mu.Lock()
	f.loads = append(f.loads, loadCall{generation: generation, source: source})
	block := f.blockLoad
	started := f.loadStart
	f.loadStart = nil
	err := f.loadErr
	f.mu.Unlock()

	if started != nil {
		close(started)
	}

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return err
}

func (f *fakeEngine) Stop(context.Context) error {
	f.mu.Lock()
	f.stops++
	f.mu.Unlock()

	return nil
}

func (f *fakeEngine) SetVolume(_ context.Context, volume int) error {
	f.mu.Lock()
	f.volumes = append(f.volumes, volume)
	f.mu.Unlock()

	return nil
}

func (f *fakeEngine) SetMuted(_ context.Context, muted bool) error {
	f.mu.Lock()
	f.mutes = append(f.mutes, muted)
	f.mu.Unlock()

	return nil
}

func (f *fakeEngine) Events() <-chan Event {
	return f.events
}

func (f *fakeEngine) Close(context.Context) error {
	f.closeOne.Do(func() {
		f.mu.Lock()
		f.closeCalls++
		f.mu.Unlock()
		close(f.events)
	})

	return nil
}

func (f *fakeEngine) setBlockLoad(block chan struct{}) {
	f.mu.Lock()
	f.blockLoad = block
	f.mu.Unlock()
}

func (f *fakeEngine) setLoadStart(start chan struct{}) {
	f.mu.Lock()
	f.loadStart = start
	f.mu.Unlock()
}

func (f *fakeEngine) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closeCalls
}

func (f *fakeEngine) emit(event Event) {
	f.events <- event
}

func (f *fakeEngine) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.loads)
}

func (f *fakeEngine) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.stops
}

func (f *fakeEngine) lastVolume() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.volumes) == 0 {
		return -1
	}

	return f.volumes[len(f.volumes)-1]
}

func (f *fakeEngine) lastMute() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.mutes) == 0 {
		return false
	}

	return f.mutes[len(f.mutes)-1]
}

func newTestController(t *testing.T, opts ...Option) (*Controller, *fakeEngine) {
	t.Helper()

	engine := newFakeEngine()
	controller := New(engine, opts...)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = controller.Close(ctx)
	})

	return controller, engine
}

func waitState(t *testing.T, controller *Controller, want State) Snapshot {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		if snapshot.State == want {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}

	snapshot := controller.Snapshot()
	t.Fatalf("state = %s (%s), want %s", snapshot.State, snapshot.Detail, want)

	return snapshot
}

func waitSnapshot(t *testing.T, controller *Controller, label string, done func(Snapshot) bool) Snapshot {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		if done(snapshot) {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s; last snapshot %+v", label, controller.Snapshot())

	return Snapshot{}
}

func TestPlayTransitions(t *testing.T) {
	controller, engine := newTestController(t)

	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)

	if got := engine.loadCount(); got != 1 {
		t.Fatalf("loads %d, want 1", got)
	}
}

func TestBuffering(t *testing.T) {
	controller, engine := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)

	engine.emit(Event{Generation: 1, Kind: EventBuffering, Buffering: true})
	waitState(t, controller, StateBuffering)

	engine.emit(Event{Generation: 1, Kind: EventBuffering, Buffering: false})
	waitState(t, controller, StatePlaying)
}

func TestEndErrorSanitizesURL(t *testing.T) {
	controller, engine := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	engine.emit(Event{
		Generation: 1,
		Kind:       EventEnded,
		Err:        errors.New("failed https://user:pass@example.com/private.m3u8"),
	})

	snapshot := waitState(t, controller, StateError)
	if strings.Contains(snapshot.Error, "pass") || strings.Contains(snapshot.Error, "https://") {
		t.Fatalf("error leaked credentials: %q", snapshot.Error)
	}
	if !strings.Contains(snapshot.Error, "[stream]") {
		t.Fatalf("error %q missing redacted URL marker", snapshot.Error)
	}
}

func TestRapidSwitchIgnoresStaleEvents(t *testing.T) {
	controller, engine := newTestController(t)

	if err := controller.Play(Source{URL: "https://stream.example/a.m3u8"}); err != nil {
		t.Fatalf("Play A: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for engine.loadCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if err := controller.Play(Source{URL: "https://stream.example/b.m3u8"}); err != nil {
		t.Fatalf("Play B: %v", err)
	}
	for engine.loadCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if engine.loadCount() != 2 {
		t.Fatalf("loads %d, want 2", engine.loadCount())
	}

	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	time.Sleep(20 * time.Millisecond)
	if got := controller.Snapshot().State; got == StatePlaying {
		t.Fatal("stale generation changed state to playing")
	}

	engine.emit(Event{Generation: 2, Kind: EventPlaying})
	snapshot := waitState(t, controller, StatePlaying)
	if snapshot.Source.URL != "https://stream.example/b.m3u8" {
		t.Fatalf("source %q, want newest", snapshot.Source.URL)
	}
}

func TestStopFromPlaying(t *testing.T) {
	controller, engine := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)

	if err := controller.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitState(t, controller, StateStopped)

	// A late event from the stopped load must not revive playback.
	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	time.Sleep(20 * time.Millisecond)
	if got := controller.Snapshot().State; got != StateStopped {
		t.Fatalf("state %s after late event, want stopped", got)
	}
	if engine.stopCount() == 0 {
		t.Fatal("engine Stop not called")
	}
}

func TestStopDuringLoading(t *testing.T) {
	controller, _ := newTestController(t)
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	if err := controller.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitState(t, controller, StateStopped)
}

func TestLoadingTimeout(t *testing.T) {
	controller, _ := newTestController(t, WithLoadingTimeout(20*time.Millisecond))
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}

	snapshot := waitState(t, controller, StateError)
	if !strings.Contains(snapshot.Error, "did not start") {
		t.Fatalf("error %q, want timeout", snapshot.Error)
	}
}

func TestVolumeAndMute(t *testing.T) {
	controller, engine := newTestController(t)

	if err := controller.SetVolume(-20); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
	waitSnapshot(t, controller, "volume 0", func(snapshot Snapshot) bool {
		return snapshot.Volume == 0
	})
	if got := engine.lastVolume(); got != 0 {
		t.Fatalf("engine volume %d, want 0", got)
	}

	if err := controller.SetVolume(150); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
	waitSnapshot(t, controller, "volume 100", func(snapshot Snapshot) bool {
		return snapshot.Volume == 100
	})

	if err := controller.SetMuted(true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	waitSnapshot(t, controller, "mute", func(snapshot Snapshot) bool {
		return snapshot.Muted
	})
	if !engine.lastMute() {
		t.Fatal("mute not applied")
	}
}

func TestSubscribeDeliversState(t *testing.T) {
	controller, engine := newTestController(t)

	updates := make(chan Snapshot, 8)
	unsubscribe := controller.Subscribe(func(snapshot Snapshot) {
		select {
		case updates <- snapshot:
		default:
		}
	})
	defer unsubscribe()

	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	select {
	case snapshot := <-updates:
		if snapshot.State != StateIdle && snapshot.State != StateLoading {
			t.Fatalf("first snapshot state %s, want idle or loading", snapshot.State)
		}
	case <-time.After(time.Second):
		t.Fatal("no snapshot delivered")
	}

	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	deadline := time.After(time.Second)
	for {
		select {
		case snapshot := <-updates:
			if snapshot.State == StatePlaying {
				return
			}
		case <-deadline:
			t.Fatal("playing snapshot not delivered")
		}
	}
}

func TestSubscriberPanicDoesNotStopNotifier(t *testing.T) {
	controller, engine := newTestController(t)
	controller.Subscribe(func(Snapshot) { panic("boom") })

	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	waitState(t, controller, StateLoading)

	engine.emit(Event{Generation: 1, Kind: EventPlaying})
	waitState(t, controller, StatePlaying)
}

func TestRequestFullscreen(t *testing.T) {
	called := make(chan struct{}, 1)
	controller, _ := newTestController(t, WithFullscreenHandler(func() {
		called <- struct{}{}
	}))

	controller.RequestFullscreen()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("fullscreen handler not called")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	controller, engine := newTestController(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.Close(ctx); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := controller.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if engine.closeCount() != 1 {
		t.Fatalf("engine close calls %d, want 1", engine.closeCount())
	}
	if err := controller.Play(Source{URL: "https://stream.example/live.m3u8"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Play after Close error %v, want ErrClosed", err)
	}
}

func TestQueueIsBounded(t *testing.T) {
	controller, engine := newTestController(t)
	block := make(chan struct{})
	started := make(chan struct{})
	engine.setBlockLoad(block)
	engine.setLoadStart(started)

	if err := controller.Play(Source{URL: "https://stream.example/first.m3u8"}); err != nil {
		t.Fatalf("first Play: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("engine load did not start")
	}

	var busy bool
	for range intentQueue + 2 {
		err := controller.Play(Source{URL: "https://stream.example/next.m3u8"})
		if errors.Is(err, ErrBusy) {
			busy = true

			break
		}
	}
	if !busy {
		t.Fatal("intent queue never reported busy")
	}

	engine.setBlockLoad(nil)
	close(block)
}

func TestConcurrentAccess(t *testing.T) {
	controller, _ := newTestController(t)

	var group sync.WaitGroup
	for i := range 10 {
		group.Go(func() {
			switch i % 4 {
			case 0:
				_ = controller.Play(Source{URL: "https://stream.example/live.m3u8"})
			case 1:
				_ = controller.Stop()
			case 2:
				_ = controller.SetVolume(i * 10)
			case 3:
				_ = controller.SetMuted(i%2 == 0)
			}
		})
	}
	group.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestStateHelpers(t *testing.T) {
	if !StateIdle.CanPlay() || !StateError.CanPlay() || !StateStopped.CanPlay() {
		t.Fatal("idle, error and stopped should allow Play")
	}
	if StateLoading.CanPlay() || StatePlaying.CanPlay() {
		t.Fatal("loading and playing should not start a new Play without switching")
	}
	if StateUnknown.String() != "unknown" {
		t.Fatalf("unknown string %q", StateUnknown.String())
	}
}
