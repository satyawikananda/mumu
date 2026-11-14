# Architecture

Mumu TV is a native macOS desktop TV player. MyGo renders every screen, libmpv
plays the stream, and all user data stays on the device. The application has no
WebView, no hosted backend and no account system.

This document describes the system boundaries and the runtime shape. Component
details live in the linked documents.

## Product constraints

- Native first. The interface is MyGo `ui`; playback is libmpv.
- One viewing session. Watch and Picture-in-Picture are two presentations of the
  same retained video surface and the same player.
- Local first. Playlists, favorites, recent history and preferences live in
  SQLite on the Mac.
- No autoplay. Launch never starts audio or network playback.
- Evidence over assumption. A stream is not proven online, licensed or
  playable by its URL.

## Module boundaries

```text
cmd/mumu/                 application entry point
internal/app/             composition root, window lifecycle, background jobs
internal/ui/              MyGo shell, pages, tokens, fonts and components
internal/player/          playback state machine and Engine port
internal/player/libmpv/   libmpv adapter and runtime discovery
internal/video/           retained native video surface and presentation
internal/catalog/         channel domain, identity, filters and provider boundary
internal/catalog/iptvorg/ community catalog and blocklists
internal/netguard/        bounded HTTP client and untrusted URL policy
internal/playlist/        M3U parsing, content detection and source loading
internal/sources/         catalog-to-storage mapping, built-in catalog and imports
internal/logos/           bounded logo fetch, decode and cache
internal/storage/         SQLite repositories and migrations
```

Dependencies point inward: UI dispatches intents, the app coordinates work,
`player` owns playback state, `video` owns presentation, and domain packages own
no UI or native code. `third_party/go-mpv` is a pinned local patch of the Go
libmpv binding.

## Runtime flow

```text
source or playlist
      │
      ▼
catalog / playlist / sources
      │ normalized channels
      ▼
storage ──────────────► library cache
      │                     │
      │                     ▼
      │                 UI intents
      │                     │
      ▼                     ▼
Playback controller ──► libmpv engine ──► retained video surface
      │                                        │
      └──────── snapshots ──► Watch / PiP ◄────┘
```

- Catalog providers produce the same channel model for built-in and user
  sources.
- Storage persists user intent. The network catalog remains replaceable and
  cached by the source that owns it.
- The player controller owns loading, generations, retries, pause and teardown.
- The video surface owns the retained native view and its render thread. It
  never creates a second decoder or an external player window.
- PiP moves the retained view between native windows. It does not reload the
  stream.

## Threading and lifetime

- MyGo widgets are built and mutated only on the UI thread.
- Network, parsing, catalog refresh, storage writes and playback waits run in
  bounded background jobs with cancellation.
- Controller snapshots reach the UI through `Window.Update`.
- A request or generation identifier prevents events from an older channel load
  from overwriting the active one.
- Shutdown order is fixed: stop the video surface, close the controller and
  engine, then close the local library.
- Native view calls stay on the main thread; render calls stay on the dedicated
  locked render thread.

## Data and state

SQLite stores sources, channels, stream variants, favorites, recent channels and
settings. The database file is local, private to the user and never deleted
automatically. Playback history records only real `playing` state, not clicks or
failed attempts.

Storage is the source of truth for user intent. Catalog caches are replaceable
and must not grow without a bound. Credentials are not stored; only sanitized
`User-Agent` and `Referer` playback headers are kept.

## Trust and network policy

- Only `http` and `https` are accepted for remote media.
- Loopback, private, link-local and local network destinations are refused
  unless the user explicitly consents for one source.
- Redirects, response sizes and request timeouts are bounded.
- A remote catalog cannot widen the policy of a built-in source.
- The iptv-org catalog is a labelled community list, not a rights review or a
  guarantee that a stream is online.
- Logs redact URLs, credentials and header values.

## Failure behavior

- Player states distinguish idle, loading, playing, buffering, paused, error and
  stopping.
- Recovery is bounded. Exhaustion is terminal and must not claim it is still
  retrying.
- A missing libmpv runtime is an actionable playback error, not a launch crash.
- Renderer failures become a playback error and do not crash the app.
- Live streams are not assumed seekable. Watch does not draw a fake playhead.

## Platform constraints

- macOS on Apple Silicon is the supported release target.
- MyGo uses Metal; the video bridge uses a retained `NSOpenGLView` under the
  MyGo surface. macOS OpenGL is deprecated, so the bridge stays replaceable.
- The app does not claim direct Metal interop, zero-copy rendering, hardware
  decoding, frame-perfect presentation or support for every codec or provider.
- Windows and Linux adapters may compile, but they are not supported releases.

## Related documents

- [Library](library.md)
- [Playback](playback.md)
- [Player controller](player-controller.md)
- [Picture-in-Picture](pip.md)
- [Sources](sources.md)
- [Storage](storage.md)
- [Validation](validation.md)
- [Video surface](video-surface.md)
- [Watch](watch.md)
- [UX](../ux/shell.md)
- [Release](../release/macos.md)
