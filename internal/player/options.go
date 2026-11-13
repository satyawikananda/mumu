package player

import (
	"log/slog"
	"time"
)

type config struct {
	initialVolume  int
	initialMuted   bool
	stallTimeout   time.Duration
	retryFloor     time.Duration
	maxAttempts    int
	commandTimeout time.Duration
	closeTimeout   time.Duration
	logger         *slog.Logger
	fullscreen     func()
}

// DefaultVolume is the starting volume when the viewer has no saved choice.
// The app shows it before any controller exists.
const DefaultVolume = 30

// Recovery limits. MaxAttempts counts the initial attempt, as the product
// contract requires.
const (
	MaxAttempts = 3
)

func defaultConfig() config {
	return config{
		initialVolume:  DefaultVolume,
		initialMuted:   false,
		stallTimeout:   8 * time.Second,
		retryFloor:     8 * time.Second,
		maxAttempts:    MaxAttempts,
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

// WithStallTimeout bounds how long an attempt may go without progress before
// the next variant is tried. A zero value keeps the default.
func WithStallTimeout(timeout time.Duration) Option {
	return func(c *config) {
		if timeout > 0 {
			c.stallTimeout = timeout
		}
	}
}

// WithRetryFloor sets the minimum time between two attempt starts. A zero
// value retries immediately, for tests; a negative value keeps the default.
func WithRetryFloor(wait time.Duration) Option {
	return func(c *config) {
		if wait >= 0 {
			c.retryFloor = wait
		}
	}
}

// WithMaxAttempts bounds one recovery episode, including the initial attempt.
// Values below one are ignored.
func WithMaxAttempts(attempts int) Option {
	return func(c *config) {
		if attempts > 0 {
			c.maxAttempts = attempts
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
