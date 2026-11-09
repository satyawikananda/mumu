package player

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	intentQueue = 16
	maxDrain    = 64
)

// Player is the UI-facing playback control surface.
type Player interface {
	Play(source Source) error
	Stop() error
	SetVolume(volume int) error
	SetMuted(muted bool) error
	RequestFullscreen()
	Snapshot() Snapshot
	Metrics() Metrics
	Subscribe(fn func(Snapshot)) (unsubscribe func())
	Close(ctx context.Context) error
}

// Controller owns the playback state machine and one Engine.
//
// Intents from the UI are queued and applied by one actor goroutine. State
// reaches subscribers through a notifier goroutine; workers never touch UI
// widgets.
type Controller struct {
	engine Engine
	cfg    config

	intents chan intent
	notify  chan struct{}

	ctx        context.Context
	cancel     context.CancelFunc
	cancelOnce sync.Once
	done       chan struct{}

	mu     sync.RWMutex
	snap   Snapshot
	subs   map[uint64]func(Snapshot)
	nextID uint64

	closeMu  sync.Mutex
	closeErr error

	generation uint64
	loadTimer  *time.Timer
}

type intentKind uint8

const (
	intentPlay intentKind = iota + 1
	intentStop
	intentVolume
	intentMute
)

type intent struct {
	kind   intentKind
	source Source
	volume int
	muted  bool
}

var _ Player = (*Controller)(nil)

// New starts a controller around an already-initialized engine.
func New(engine Engine, opts ...Option) *Controller {
	if engine == nil {
		panic("player: nil engine")
	}

	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	controller := &Controller{
		engine:  engine,
		cfg:     cfg,
		intents: make(chan intent, intentQueue),
		notify:  make(chan struct{}, 1),
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
		subs:    map[uint64]func(Snapshot){},
		snap: Snapshot{
			State:  StateIdle,
			Detail: "Idle",
			Volume: cfg.initialVolume,
			Muted:  cfg.initialMuted,
		},
	}

	go controller.run()
	go controller.runNotifier()

	return controller
}

// Play requests a source. The newest request wins while others are queued.
func (c *Controller) Play(source Source) error {
	if strings.TrimSpace(source.URL) == "" {
		return fmt.Errorf("%w: source URL is empty", ErrInvalid)
	}
	if c.isDone() {
		return ErrClosed
	}
	if c.Snapshot().State == StateStopping {
		return ErrBusy
	}

	return c.post(intent{kind: intentPlay, source: source})
}

// Stop requests playback teardown.
func (c *Controller) Stop() error {
	if c.isDone() {
		return ErrClosed
	}

	return c.post(intent{kind: intentStop})
}

// SetVolume changes volume, clamped to 0..100.
func (c *Controller) SetVolume(volume int) error {
	if c.isDone() {
		return ErrClosed
	}

	return c.post(intent{kind: intentVolume, volume: clampVolume(volume)})
}

// SetMuted changes mute state.
func (c *Controller) SetMuted(muted bool) error {
	if c.isDone() {
		return ErrClosed
	}

	return c.post(intent{kind: intentMute, muted: muted})
}

// RequestFullscreen asks the app to toggle native fullscreen.
func (c *Controller) RequestFullscreen() {
	if c.isDone() {
		return
	}
	if c.cfg.fullscreen != nil {
		c.cfg.fullscreen()
	}
}

// Snapshot returns the current state. It is safe from any goroutine.
func (c *Controller) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.snap
}

// Metrics returns the latest playback telemetry.
func (c *Controller) Metrics() Metrics {
	return c.Snapshot().Metrics
}

// Subscribe registers a state callback. The callback runs on the notifier
// goroutine; it must return quickly and must not touch widgets directly.
func (c *Controller) Subscribe(fn func(Snapshot)) func() {
	if fn == nil {
		return func() {}
	}

	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.subs[id] = fn
	c.mu.Unlock()

	c.notifyLatest()

	var once sync.Once

	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.subs, id)
			c.mu.Unlock()
		})
	}
}

// Close cancels playback and waits for engine teardown bounded by ctx.
func (c *Controller) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	c.cancelOnce.Do(c.cancel)

	select {
	case <-c.done:
		c.closeMu.Lock()
		defer c.closeMu.Unlock()

		return c.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Controller) post(in intent) error {
	select {
	case <-c.done:
		return ErrClosed
	default:
	}

	select {
	case c.intents <- in:
		return nil
	default:
		return ErrBusy
	}
}

