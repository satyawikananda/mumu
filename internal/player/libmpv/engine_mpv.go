//go:build darwin && arm64

// Package libmpv implements the player engine with one libmpv client.
//
// The package opens the libmpv runtime on the first New, never in a package
// init: a machine without mpv must get an error, not a panic, and a build
// without a runtime must still start. Import it only from the composition
// root, so packages whose tests never touch native code stay clean.
package libmpv

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	mpv "github.com/gen2brain/go-mpv"

	"mumu/internal/player"
	"mumu/internal/player/libmpv/library"
)

type commandKind uint8

const (
	commandLoad commandKind = iota + 1
	commandStop
	commandVolume
	commandMute
	commandPause
	commandReadAhead
)

// bundleDir is where a packaged app may carry libmpv. The search only uses it
// when it already exists; packaging and redistribution live in packaging/macos.
const bundleDir = "Resources"

// bundledLibraryDir is the subdirectory of bundleDir that ships the LGPL
// libmpv runtime and its dependencies. Keeping them together leaves
// Contents/Resources root for the app's icons and notices.
const bundledLibraryDir = "libmpv"

type command struct {
	kind       commandKind
	generation uint64
	source     player.Source
	volume     int
	muted      bool
	paused     bool
	readAhead  int
	done       chan error
}

// Engine is a single libmpv client with a command queue and one event pump.
type Engine struct {
	client *mpv.Mpv
	cfg    Config

	commands chan command
	events   chan player.Event

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	closeOnce sync.Once

	renderMu sync.Mutex
	render   *glRender

	mu                sync.Mutex
	pendingGeneration uint64
	activeGeneration  uint64
	activeEntry       int64

	// paused and readAhead belong to the pump goroutine. readAhead keeps the
	// value the runtime had before a pause bounded it to zero, so unpause
	// restores the same look-ahead instead of guessing a default.
	paused    bool
	readAhead string
}

var _ player.Engine = (*Engine)(nil)

