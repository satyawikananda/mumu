# ADR 0003: Lifecycle, cancellation and UI dispatch

- Status: accepted
- Date: 2026-11-09
- Issues: #3, #5

## Context

MyGo draws native UI on the main thread and expects the view to read app
state there. Network loads, playlist parsing and decoder waits must not
block that thread. The app also needs one predictable shutdown path before
playback and background work exist.

## Decision

- `app.New()` creates one root `context.Context` and its cancel function.
- `App.Go(fn)` starts work with that context, using `sync.WaitGroup.Go`.
  A returned error or a recovered panic becomes user-visible state through
  `App.Fail`. `context.Canceled` is normal shutdown, not an error.
- State reaches the UI only through the `Dispatcher` port
  (`Update(func())`, `Invalidate()`), which `*mygo.Window` implements.
  Workers never touch widgets or the view.
- `App.Stop()` marks the app stopped, cancels the context and waits up to
  two seconds for jobs to finish. It is idempotent and runs from
  `App.OnBeforeQuit`.
- If the window is not open yet, or shutdown has started, published
  updates are dropped rather than drawn into an invalid window.
- Stale playback events are out of scope here. Issue #5 adds generation
  identifiers around the player state machine.

## Consequences

- Background jobs must not call main-thread-blocking APIs during shutdown,
  because `Stop` waits on the main thread for a bounded time. This rule is
  part of the player adapter's contract in #5.
- The two-second bound can only be verified by loading the app; unit tests
  cover cancellation, panic recovery and repeated `Stop`.
- Because workers cannot touch the shell, `internal/ui` needs no locks.
  Its fields change on the main thread only.
- Error text reaches the banner verbatim. Callers must not include
  credentials, query tokens or private paths; network and storage work
  added later needs a redaction step before publishing errors.