func (c *Controller) isDone() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Controller) run() {
	defer close(c.done)
	defer c.shutdownEngine()

	for {
		select {
		case <-c.ctx.Done():
			c.stopLoadTimer()
			c.transition(StateStopping, "Stopping")
			c.engineStop()
			c.transition(StateStopped, "Stopped")

			return
		case first := <-c.intents:
			c.drainIntents(first)
		case <-c.loadTimerC():
			c.handleLoadTimeout()
		case event, ok := <-c.engine.Events():
			if !ok {
				c.handleEvent(Event{Kind: EventShutdown})

				return
			}
			c.handleEvent(event)
		}
	}
}

func (c *Controller) drainIntents(first intent) {
	pending := []intent{first}
	for len(pending) < maxDrain {
		select {
		case next := <-c.intents:
			pending = append(pending, next)
		default:
			c.applyIntents(pending)

			return
		}
	}

	c.applyIntents(pending)
}

func (c *Controller) applyIntents(pending []intent) {
	lastPlay := -1
	for i := range pending {
		if pending[i].kind == intentPlay {
			lastPlay = i
		}
	}

	for i, in := range pending {
		if in.kind == intentPlay && i != lastPlay {
			continue
		}
		c.handleIntent(in)
	}
}

func (c *Controller) handleIntent(in intent) {
	switch in.kind {
	case intentPlay:
		c.handlePlay(in.source)
	case intentStop:
		c.handleStop()
	case intentVolume:
		c.handleVolume(in.volume)
	case intentMute:
		c.handleMute(in.muted)
	}
}

func (c *Controller) handlePlay(source Source) {
	if c.Snapshot().State == StateStopping {
		return
	}

	c.stopLoadTimer()
	c.generation++
	generation := c.generation

	c.update(func(s *Snapshot) {
		s.State = StateLoading
		s.Generation = generation
		s.Source = source
		s.Detail = "Loading"
		s.Error = ""
		s.Metadata = Metadata{}
		s.Capabilities = Capabilities{}
		s.Metrics = Metrics{Generation: generation}
	})
	c.resetLoadTimer()

	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	if err := c.engine.Load(ctx, generation, source); err != nil {
		c.stopLoadTimer()
		c.fail(generation, fmt.Errorf("load source: %w", err))
	}
}

func (c *Controller) handleStop() {
	state := c.Snapshot().State
	if state == StateIdle || state == StateStopped {
		c.transition(StateStopped, "Stopped")

		return
	}

	c.generation++
	c.stopLoadTimer()
	c.transition(StateStopping, "Stopping")
	c.engineStop()
	c.transition(StateStopped, "Stopped")
}

func (c *Controller) handleVolume(volume int) {
	volume = clampVolume(volume)
	c.update(func(s *Snapshot) { s.Volume = volume })

	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	if err := c.engine.SetVolume(ctx, volume); err != nil && !errors.Is(err, ErrClosed) {
		c.cfg.logger.Debug("player volume", "error", err)
	}
}

func (c *Controller) handleMute(muted bool) {
	c.update(func(s *Snapshot) { s.Muted = muted })

	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	if err := c.engine.SetMuted(ctx, muted); err != nil && !errors.Is(err, ErrClosed) {
		c.cfg.logger.Debug("player mute", "error", err)
	}
}

func (c *Controller) handleLoadTimeout() {
	snapshot := c.Snapshot()
	if snapshot.State != StateLoading {
		return
	}

	c.stopLoadTimer()
	c.fail(snapshot.Generation, errors.New("stream did not start in time"))
	c.engineStop()
}

func (c *Controller) handleEvent(event Event) {
	if event.Generation != 0 && event.Generation != c.generation {
		return
	}

	switch event.Kind {
	case EventLoaded:
		if c.Snapshot().State == StateLoading {
			c.update(func(s *Snapshot) { s.Detail = "Loaded" })
		}
	case EventStarted:
		if c.Snapshot().State == StateLoading {
			c.update(func(s *Snapshot) { s.Detail = "Starting" })
		}
	case EventPlaying:
		c.stopLoadTimer()
		state := c.Snapshot().State
		if state == StateLoading || state == StatePlaying || state == StateBuffering {
			c.transition(StatePlaying, "Playing")
		}
	case EventBuffering:
		state := c.Snapshot().State
		switch {
		case event.Buffering && state == StatePlaying:
			c.transition(StateBuffering, "Buffering")
		case !event.Buffering && state == StateBuffering:
			c.transition(StatePlaying, "Playing")
		}
	case EventEnded:
		c.stopLoadTimer()
		if event.Err != nil {
			c.fail(event.Generation, event.Err)
		} else if c.Snapshot().State != StateError {
			c.transition(StateStopped, "Playback ended")
		}
	case EventFailed:
		c.stopLoadTimer()
		if event.Err != nil {
			c.fail(event.Generation, event.Err)
		} else {
			c.fail(event.Generation, errors.New(event.Detail))
		}
	case EventMetrics:
		c.applyMetrics(event)
	case EventShutdown:
		if c.ctx.Err() == nil {
			c.fail(c.generation, errors.New("player stopped unexpectedly"))
		}
	}
}

