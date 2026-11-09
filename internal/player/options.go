package player

import (
	"log/slog"
	"time"
)

type config struct {
	initialVolume  int
	initialMuted   bool
	loadingTimeout time.Duration
	commandTimeout time.Duration
	closeTimeout   time.Duration
	logger         *slog.Logger
	fullscreen     func()
}

// DefaultVolume is the starting volume when the viewer has no saved choice.
// The app shows it before any controller exists.
const DefaultVolume = 30

func defaultConfig() config {
	return config{
		initialVolume:  DefaultVolume,
		initialMuted:   false,
		loadingTimeout: 20 * time.Second,
		commandTimeout: 2 * time.Second,
		closeTimeout:   3 * time.Second,
		logger:         slog.Default(),
	}
}

// Option customizes a Controller.
type Option func(*config)

// WithInitialVolume sets the starting volume, clamped to 0..100.
func WithInitialVolume(volume int) Option {
	return func(c *config) {
		c.initialVolume = clampVolume(volume)
	}
}

// WithInitialMuted sets the starting mute state.
func WithInitialMuted(muted bool) Option {
	return func(c *config) {
		c.initialMuted = muted
	}
}

// WithLoadingTimeout bounds how long a source may stay in loading.
func WithLoadingTimeout(timeout time.Duration) Option {
	return func(c *config) {
		if timeout > 0 {
			c.loadingTimeout = timeout
		}
	}
}

// WithCommandTimeout bounds engine command submission.
func WithCommandTimeout(timeout time.Duration) Option {
	return func(c *config) {
		if timeout > 0 {
			c.commandTimeout = timeout
		}
	}
}

// WithCloseTimeout bounds engine teardown.
func WithCloseTimeout(timeout time.Duration) Option {
	return func(c *config) {
		if timeout > 0 {
			c.closeTimeout = timeout
		}
	}
}

// WithLogger sets the diagnostic logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithFullscreenHandler receives a fullscreen request from the UI. The
// handler runs on the caller's goroutine and must not block.
func WithFullscreenHandler(handler func()) Option {
	return func(c *config) {
		c.fullscreen = handler
	}
}
