package player

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mumu/internal/netguard"
)

// qualityAuto keeps the supported-stream order the app supplied.
const qualityAuto = "auto"

// normalizeRequest validates one episode and copies the keys it owns. Only
// http and https reach an engine; a private host or a header is a source-policy
// decision made before this call.
func normalizeRequest(request Request) (Request, error) {
	if len(request.Variants) == 0 {
		return Request{}, fmt.Errorf("%w: no streams", ErrInvalid)
	}

	out := Request{
		Variants: make([]Variant, 0, len(request.Variants)),
		Quality:  strings.TrimSpace(request.Quality),
	}
	if out.Quality == "" {
		out.Quality = qualityAuto
	}

	for _, variant := range request.Variants {
		source := variant.Source
		source.URL = strings.TrimSpace(source.URL)
		if source.URL == "" {
			return Request{}, fmt.Errorf("%w: source URL is empty", ErrInvalid)
		}
		if _, err := sourceScheme(source); err != nil {
			return Request{}, err
		}
		// Re-apply the stored source policy at the boundary the engine trusts.
		// Import already filtered these URLs; this rejects a stale or tampered
		// row before it can reach the media engine.
		policy := netguard.Policy{AllowPrivate: source.AllowPrivate}
		if _, err := policy.ValidateURL(source.URL); err != nil {
			return Request{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		if len(source.Headers) > 0 {
			headers := make(map[string]string, len(source.Headers))
			for name, value := range source.Headers {
				headers[name] = value
			}
			source.Headers = headers
		}

		out.Variants = append(out.Variants, Variant{
			Source:  source,
			Quality: strings.TrimSpace(variant.Quality),
		})
	}

	return out, nil
}

// orderedVariants puts the selected variant first and keeps the rest as
// recovery candidates. The returned quality is "auto" when the label is not
// one of the real variants, so the snapshot never claims an unavailable choice.
func orderedVariants(request Request) ([]Variant, string) {
	selected := strings.TrimSpace(request.Quality)
	if selected == "" || strings.EqualFold(selected, qualityAuto) {
		return request.Variants, qualityAuto
	}

	for i, variant := range request.Variants {
		if !strings.EqualFold(strings.TrimSpace(variant.Quality), selected) {
			continue
		}

		order := make([]Variant, 0, len(request.Variants))
		order = append(order, variant)
		order = append(order, request.Variants[:i]...)
		order = append(order, request.Variants[i+1:]...)

		return order, variant.Quality
	}

	return request.Variants, qualityAuto
}

func (c *Controller) handlePlayRequest(request Request) {
	if c.Snapshot().State == StateStopping {
		return
	}

	c.cancelRecovery()
	c.stopTimers()
	c.request = request
	c.order, c.selectedQuality = orderedVariants(request)
	c.episode++
	c.attempt = 1
	c.hasFrame = false
	c.paused = false
	c.unpauseOnLoad = c.enginePaused
	c.lastProgress = 0
	c.startAttempt("Loading")
}

func (c *Controller) handlePause() {
	state := c.Snapshot().State
	if c.paused || !c.hasFrame || (state != StatePlaying && state != StateBuffering) {
		return
	}

	c.cancelRecovery()
	c.stopTimers()
	c.paused = true
	c.enginePaused = true
	c.unpauseOnLoad = false
	c.update(func(s *Snapshot) {
		s.State = StatePaused
		s.Detail = "Paused"
		s.Error = ""
		s.Recovery = Recovery{}
	})

	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	if err := c.engine.SetPaused(ctx, true); err != nil && !errors.Is(err, ErrClosed) {
		c.cfg.logger.Debug("player pause", "error", err)
	}
}

func (c *Controller) handleResume() {
	if !c.paused || len(c.order) == 0 {
		return
	}

	c.stopTimers()
	c.paused = false
	c.unpauseOnLoad = true
	c.episode++
	c.attempt = 1
	c.lastProgress = 0
	c.startAttempt("Reconnecting")
}

func (c *Controller) handleQuality(label string) {
	state := c.Snapshot().State
	if state == StateStopping || state == StateIdle || state == StateStopped || state == StateError || len(c.order) == 0 {
		return
	}

	request := c.request
	request.Quality = strings.TrimSpace(label)
	normalized, err := normalizeRequest(request)
	if err != nil {
		c.cfg.logger.Debug("player quality", "error", err)

		return
	}

	c.cancelRecovery()
	c.stopTimers()
	c.request = normalized
	c.order, c.selectedQuality = orderedVariants(normalized)
	c.episode++
	c.attempt = 1
	c.lastProgress = 0
	if !c.paused {
		c.unpauseOnLoad = c.enginePaused
	}
	c.startAttempt("Reconnecting")
}

// startAttempt loads the next variant of the current episode. The state keeps
// the last frame when recovery has one, and stays paused while the viewer holds
// a paused quality change.
func (c *Controller) startAttempt(detail string) {
	if len(c.order) == 0 {
		return
	}

	variant := c.order[(c.attempt-1)%len(c.order)]
	c.generation++
	generation := c.generation
	c.attemptStarted = time.Now()
	c.retryPending = false
	c.loading = true
	c.lastProgress = 0

	state := StateLoading
	switch {
	case c.paused:
		state = StatePaused
	case c.hasFrame:
		state = StateBuffering
	}

	c.armStallTimer()
	c.update(func(s *Snapshot) {
		s.State = state
		s.Generation = generation
		s.Episode = c.episode
		s.Source = variant.Source
		s.Detail = detail
		s.Error = ""
		s.Metadata = Metadata{}
		s.Capabilities = Capabilities{}
		s.Metrics = Metrics{Generation: generation}
		s.Quality = c.qualityFor(variant)
		s.Recovery = Recovery{Attempt: c.attempt, MaxAttempts: c.cfg.maxAttempts}
	})

	ctx, cancel := context.WithTimeout(c.ctx, c.cfg.commandTimeout)
	defer cancel()

	if err := c.engine.Load(ctx, generation, variant.Source); err != nil {
		c.attemptFailed(generation, fmt.Errorf("load source: %w", err))
	}
}

// attemptFailed advances a recoverable episode or stops at a terminal error.
// A failure that already consumed the stall timeout retries at once; a fast
// failure waits out the retry floor, which keeps the countdown truthful.
func (c *Controller) attemptFailed(generation uint64, err error) {
	if err == nil || generation != c.generation {
		return
	}

	c.stopStallTimer()
	c.stopRetryTimer()
	c.retryPending = false
	c.loading = false

	if terminalError(err) || c.attempt >= c.cfg.maxAttempts {
		c.terminal(generation, err)

		return
	}

	c.attempt++
	wait := c.cfg.retryFloor - time.Since(c.attemptStarted)
	if wait < 0 {
		wait = 0
	}

	c.update(func(s *Snapshot) {
		if s.Generation != generation {
			return
		}
		switch {
		case c.paused:
			s.State = StatePaused
		case c.hasFrame:
			s.State = StateBuffering
		default:
			s.State = StateLoading
		}
		s.Recovery = Recovery{
			Attempt:     c.attempt,
			MaxAttempts: c.cfg.maxAttempts,
			Pending:     true,
			RetryIn:     wait,
		}
		s.Detail = "Reconnecting"
		s.Error = ""
	})

	if wait <= 0 {
		c.startAttempt("Reconnecting")

		return
	}

	c.retryPending = true
	c.retryTimer = resetTimer(c.retryTimer, wait)
	if wait > time.Second {
		c.retryTick = resetTimer(c.retryTick, time.Second)
	}
}

// terminal stops every retry and reports exhaustion. No later attempt is
// scheduled, so the UI never claims it is still retrying.
func (c *Controller) terminal(generation uint64, err error) {
	if err == nil {
		return
	}

	c.stopTimers()
	c.retryPending = false
	c.loading = false
	c.hasFrame = false
	c.paused = false
	c.enginePaused = false
	c.unpauseOnLoad = false

	detail := sanitizeDetail(err.Error())
	c.cfg.logger.Debug("player failure", "generation", generation, "detail", detail)
	c.engineStop()

	attempt := max(c.attempt, 1)
	c.update(func(s *Snapshot) {
		if s.Generation != generation {
			return
		}
		s.State = StateError
		s.Detail = detail
		s.Error = detail
		s.Recovery = Recovery{
			Attempt:     attempt,
			MaxAttempts: c.cfg.maxAttempts,
			Terminal:    true,
		}
	})
}

func (c *Controller) fail(generation uint64, err error) {
	c.terminal(generation, err)
}

func (c *Controller) clearRecovery() {
	c.update(func(s *Snapshot) {
		s.Recovery = Recovery{}
		s.Error = ""
	})
}

func (c *Controller) cancelRecovery() {
	c.stopRetryTimer()
	c.stopRetryTick()
	c.retryPending = false
}

func (c *Controller) handleStallTimeout() {
	if c.paused || c.retryPending || len(c.order) == 0 {
		return
	}

	state := c.Snapshot().State
	if state != StateLoading && state != StatePlaying && state != StateBuffering {
		return
	}
	if c.Snapshot().Recovery.Terminal {
		return
	}

	c.attemptFailed(c.generation, errors.New("stream stopped making progress"))
}

func (c *Controller) handleRetryTimer() {
	if !c.retryPending {
		return
	}
	c.retryPending = false
	c.stopRetryTick()
	c.startAttempt("Reconnecting")
}

// handleRetryTick advances the published countdown once a second while a retry
// waits. It never runs the attempt itself; the retry timer owns that.
func (c *Controller) handleRetryTick() {
	if !c.retryPending {
		return
	}

	remaining := c.Snapshot().Recovery.RetryIn - time.Second
	if remaining < 0 {
		remaining = 0
	}
	c.update(func(s *Snapshot) {
		if !c.retryPending {
			return
		}
		s.Recovery.RetryIn = remaining
	})
	if remaining > 0 {
		c.retryTick = resetTimer(c.retryTick, time.Second)
	}
}

func (c *Controller) armStallTimer() {
	if c.cfg.stallTimeout <= 0 {
		return
	}

	c.stallTimer = resetTimer(c.stallTimer, c.cfg.stallTimeout)
}

func (c *Controller) stopStallTimer() {
	stopTimer(c.stallTimer)
}

func (c *Controller) stopRetryTimer() {
	stopTimer(c.retryTimer)
}

func (c *Controller) stopRetryTick() {
	stopTimer(c.retryTick)
}

func (c *Controller) stopTimers() {
	c.stopStallTimer()
	c.stopRetryTimer()
	c.stopRetryTick()
	c.retryPending = false
}

func (c *Controller) stallTimerC() <-chan time.Time {
	if c.stallTimer == nil {
		return nil
	}

	return c.stallTimer.C
}

func (c *Controller) retryTimerC() <-chan time.Time {
	if c.retryTimer == nil {
		return nil
	}

	return c.retryTimer.C
}

func (c *Controller) retryTickC() <-chan time.Time {
	if c.retryTick == nil {
		return nil
	}

	return c.retryTick.C
}

func resetTimer(timer *time.Timer, wait time.Duration) *time.Timer {
	if timer == nil {
		return time.NewTimer(wait)
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(wait)

	return timer
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func terminalError(err error) bool {
	return errors.Is(err, ErrRuntimeUnavailable) ||
		errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrUnsupportedScheme) ||
		errors.Is(err, ErrClosed) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func (c *Controller) qualityFor(variant Variant) Quality {
	label := strings.TrimSpace(variant.Quality)
	quality := Quality{
		Selected: c.selectedQuality,
		Current:  label,
		Known:    label != "",
	}
	quality.Fallback = !strings.EqualFold(c.selectedQuality, qualityAuto) &&
		!strings.EqualFold(c.selectedQuality, label)

	return quality
}