func (c *Controller) applyMetrics(event Event) {
	if event.Metrics.Generation != 0 && event.Metrics.Generation != c.generation {
		return
	}

	c.update(func(s *Snapshot) {
		s.Metrics = event.Metrics
		if event.Metadata != (Metadata{}) {
			s.Metadata = event.Metadata
		}
		if event.Capabilities != (Capabilities{}) {
			s.Capabilities = event.Capabilities
		}

		switch {
		case s.State == StateLoading && event.Metrics.TimePos > 0 && event.Metrics.PausedForCache:
			s.State = StateBuffering
			s.Detail = "Buffering"
			s.Error = ""
		case s.State == StateLoading && event.Metrics.TimePos > 0:
			s.State = StatePlaying
			s.Detail = "Playing"
			s.Error = ""
		case s.State == StatePlaying && event.Metrics.PausedForCache:
			s.State = StateBuffering
			s.Detail = "Buffering"
		case s.State == StateBuffering && !event.Metrics.PausedForCache:
			s.State = StatePlaying
			s.Detail = "Playing"
		}
	})

	if event.Metrics.TimePos > 0 {
		c.stopLoadTimer()
	}
}

func (c *Controller) transition(state State, detail string) {
	c.update(func(s *Snapshot) {
		s.State = state
		s.Detail = detail
		if state != StateError {
			s.Error = ""
		}
	})
}

func (c *Controller) fail(generation uint64, err error) {
	if err == nil {
		return
	}

	detail := sanitizeDetail(err.Error())
	c.cfg.logger.Debug("player failure", "generation", generation, "detail", detail)

	c.update(func(s *Snapshot) {
		if s.Generation != generation {
			return
		}
		s.State = StateError
		s.Detail = detail
		s.Error = detail
	})
}

func (c *Controller) update(mutate func(*Snapshot)) {
	c.mu.Lock()
	before := c.snap
	mutate(&c.snap)
	after := c.snap
	c.mu.Unlock()

	if before != after {
		c.notifyLatest()
	}
}

func (c *Controller) notifyLatest() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func (c *Controller) runNotifier() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.notify:
			snapshot := c.Snapshot()
			for _, fn := range c.subscribers() {
				c.callSubscriber(fn, snapshot)
			}
		}
	}
}

func (c *Controller) callSubscriber(fn func(Snapshot), snapshot Snapshot) {
	defer func() {
		if recover() != nil {
			// Do not log the panic value or snapshot: both may contain
			// user-supplied source data.
			c.cfg.logger.Error("player subscriber panic")
		}
	}()

	fn(snapshot)
}

func (c *Controller) subscribers() []func(Snapshot) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	subs := make([]func(Snapshot), 0, len(c.subs))
	for _, fn := range c.subs {
		subs = append(subs, fn)
	}

	return subs
}

func (c *Controller) engineStop() {
	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	err := c.engine.Stop(ctx)
	if err != nil &&
		!errors.Is(err, ErrClosed) &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		c.cfg.logger.Debug("player stop", "error", err)
	}
}

func (c *Controller) shutdownEngine() {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.closeTimeout)
	defer cancel()

	if err := c.engine.Close(ctx); err != nil && !errors.Is(err, ErrClosed) {
		c.closeMu.Lock()
		c.closeErr = err
		c.closeMu.Unlock()
		c.cfg.logger.Warn("player engine close", "error", err)
	}
}

func (c *Controller) resetLoadTimer() {
	if c.cfg.loadingTimeout <= 0 {
		return
	}

	if c.loadTimer == nil {
		c.loadTimer = time.NewTimer(c.cfg.loadingTimeout)

		return
	}

	if !c.loadTimer.Stop() {
		select {
		case <-c.loadTimer.C:
		default:
		}
	}
	c.loadTimer.Reset(c.cfg.loadingTimeout)
}

func (c *Controller) stopLoadTimer() {
	if c.loadTimer == nil {
		return
	}

	if !c.loadTimer.Stop() {
		select {
		case <-c.loadTimer.C:
		default:
		}
	}
}

func (c *Controller) loadTimerC() <-chan time.Time {
	if c.loadTimer == nil {
		return nil
	}

	return c.loadTimer.C
}

func clampVolume(volume int) int {
	return min(max(volume, 0), 100)
}

var streamURLPattern = regexp.MustCompile(`(?i)\bhttps?://[^\s]+`)

func sanitizeDetail(message string) string {
	message = streamURLPattern.ReplaceAllString(message, "[stream]")
	message = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, message)
	message = strings.TrimSpace(message)

	runes := []rune(message)
	if len(runes) > 400 {
		message = string(runes[:400]) + "..."
	}
	if message == "" {
		return "playback failed"
	}

	return message
}