// New creates and initializes one libmpv client. The caller must close it.
//
// It fails with an error wrapping player.ErrRuntimeUnavailable when this
// machine has no usable libmpv; the runtime is searched on every attempt, so a
// viewer who installs mpv can retry without restarting the app.
func New(cfg Config) (*Engine, error) {
	cfg = cfg.withDefaults()

	if err := openRuntime(); err != nil {
		return nil, err
	}

	client := mpv.New()
	if client == nil {
		return nil, fmt.Errorf("%w: create libmpv client", player.ErrRuntimeUnavailable)
	}

	options := map[string]string{
		"vo":                     cfg.VideoOutput,
		"hwdec":                  cfg.HardwareDecoding,
		"config":                 "no",
		"load-scripts":           "no",
		"ytdl":                   "no",
		"input-default-bindings": "no",
		"input-terminal":         "no",
		"terminal":               "no",
		"idle":                   "yes",
		"keep-open":              "no",
		"volume":                 strconv.Itoa(cfg.Volume),
		"mute":                   yesNo(cfg.Muted),
		"network-timeout":        strconv.Itoa(int(cfg.NetworkTimeout.Seconds())),
		"demuxer-max-bytes":      cfg.DemuxerMaxBytes,
	}
	if cfg.AudioOutput != "" {
		options["ao"] = cfg.AudioOutput
	}

	// A build without Lua, JavaScript or C plugins does not register these
	// scripting options at all. They are still set when present, so a runtime
	// that has scripting never auto-loads a script.
	optional := map[string]bool{
		"load-scripts": true,
		"ytdl":         true,
	}

	for name, value := range options {
		if err := client.SetOptionString(name, value); err != nil {
			if optional[name] {
				continue
			}
			client.TerminateDestroy()

			return nil, fmt.Errorf("set libmpv option %s: %w", name, err)
		}
	}
	if err := client.Initialize(); err != nil {
		client.TerminateDestroy()

		return nil, fmt.Errorf("initialize libmpv: %w", err)
	}

	// The actual values handed to mpv, so a silent launch can be traced to the
	// engine rather than guessed from the UI state.
	slog.Debug("libmpv audio options", "volume", cfg.Volume, "muted", cfg.Muted)

	ctx, cancel := context.WithCancel(context.Background())
	engine := &Engine{
		client:   client,
		cfg:      cfg,
		commands: make(chan command, 16),
		events:   make(chan player.Event, 32),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	go engine.run()

	return engine, nil
}

// openRuntime opens libmpv from the first location that works: the dynamic
// loader's own search path (which honours DYLD_LIBRARY_PATH), the Homebrew
// prefixes, then the app bundle. A failure reports the names it tried, never a
// full path, so a log line cannot reveal where the app was launched from.
func openRuntime() error {
	candidate, err := library.Resolve(library.Search{
		Override: library.FromEnv(os.Getenv),
		Bundle:   bundleDirs(),
		Open: func(candidate string) error {
			return mpv.Load([]string{candidate})
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", player.ErrRuntimeUnavailable, err)
	}
	slog.Debug("libmpv runtime ready", "library", filepath.Base(candidate))

	return nil
}

// bundleDirs returns the directories a packaged app may carry libmpv in.
func bundleDirs() []string {
	executable, err := os.Executable()
	if err != nil {
		return nil
	}

	resources := filepath.Join(filepath.Dir(executable), "..", bundleDir)

	return []string{filepath.Join(resources, bundledLibraryDir), resources}
}

// Load starts a source for generation. The actual result arrives as events.
func (e *Engine) Load(ctx context.Context, generation uint64, source player.Source) error {
	return e.call(ctx, command{
		kind:       commandLoad,
		generation: generation,
		source:     source,
	})
}

// Stop stops the current source.
func (e *Engine) Stop(ctx context.Context) error {
	return e.call(ctx, command{kind: commandStop})
}

// SetVolume sets the mpv volume property.
func (e *Engine) SetVolume(ctx context.Context, volume int) error {
	return e.call(ctx, command{kind: commandVolume, volume: volume})
}

// SetMuted sets the mpv mute property.
func (e *Engine) SetMuted(ctx context.Context, muted bool) error {
	return e.call(ctx, command{kind: commandMute, muted: muted})
}

// SetPaused pauses or resumes playback without reloading the source.
//
// Pause holds the frame the render thread last presented, stops audio and asks
// the demuxer to stop reading ahead. Live streams still refresh their playlist
// and finish in-flight reads, so the network is bounded, not silent.
func (e *Engine) SetPaused(ctx context.Context, paused bool) error {
	return e.call(ctx, command{kind: commandPause, paused: paused})
}

// SetReadAhead bounds how far the demuxer reads ahead, in seconds. The spike
// uses zero while paused: pause alone holds the frame and stops audio, but the
// demuxer keeps filling its cache from the live stream. It is an experiment
// seam for #37, not a product setting yet.
func (e *Engine) SetReadAhead(ctx context.Context, seconds int) error {
	if seconds < 0 {
		return fmt.Errorf("%w: read-ahead cannot be negative", player.ErrInvalid)
	}

	return e.call(ctx, command{kind: commandReadAhead, readAhead: seconds})
}

// Events returns the event stream. It closes when the engine closes.
func (e *Engine) Events() <-chan player.Event {
	return e.events
}

// Close stops the pump and destroys the client. It is idempotent.
func (e *Engine) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	e.closeOnce.Do(e.cancel)

	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) call(ctx context.Context, cmd command) error {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-e.done:
		return player.ErrClosed
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	cmd.done = make(chan error, 1)
	select {
	case e.commands <- cmd:
	case <-e.done:
		return player.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-cmd.done:
		return err
	case <-e.done:
		return player.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) run() {
	defer close(e.done)
	defer close(e.events)
	defer e.client.TerminateDestroy()

	ticker := time.NewTicker(e.cfg.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case cmd := <-e.commands:
			e.handleCommand(cmd)
		case <-ticker.C:
			e.publishMetrics()
		default:
			event := e.client.WaitEvent(0.05)
			if event != nil && event.EventID != mpv.EventNone {
				e.handleEvent(event)
			}
		}
	}
}

func (e *Engine) handleCommand(cmd command) {
	switch cmd.kind {
	case commandLoad:
		e.mu.Lock()
		e.pendingGeneration = cmd.generation
		e.activeGeneration = 0
		e.activeEntry = 0
		e.mu.Unlock()

		// Forward the per-source playback headers immediately before the load
		// so a previous source cannot leak into this request.
		if err := e.setPlaybackHeaders(cmd.source.Headers); err != nil {
			e.mu.Lock()
			e.pendingGeneration = 0
			e.activeGeneration = 0
			e.activeEntry = 0
			e.mu.Unlock()

			err = fmt.Errorf("set playback headers: %w", err)
			e.fail(cmd.generation, err)
			cmd.done <- err

			return
		}

		err := e.client.CommandAsync(cmd.generation, []string{"loadfile", cmd.source.URL})
		if err != nil {
			e.fail(cmd.generation, fmt.Errorf("load source: %w", err))
		}
		cmd.done <- err
	case commandStop:
		e.mu.Lock()
		e.pendingGeneration = 0
		e.activeGeneration = 0
		e.activeEntry = 0
		e.mu.Unlock()

		e.paused = false
		e.readAhead = ""

		err := e.client.CommandAsync(0, []string{"stop"})
		// Drop the fields with the session: the next load sets its own.
		if clearErr := e.setPlaybackHeaders(nil); clearErr != nil {
			e.cfg.Logger.Debug("clear playback headers", "error", clearErr)
		}
		cmd.done <- err
	case commandVolume:
		cmd.done <- e.client.SetProperty("volume", mpv.FormatDouble, float64(cmd.volume))
	case commandMute:
		cmd.done <- e.client.SetPropertyString("mute", yesNo(cmd.muted))
	case commandPause:
		cmd.done <- e.setPaused(cmd.paused)
	case commandReadAhead:
		// A bounded read-ahead, in seconds. Zero stops the demuxer from filling
		// its cache, which pause alone does not do on a live stream.
		cmd.done <- e.client.SetPropertyString("demuxer-readahead-secs", fmt.Sprint(cmd.readAhead))
	default:
		cmd.done <- fmt.Errorf("%w: unknown command", player.ErrInvalid)
	}
}

// setPaused applies the pause property and bounds read-ahead. It runs on the
// pump goroutine and keeps the runtime value it must restore.
func (e *Engine) setPaused(paused bool) error {
	if e.paused == paused {
		return nil
	}

	if paused {
		if value := e.client.GetPropertyString("demuxer-readahead-secs"); strings.TrimSpace(value) != "" {
			e.readAhead = value
		}
		if err := e.client.SetPropertyString("pause", yesNo(true)); err != nil {
			return err
		}
		if err := e.client.SetPropertyString("demuxer-readahead-secs", "0"); err != nil {
			e.cfg.Logger.Debug("player read-ahead", "error", err)
		}
		e.paused = true

		return nil
	}

	if e.readAhead != "" {
		if err := e.client.SetPropertyString("demuxer-readahead-secs", e.readAhead); err != nil {
			e.cfg.Logger.Debug("player read-ahead", "error", err)
		}
	}
	if err := e.client.SetPropertyString("pause", yesNo(false)); err != nil {
		return err
	}
	e.paused = false
	e.readAhead = ""

	return nil
}

// setPlaybackHeaders forwards only the sanitized User-Agent and Referer
// fields. An empty header set clears the option. The option value carries
// header content, so it is never logged.
//
// mpv documents http-header-fields as a comma-separated string list, and the
// property form options/<name> is writable at runtime, which is when the pump
// sets it: immediately before loadfile for one source.
func (e *Engine) setPlaybackHeaders(headers map[string]string) error {
	return e.client.SetPropertyString("options/http-header-fields", headerOptionValue(headers))
}

func (e *Engine) handleEvent(event *mpv.Event) {
	switch event.EventID {
	case mpv.EventCommandReply:
		e.handleCommandReply(event)
	case mpv.EventStart:
		e.handleStartFile(event.StartFile().EntryID)
	case mpv.EventFileLoaded:
		if generation := e.activeGen(); generation != 0 {
			e.emit(player.Event{Generation: generation, Kind: player.EventLoaded})
		}
	case mpv.EventPlaybackRestart:
		if generation := e.activeGen(); generation != 0 {
			e.emit(player.Event{Generation: generation, Kind: player.EventPlaying})
		}
	case mpv.EventEnd:
		e.handleEndFile(event.EndFile())
	case mpv.EventShutdown:
		e.emit(player.Event{Kind: player.EventShutdown})
	}
}

func (e *Engine) handleCommandReply(event *mpv.Event) {
	generation := event.ReplyUserdata
	if generation == 0 {
		return
	}

	e.mu.Lock()
	pending := e.pendingGeneration
	e.mu.Unlock()
	if pending != generation {
		return
	}

	if event.Error != nil {
		e.mu.Lock()
		e.pendingGeneration = 0
		e.activeGeneration = 0
		e.activeEntry = 0
		e.mu.Unlock()

		e.fail(generation, fmt.Errorf("load source: %w", event.Error))

		return
	}

	entry := entryID(event.CommandReply())

	e.mu.Lock()
	e.activeGeneration = generation
	e.activeEntry = entry
	e.pendingGeneration = 0
	e.mu.Unlock()

	e.emit(player.Event{Generation: generation, Kind: player.EventLoaded})
}

func (e *Engine) handleStartFile(entry int64) {
	e.mu.Lock()
	generation := e.activeGeneration
	expected := e.activeEntry
	if generation != 0 && expected == 0 {
		e.activeEntry = entry
	}
	e.mu.Unlock()

	if generation == 0 || (expected != 0 && entry != expected) {
		return
	}

	e.emit(player.Event{Generation: generation, Kind: player.EventStarted})
}

func (e *Engine) handleEndFile(end mpv.EventEndFile) {
	e.mu.Lock()
	generation := e.activeGeneration
	expected := e.activeEntry
	e.mu.Unlock()

	if generation == 0 || (expected != 0 && end.EntryID > 0 && end.EntryID != expected) {
		return
	}

	e.mu.Lock()
	e.activeGeneration = 0
	e.activeEntry = 0
	e.pendingGeneration = 0
	e.mu.Unlock()

	event := player.Event{
		Generation: generation,
		Kind:       player.EventEnded,
		Detail:     end.Reason.String(),
	}
	if end.Error != nil {
		event.Err = fmt.Errorf("playback ended: %w", end.Error)
	}

	e.cfg.Logger.Debug("player ended", "generation", generation, "reason", end.Reason.String(), "error", end.Error)
	e.emit(event)
}

func (e *Engine) publishMetrics() {
	generation := e.activeGen()
	if generation == 0 {
		return
	}

	timePos := durationProperty(e.client.GetPropertyString("time-pos"))
	duration := durationProperty(e.client.GetPropertyString("duration"))
	width := intProperty(e.client.GetPropertyString("width"))
	height := intProperty(e.client.GetPropertyString("height"))
	pausedForCache := yesProperty(e.client.GetPropertyString("paused-for-cache"))
	seekable := yesProperty(e.client.GetPropertyString("seekable"))
	live := duration <= 0

	metadata := player.Metadata{
		Title:           e.client.GetPropertyString("media-title"),
		Width:           width,
		Height:          height,
		VideoCodec:      e.client.GetPropertyString("video-codec"),
		AudioCodec:      e.client.GetPropertyString("audio-codec-name"),
		HardwareDecoder: e.client.GetPropertyString("hwdec-current"),
		VideoOutput:     e.client.GetPropertyString("current-vo"),
		AudioOutput:     e.client.GetPropertyString("current-ao"),
		Duration:        duration,
		Seekable:        seekable,
		Live:            live,
	}
	capabilities := player.Capabilities{
		HasAudio: metadata.AudioCodec != "",
		HasVideo: metadata.Width > 0 && metadata.Height > 0,
		Seekable: seekable,
		Live:     live,
	}
	metrics := player.Metrics{
		Generation:     generation,
		TimePos:        timePos,
		PausedForCache: pausedForCache,
	}

	e.cfg.Logger.Debug(
		"player metrics",
		"generation", generation,
		"width", width,
		"height", height,
		"video_codec", metadata.VideoCodec,
		"audio_codec", metadata.AudioCodec,
		"hwdec", metadata.HardwareDecoder,
		"vo", metadata.VideoOutput,
		"ao", metadata.AudioOutput,
		"paused_for_cache", pausedForCache,
	)

	e.emit(player.Event{
		Generation:   generation,
		Kind:         player.EventMetrics,
		Metadata:     metadata,
		Metrics:      metrics,
		Capabilities: capabilities,
	})
}

func (e *Engine) activeGen() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.activeGeneration
}

func (e *Engine) fail(generation uint64, err error) {
	e.cfg.Logger.Debug("player failure", "generation", generation, "error", err)
	e.emit(player.Event{
		Generation: generation,
		Kind:       player.EventFailed,
		Err:        err,
	})
}

func (e *Engine) emit(event player.Event) {
	select {
	case e.events <- event:
	case <-e.ctx.Done():
	}
}

func entryID(reply any) int64 {
	switch value := reply.(type) {
	case map[string]any:
		return int64Value(value["playlist_entry_id"])
	case int64:
		return value
	default:
		return 0
	}
}

func int64Value(value any) int64 {
	switch number := value.(type) {
	case int64:
		return number
	case uint64:
		return int64(number)
	case float64:
		return int64(number)
	default:
		return 0
	}
}

func durationProperty(value string) time.Duration {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || seconds < 0 {
		return 0
	}

	return time.Duration(seconds * float64(time.Second))
}

func intProperty(value string) int {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < 0 {
		return 0
	}

	return number
}

func yesProperty(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "yes")
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}

	return "no"
}

func (c Config) withDefaults() Config {
	if c.Volume < 0 || c.Volume > 100 {
		c.Volume = defaultVolume
	}
	if c.HardwareDecoding == "" {
		c.HardwareDecoding = "auto-safe"
	}
	if c.VideoOutput == "" {
		c.VideoOutput = "null"
	}
	if c.NetworkTimeout <= 0 {
		c.NetworkTimeout = defaultNetworkTimeout
	}
	if c.DemuxerMaxBytes == "" {
		c.DemuxerMaxBytes = defaultDemuxerMaxBytes
	}
	if c.MetricsInterval <= 0 {
		c.MetricsInterval = defaultMetricsInterval
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}

	return c
}
