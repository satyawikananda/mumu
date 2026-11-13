package player

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	PlayRequest(request Request) error
	Pause() error
	Resume() error
	SelectQuality(label string) error
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
	episode    uint64

	// request is the current episode's variants and quality selection. order
	// is the attempt order, selected is "auto" or the resolved label.
	request         Request
	order           []Variant
	selectedQuality string
	attempt         int
	attemptStarted  time.Time
	lastProgress    time.Duration

	// paused is the viewer's local pause intent; enginePaused tracks the
	// native property, which can outlive one attempt. hasFrame reports that a
	// real frame reached the screen, which decides whether recovery can keep
	// showing it.
	paused       bool
	enginePaused bool
	// unpauseOnLoad resumes a paused frame after a reload, for Resume, a new
	// channel or a quality change while playing.
	unpauseOnLoad bool
	hasFrame      bool
	loading       bool
	retryPending  bool
	stallTimer    *time.Timer
	retryTimer    *time.Timer
	retryTick     *time.Timer
}

type intentKind uint8

const (
	intentPlay intentKind = iota + 1
	intentStop
	intentVolume
	intentMute
	intentPause
	intentResume
	intentQuality
)

type intent struct {
	kind    intentKind
	request Request
	volume  int
	muted   bool
	quality string
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
			State:   StateIdle,
			Detail:  "Idle",
			Volume:  cfg.initialVolume,
			Muted:   cfg.initialMuted,
			Quality: Quality{Selected: qualityAuto},
		},
	}

	go controller.run()
	go controller.runNotifier()

	return controller
}

// Play requests one source. It is PlayRequest with a single Auto variant.
//
// Play rejects an empty URL and any scheme other than http or https; a
// private host or a header is a source-policy decision made before this call.
func (c *Controller) Play(source Source) error {
	return c.PlayRequest(Request{Variants: []Variant{{Source: source}}, Quality: qualityAuto})
}

// PlayRequest requests one channel as an ordered variant list and a quality
// selection. The newest request wins while others are queued.
func (c *Controller) PlayRequest(request Request) error {
	request, err := normalizeRequest(request)
	if err != nil {
		return err
	}
	if c.isDone() {
		return ErrClosed
	}
	if c.Snapshot().State == StateStopping {
		return ErrBusy
	}

	return c.post(intent{kind: intentPlay, request: request})
}

// Pause holds the displayed frame and stops audio and read-ahead.
func (c *Controller) Pause() error {
	if c.isDone() {
		return ErrClosed
	}

	state := c.Snapshot().State
	if state == StatePaused || (state != StatePlaying && state != StateBuffering) {
		return nil
	}

	return c.post(intent{kind: intentPause})
}

// Resume reconnects a paused episode at the live edge. It never seeks or
// continues from the paused position.
func (c *Controller) Resume() error {
	if c.isDone() {
		return ErrClosed
	}
	if c.Snapshot().State != StatePaused {
		return nil
	}

	return c.post(intent{kind: intentResume})
}

// SelectQuality switches the current episode to another declared variant. The
// label is an exact playlist label; an unavailable label falls back to Auto.
func (c *Controller) SelectQuality(label string) error {
	if c.isDone() {
		return ErrClosed
	}

	state := c.Snapshot().State
	if state == StateStopping || !state.Active() {
		return ErrNoSession
	}

	return c.post(intent{kind: intentQuality, quality: strings.TrimSpace(label)})
}

// sourceScheme returns the validated scheme of source. The URL own scheme
// wins, and a declared Scheme must agree with it.
func sourceScheme(source Source) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(source.URL))
	if err != nil {
		return "", fmt.Errorf("%w: source URL is invalid", ErrInvalid)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !isHTTPScheme(scheme) {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedScheme, scheme)
	}

	declared := strings.ToLower(strings.TrimSpace(source.Scheme))
	if declared == "" {
		return scheme, nil
	}
	if !isHTTPScheme(declared) {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedScheme, declared)
	}
	if declared != scheme {
		return "", fmt.Errorf("%w: %q does not match the URL", ErrInvalid, declared)
	}

	return scheme, nil
}

func isHTTPScheme(scheme string) bool {
	return scheme == "http" || scheme == "https"
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
			c.stopTimers()
			c.cancelRecovery()
			c.transition(StateStopping, "Stopping")
			c.engineStop()
			c.transition(StateStopped, "Stopped")

			return
		case first := <-c.intents:
			c.drainIntents(first)
		case <-c.stallTimerC():
			c.handleStallTimeout()
		case <-c.retryTimerC():
			c.handleRetryTimer()
		case <-c.retryTickC():
			c.handleRetryTick()
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
	lastQuality := -1
	for i := range pending {
		switch pending[i].kind {
		case intentPlay:
			lastPlay = i
		case intentQuality:
			lastQuality = i
		}
	}

	for i, in := range pending {
		if in.kind == intentPlay && i != lastPlay {
			continue
		}
		if in.kind == intentQuality && i != lastQuality {
			continue
		}
		c.handleIntent(in)
	}
}

