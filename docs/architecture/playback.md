# Playback

Mumu TV plays live channels with libmpv. The app embeds video in its own window;
it never opens an external player, a WebView or a second decoder for PiP.

## Pieces

```text
internal/player          playback controller, state machine, Engine port
internal/player/libmpv   libmpv client adapter and runtime discovery
internal/video           retained NSOpenGLView and render thread
third_party/go-mpv       pinned Go binding with an explicit runtime loader
```

The controller owns state and intents. The libmpv adapter owns the client and
event pump. The video package owns the native drawable and its render thread.
No layer above the adapter calls libmpv directly.

## Native render bridge

```mermaid
flowchart LR
  UI[MyGo native UI] -->|bounded intents| C[libmpv control worker]
  C --> D[One decoder and audio session]
  D --> R[Locked render thread]
  R -->|libmpv OpenGL render API| GL[One NSOpenGLContext / NSOpenGLView]
  GL --> W[Retained view under MyGo's surface]
  C -->|Window.Update| UI
```

- MyGo renders its UI with Metal. The video uses a separate retained OpenGL
  view composed by AppKit under MyGo's window surface.
- No OpenGL frame buffer is passed to Metal. Direct Metal interop and zero-copy
  presentation are not claimed.
- The retained view moves between the main window and PiP without a new stream
  load, decoder or player.
- MyGo keeps pointer input because the video view is placed under its surface.

## Threading

| Goroutine | Responsibility |
|---|---|
| main (AppKit) | create, attach, detach, resize and close the native view |
| render thread | make the GL context current, render, report swap, free |
| libmpv pump | every client API call, event wait and property poll |
| callback thread | signal the render thread through one non-blocking channel |

Drawing and client control never share a goroutine. Native attachment and
rendering are guarded by a Go mutex and a native CGL context lock. The context is
made current before rendering and after every drawable change.

## Runtime loading

The real adapter is part of the default `darwin/arm64` build. Other targets
compile a stub that reports `player.ErrRuntimeUnavailable`, so cross-builds need
no native runtime.

The runtime is loaded on the first Play, never at package init or launch. Search
order:

1. `MUMU_MPV_LIBRARY`, a dylib or a `:`-separated directory list.
2. The dynamic loader path, including `DYLD_LIBRARY_PATH`.
3. `/opt/homebrew/lib`, then `/usr/local/lib`.
4. `Contents/Resources/libmpv` and `Contents/Resources` next to the executable.

A missing or incompatible runtime produces one actionable Watch error. Installing
a runtime and pressing Retry works without restarting the app.

`MUMU_MPV_LIBRARY` exists because the `go` command strips `DYLD_LIBRARY_PATH`
from binaries it runs. Error messages name library files that were tried, never a
full launch path.

## Default engine options

The adapter uses `config=no`, `load-scripts=no`, `ytdl=no`, no terminal or input
bindings, `idle=yes`, `keep-open=no`, a 10-second network timeout,
`demuxer-max-bytes=32MiB` and `hwdec=auto-safe`. Headless control uses
`vo=null`. The adapter reports `hwdec-current`, `current-vo` and `current-ao`
instead of assuming them.

## Licensing

The Go binding is MIT. libmpv, FFmpeg and their dependencies have their own
licenses and build-dependent obligations. The release ships an LGPL build with
GPL-only components disabled. The app is dynamically linked and unmodified, so
users may replace the runtime with a compatible build.

See [macOS packaging](../release/macos.md) for the pinned runtime and
[THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md) for notices.

## Limits

- macOS OpenGL is deprecated. The bridge stays isolated so a future native
  adapter can replace it.
- The bundled LGPL runtime has no VideoToolbox interop, so playback is software
  decoded. The app does not claim hardware decoding.
- Presentation smoothness, GPU attribution, energy use and thermal behavior are
  not guaranteed by the current validation.
- Live streams are not assumed seekable. Pause holds a frame and stops audio, but
  a small residual network read remains while paused.
- Codec and provider coverage is not universal.

See [Validation](validation.md) for the tested configuration and remaining gaps.
