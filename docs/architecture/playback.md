# Playback

Mumu TV plays live channels with libmpv. The app embeds video in its own window;
it never opens an external player, a WebView or a second decoder for PiP.

## Pieces

```text
internal/player          playback controller, state machine, Engine port
internal/player/libmpv   libmpv client adapter and runtime discovery
internal/player/libmpv/library  platform-aware search order, testable without a runtime
internal/video           retained NSOpenGLView and render thread (macOS)
third_party/go-mpv       pinned Go binding with an explicit runtime loader
```

The controller owns state and intents. The libmpv adapter owns the client and
event pump. The video package owns the native drawable and its render thread.
No layer above the adapter calls libmpv directly.

## Platform capability

| Target | Engine | Video presentation | Product behavior |
|---|---|---|---|
| `darwin/arm64` | real libmpv | retained `NSOpenGLView` and render thread | Watch and PiP |
| `windows/amd64`, `linux/amd64` | real libmpv, headless | none | Play refuses with an actionable error before loading a stream |
| any other target | stub | none | the runtime is reported unavailable |

Headless means the engine can create a client, decode into `vo=null`/`ao=null`,
report metadata and metrics, and honour pause, volume, mute, stop, cancellation
and teardown. It is not a player window: it does not attach a drawable, resize
with a window or enter PiP. The app therefore never exposes audio-only playback
or records a channel as watched when presentation is unsupported. The shared
engine depends on the `player.FrameRenderer` port, not on the macOS concrete
renderer.

## Native render bridge (macOS)

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

The real adapter is part of the `darwin/arm64`, `windows/amd64` and
`linux/amd64` builds. Other targets compile a stub that reports
`player.ErrRuntimeUnavailable`, so cross-builds need no native runtime.

The runtime is loaded on the first Play, never at package init or launch. A
packaged app prefers its bundled runtime over a system copy; a dev build
without a bundle keeps the loader-first order.

macOS (`libmpv.dylib`, then `libmpv.2.dylib`):

1. `MUMU_MPV_LIBRARY`, a dylib or a `:`-separated directory list.
2. `Contents/Resources/libmpv` and `Contents/Resources` next to the executable,
   when they exist.
3. The dynamic loader path, including `DYLD_LIBRARY_PATH`.
4. `/opt/homebrew/lib`, then `/usr/local/lib`.

Linux (`libmpv.so.2`, then `libmpv.so`):

1. `MUMU_MPV_LIBRARY`, a file or a `:`-separated directory list.
2. `<executable-dir>/libmpv`, then the executable directory.
3. The dynamic loader path, including `LD_LIBRARY_PATH`.
4. `/usr/local/lib`, `/usr/lib`, `/lib`.

Windows (`libmpv-2.dll`, `libmpv.dll`, `mpv-2.dll`, `mpv-1.dll`):

1. `MUMU_MPV_LIBRARY`, a file or a `;`-separated directory list. Every entry
   must be absolute; a relative entry is rejected with a clear error.
2. `<executable-dir>/libmpv`, then the executable directory.

Windows opens a DLL with `LoadLibraryExW` and
`LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`. Dependencies
therefore resolve from the DLL's own directory and System32 only. The loader
never searches the working directory or `PATH`, and the process search path is
not modified.

Before registration, the binding checks the binary architecture. It then
requires every entry point it registers and accepts only client API major `2`
with minor `1` or newer. A failed registration releases the native handle,
clears partially staged symbols and can be retried; a successful runtime stays
resident for the process. A failed architecture, symbol or API check is
classified in the user-facing error instead of echoing a native diagnostic that
could contain a path.

A packaged app therefore uses the runtime the release verified; a Homebrew or
system copy cannot silently shadow it. A dev build without a bundle keeps the
loader-first order.

The playback network path is guarded separately from the app's own HTTP client.
libmpv's HTTP(S) traffic is routed through a loopback proxy that applies the
`netguard` address policy to every request, CONNECT tunnel, redirect and HLS
segment, so a rebinding hostname or a private segment URL is refused. A source
whose stored policy carries explicit private-host consent bypasses the proxy,
which is the only case where mumu intentionally reaches a private destination.
The proxy fails closed: if it cannot be applied, the load fails instead of
connecting unproxied.

A missing or incompatible runtime produces one actionable Watch error. Installing
a runtime and pressing Retry works without restarting the app.

`MUMU_MPV_LIBRARY` exists because the `go` command strips `DYLD_LIBRARY_PATH`
from binaries it runs. Error messages name library files that were tried, never a
full launch path.

## Default engine options

The adapter uses `config=no`, `load-scripts=no`, `ytdl=no`, no terminal or input
bindings, `idle=yes`, `keep-open=no`, a 10-second network timeout,
`demuxer-max-bytes=32MiB` and `hwdec=auto-safe`. Headless runs use `vo=null` and
`ao=null`; the app sets `vo=libmpv` only where a native drawable exists. The
adapter reports `hwdec-current`, `current-vo` and `current-ao` instead of
assuming them.

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
- Windows and Linux have no video presentation. Their headless tests prove the
  engine, loader and media proxy, not a window, resizing or PiP.
- Release packaging remains macOS-only. A headless build is not a supported
  desktop release on Windows or Linux.

See [Validation](validation.md) for the tested configuration and remaining gaps.
