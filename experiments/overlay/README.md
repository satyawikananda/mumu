# Native video overlay spike

Standalone macOS experiment for [#32](https://github.com/satyawikananda/mumu/issues/32).
It plays one real libmpv session and draws MyGo chrome above it. It is not the
application, and it is not a second player.

Read the decision in
[`docs/architecture/adr/0009-native-video-overlay.md`](../../docs/architecture/adr/0009-native-video-overlay.md)
before reusing this bridge.

## What it proves

MyGo 0.9 draws its native UI in a layer-backed `MyGoSurfaceView` whose Metal
layer is not opaque, and it already places a native view **under** that surface
for `ui.WebView` and for native-module views. `ui.NativeView` is the public form
of that placement: it puts an `NSView` in the element's content box, clipped to
the element, and moves it between windows that build the same element.

The spike uses that path for mumu's retained `NSOpenGLView`:

- `-mode below` (default): the app inserts the retained view under MyGo's
  surface itself, so chrome paints above live video and MyGo keeps every pointer
  and keyboard event. This is the candidate path in ADR 0009.
- `-mode nativeview`: MyGo hosts the view as a `ui.NativeView` element. The
  composition is right, but MyGo hands the pointer to the placed view across its
  whole box, which swallows clicks on chrome drawn over it. Recorded, not used.
- `-mode above`: the surface attaches over the window content, as the product
  does today. Chrome built in the window stays behind the video, which is the
  comparison case that shows why the overlay path is needed.

Exactly one libmpv client, one stream load and one retained view exist in both
modes. Moving between the main window and the floating panel reparents that
view; it does not reload the source.

## Requirements

- macOS arm64, Go 1.27.3, MyGo 0.9.0, the pinned `gen2brain/go-mpv` binding.
- A libmpv runtime with the render API. The release build in
  `build/runtime/lib` works:
  `MUMU_MPV_LIBRARY=build/runtime/lib`.
- The synthetic HLS fixture from `../hlsfixture`, or any authorized test
  stream. Do not point it at third-party channels for evidence.

## Build and run

```sh
# One terminal: the local HLS fixture (rolling six-segment playlist).
go build -o /private/tmp/mumu-hlsfixture ./experiments/hlsfixture
/private/tmp/mumu-hlsfixture -resolution 720 -seconds 600 -port 18765

# Another terminal: the spike.
go build -o /private/tmp/mumu-overlay-spike ./experiments/overlay
MUMU_MPV_LIBRARY=build/runtime/lib /private/tmp/mumu-overlay-spike \
  -stream http://127.0.0.1:18765/index.m3u8 \
  -autoplay
```

Flags:

| Flag | Meaning |
|---|---|
| `-stream` | URL loaded into the single session (required) |
| `-mode` | `below` (default), `nativeview` or `above` |
| `-volume` | engine volume, 0-100, default 70 |
| `-autoplay` | load as soon as the window is ready; spike only, the product never autoplays |
| `-autoclose` | quit after this duration, for scripted evidence runs |
| `-logfile` | write logs to a file instead of stderr, for runs started through `open(1)` |
| `-pause-after` | pause the stream after this duration, for scripted evidence runs |
| `-resume-after` | resume a scripted pause after this duration |
| `-keepfront` | keep the window in front for scripted screenshots; experiment only |

A bare command-line binary does not come to the front of other apps on macOS.
For screen recording, wrap it in a minimal bundle and start it through
`launchctl`:

```sh
mkdir -p /private/tmp/Spike.app/Contents/MacOS
cp /private/tmp/mumu-overlay-spike /private/tmp/Spike.app/Contents/MacOS/
# Info.plist: CFBundleExecutable=overlay-spike, CFBundlePackageType=APPL
launchctl setenv MUMU_MPV_LIBRARY "$PWD/build/runtime/lib"
open -n /private/tmp/Spike.app --args -stream http://127.0.0.1:18765/index.m3u8 -autoplay
```

## Controls

| Input | Effect |
|---|---|
| Play / Pause | load the stream, or pause and resume the same session |
| Stop | stop the stream, keep the session |
| Drawer | open the channel drawer |
| Fullscreen | toggle fullscreen on the window that shows the video |
| PiP | move the same session between the main window and the panel |
| Hide chrome | remove the chrome to judge the video alone |
| Clicks counter | counts pointer hits on chrome above video; the hit test evidence |

## Evidence

The spike logs, per run:

- the window content bounds and the retained view handle;
- the drawable size in backing pixels that `SyncDrawable` read from the hosted
  view, so a zero or stale drawable is visible instead of guessed;
- engine events (`loaded`, `playing`, `failed`) and pause requests;
- attach and sync failures.

Capture a window without bringing it forward:

```sh
screencapture -x -o -l <window-id> frame.png
```

`CGWindowListCopyWindowInfo` gives the window id for owner `mumu overlay spike`.

Measure frames with `MYGO_FRAME_STATS=1` and CPU with `ps -o %cpu,rss -p <pid>`.
Audio during pause needs a person at the machine: the fixture beeps every
second, so silence during pause is audible. Do not claim that evidence from a
log alone.
