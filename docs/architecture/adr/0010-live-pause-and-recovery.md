# ADR 0010: Live pause, bounded reconnection and stream variants

- Status: accepted
- Date: 2026-11-13
- Issues: [#37](https://github.com/satyawikananda/mumu/issues/37), tracker [#30](https://github.com/satyawikananda/mumu/issues/30)
- Evidence: [overlay pause seam](../evidence/overlay-2026-11-12/README.md#pause-seam)
- Related: [playback controller](../player-controller.md), [Watch](../watch.md), [PiP](../pip.md), [ADR 0009](0009-native-video-overlay.md)

## Context

The M0 overlay spike proved that a retained libmpv session can hold its last
presented frame while paused, stop its audio output and bound demuxer
read-ahead. It also measured a residual live read: pause alone does not close
the demuxer, and `demuxer-readahead-secs=0` still leaves playlist refreshes and
in-flight segment reads, about 5% of the playing rate in that run.

The M5 product needs an honest live pause, a reconnect that reaches the current
live edge, a finite recovery episode and real quality selection. It must not
invent seek, timeshift, a second decoder or a guaranteed zero-network pause.

## Decision

### Pause is local

- `Pause` sets libmpv's `pause` property and `demuxer-readahead-secs=0`.
  The displayed frame stays, audio stops, read-ahead is bounded.
- The controller reports `paused`. The native surface stays attached, because
  the frame is still the picture to show.
- A paused session ignores later engine `Ended`/`Failed` events; the frame is
  held locally. A quality change made while paused loads the new variant with
  pause still set, so it does not start playing.
- The UI must not claim zero network. The residual read is documented and
  measured again on the shipping runtime when native evidence is recorded.

### Resume reconnects at the live edge

- `Resume` increments the episode, increments the generation and calls
  `loadfile` for the same variant. Pause stays set until the file is loaded,
  then the engine unpauses. The old paused position is never resumed or
  seeked.
- The state passes through `buffering` while the replacement stream connects.
  No timeshift buffer, elapsed time or seek control exists.

### Recovery is finite

- One episode makes at most **three attempts including the initial one**.
- Each attempt has an **8-second no-progress deadline**: no start or advancing
  `time-pos` within eight seconds fails it. A mid-play buffering stall with no
  progress uses the same deadline.
- Attempts are at least **eight seconds apart**. A fast failure waits the
  remainder and shows a retry countdown; a stall that already consumed eight
  seconds retries immediately. There is no endless retry and no retrying claim
  after the final attempt.
- Recovery advances through the remaining **variants of the same channel**. It
  never selects another channel. When a variant is exhausted, the next attempt
  cycles through the same ordered list.
- Pause (when a frame exists), Stop, a new Play/PlayRequest, a quality change
  and Close cancel recovery. A manual Retry replays the request as a new
  episode with attempts reset.
- `ErrRuntimeUnavailable`, invalid requests, unsupported schemes, closed
  player and context cancellation are terminal: a missing libmpv runtime never
  loops.

### Quality comes from declared variants

- A request carries ordered `Variant`s and a quality selection: `auto` or an
  exact playlist label.
- `auto` keeps the supported-stream order the app already chooses (live/HLS
  first).
- An explicit label moves its first matching variant to the front. The
  snapshot reports the label actually attempted, whether it is known and
  whether it is a fallback; an unavailable label falls back to `auto` instead
  of pretending it exists.
- An unlabelled variant still participates in fallback but is not offered as a
  quality choice. Labels are playlist metadata, never measured resolution.

## Consequences

- The controller gains `Pause`, `Resume`, `SelectQuality`, `PlayRequest`,
  `StatePaused`, episode identity, quality and recovery metadata. The engine
  port gains `SetPaused`.
- App history marks one episode rather than every retry, so failed attempts do
  not create a successful recent entry.
- The app keeps one engine, one controller and one native view. Pause,
  reconnect and quality changes never create a second decoder.
- Native verification is still required: the residual read, the paused frame,
  live-edge resume and variant fallback are runtime behaviour, not compile-time
  facts.