func (c *Controller) handleIntent(in intent) {
	switch in.kind {
	case intentPlay:
		c.handlePlayRequest(in.request)
	case intentStop:
		c.handleStop()
	case intentVolume:
		c.handleVolume(in.volume)
	case intentMute:
		c.handleMute(in.muted)
	case intentPause:
		c.handlePause()
	case intentResume:
		c.handleResume()
	case intentQuality:
		c.handleQuality(in.quality)
	}
}

func (c *Controller) handleStop() {
	state := c.Snapshot().State
	if state == StateIdle || state == StateStopped {
		c.transition(StateStopped, "Stopped")

		return
	}

	c.cancelRecovery()
	c.stopTimers()
	c.generation++
	c.paused = false
	c.enginePaused = false
	c.unpauseOnLoad = false
	c.hasFrame = false
	c.loading = false
	c.attempt = 0
	c.lastProgress = 0
	c.transition(StateStopping, "Stopping")
	c.engineStop()
	c.update(func(s *Snapshot) {
		s.State = StateStopped
		s.Detail = "Stopped"
		s.Recovery = Recovery{}
	})
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

func (c *Controller) handleEvent(event Event) {
	if event.Generation != 0 && event.Generation != c.generation {
		return
	}

	switch event.Kind {
	case EventLoaded:
		c.loading = false
		c.unpauseAfterLoad()
	case EventStarted:
		if c.Snapshot().State == StateLoading {
			c.update(func(s *Snapshot) { s.Detail = "Starting" })
		}
	case EventPlaying:
		if c.paused {
			return
		}
		c.loading = false
		c.hasFrame = true
		c.attempt = 1
		state := c.Snapshot().State
		if state == StateLoading || state == StatePlaying || state == StateBuffering {
			c.transition(StatePlaying, "Playing")
			c.clearRecovery()
		}
	case EventBuffering:
		if c.paused {
			return
		}
		state := c.Snapshot().State
		switch {
		case event.Buffering && state == StatePlaying:
			c.transition(StateBuffering, "Buffering")
		case !event.Buffering && state == StateBuffering:
			c.transition(StatePlaying, "Playing")
			c.clearRecovery()
		}
	case EventEnded:
		if c.paused {
			return
		}
		if event.Err != nil {
			c.attemptFailed(event.Generation, event.Err)
		} else if c.Snapshot().State != StateError {
			c.stopTimers()
			c.transition(StateStopped, "Playback ended")
		}
	case EventFailed:
		if c.paused && !c.loading {
			return
		}
		if event.Err != nil {
			c.attemptFailed(event.Generation, event.Err)
		} else {
			c.attemptFailed(event.Generation, errors.New(event.Detail))
		}
	case EventMetrics:
		c.applyMetrics(event)
	case EventShutdown:
		if c.ctx.Err() == nil {
			c.fail(c.generation, errors.New("player stopped unexpectedly"))
		}
	}
}

// unpauseAfterLoad releases a paused engine once the replacement file is
// loaded, so a new channel or a quality switch never plays the old buffer.
func (c *Controller) unpauseAfterLoad() {
	if !c.unpauseOnLoad || c.paused || !c.enginePaused {
		return
	}

	c.unpauseOnLoad = false
	c.enginePaused = false

	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	if err := c.engine.SetPaused(ctx, false); err != nil && !errors.Is(err, ErrClosed) {
		c.cfg.logger.Debug("player resume", "error", err)
	}
}

func (c *Controller) applyMetrics(event Event) {
	if event.Metrics.Generation != 0 && event.Metrics.Generation != c.generation {
		return
	}

	progressed := event.Metrics.TimePos > c.lastProgress
	c.update(func(s *Snapshot) {
		s.Metrics = event.Metrics
		if event.Metadata != (Metadata{}) {
			s.Metadata = event.Metadata
		}
		if event.Capabilities != (Capabilities{}) {
			s.Capabilities = event.Capabilities
		}
		if c.paused || s.Recovery.Terminal {
			return
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
			s.Recovery = Recovery{}
		case s.State == StatePlaying && event.Metrics.PausedForCache:
			s.State = StateBuffering
			s.Detail = "Buffering"
		case s.State == StateBuffering && !event.Metrics.PausedForCache:
			s.State = StatePlaying
			s.Detail = "Playing"
			s.Recovery = Recovery{}
		}
	})

	if progressed && !c.paused && c.Snapshot().State != StateError {
		c.lastProgress = event.Metrics.TimePos
		c.hasFrame = true
		c.armStallTimer()
	}
	if !c.paused && c.Snapshot().State == StatePlaying && c.attempt != 1 {
		c.attempt = 1
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

func (c *Controller) update(mutate func(*Snapshot)) {
	c.mu.Lock()
	before := c.snap
	mutate(&c.snap)
	after := c.snap
	c.mu.Unlock()

	if !before.equal(after) {
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
