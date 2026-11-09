# Playback controller

Issue: [#5](https://github.com/satyawikananda/mumu/issues/5). Decision:
[ADR 0005](adr/0005-playback-controller.md). Feasibility evidence:
[native playback](native-playback-feasibility.md).

`internal/player` owns playback state, intents and teardown. It does not own
MyGo screens or the native video surface. `internal/video` remains the
presentation boundary; its native adapter lands with #11.

## States

```mermaid
stateDiagram-v2
  [*] --> idle
  idle --> loading: Play
  loading --> playing: Loaded / PlaybackRestart / time-pos > 0
  loading --> error: timeout or load failure
  playing --> buffering: paused-for-cache
  buffering --> playing: cache resumed
  loading --> stopped: End(EOF)
  playing --> stopped: End(EOF)
  buffering --> stopped: End(EOF)
  loading --> error: End(error)
  playing --> error: End(error)
  buffering --> error: End(error)
  error --> loading: Play
  stopped --> loading: Play
  idle --> stopped: Stop
  loading --> stopping: Stop
  playing --> stopping: Stop
  buffering --> stopping: Stop
  error --> stopping: Stop
  stopping --> stopped
  stopped --> [*]: Close
```

`StateUnknown` is the zero value. `stopped` still accepts `Play`; `Close` is
terminal.

## API

```go
controller := player.New(engine,
    player.WithLoadingTimeout(20*time.Second),
    player.WithFullscreenHandler(func() { /* dispatch to the window */ }),
)

controller.Subscribe(func(snapshot player.Snapshot) {
    // Runs on the notifier goroutine. Dispatch to MyGo:
    // window.Update(func() { app.player = snapshot })
})

err := controller.Play(player.Source{URL: "https://example.com/live.m3u8"})
err = controller.SetVolume(30)
err = controller.SetMuted(false)
err = controller.RequestFullscreen()
err = controller.Stop()
err = controller.Close(ctx)
```

`Play`, `Stop`, `SetVolume` and `SetMuted` enqueue intents and return
promptly. The intent queue is bounded. Multiple `Play` intents in one drain
keep only the newest, so rapid selection does not load every intermediate
channel.

## Stale events and generations

Every `Play`, `Stop` and `Close` increments a generation. Engine events and
metrics carry that generation. The controller drops anything from an older
generation. The libmpv adapter adds a second filter:

1. `CommandAsync` uses the generation as `replyUserdata`; only the matching
   command reply opens the event barrier.
2. `playlist_entry_id` from that reply is compared with `StartFile` and
   `EndFile` events.

A late `End` from the previous source cannot stop the new one.

## Threading

| Goroutine | Responsibility |
|---|---|
| actor | intents, state transitions, generation, timers |
| notifier | copies snapshots and calls subscribers |
| libmpv pump | every client API call, event wait and property poll |

The UI never calls libmpv. Subscribers never touch widgets; the app wraps them
in `Dispatcher.Update`. Loading and network waits never run on the main
thread.

## Libmpv adapter

`internal/player/libmpv` uses pinned `gen2brain/go-mpv` 0.4.0. The real
adapter is compiled with:

```sh
CGO_ENABLED=0 go build -tags mumu_mpv ./...
```

Without the tag, a stub returns `ErrUnavailable`. This keeps `go test ./...`
and the CI cross-build free of libmpv headers and dylibs. The purego build
loads `libmpv.dylib` at package init; a missing runtime still panics before
`main`, as documented in the M0 report. Runtime-loader UX remains a release
issue.

Default options follow M0: `config=no`, `load-scripts=no`, `ytdl=no`, no
terminal/input bindings, `idle=yes`, `keep-open=no`, `network-timeout=10`,
`demuxer-max-bytes=32MiB`, `hwdec=auto-safe`, and `vo=null` for headless
control. `hwdec-current`, `current-vo` and `current-ao` are reported, not
assumed.

## Smoke test

The fixture and runtime from `experiments/nativeplayback/README.md` are
required. The test is skipped when `MUMU_HLS_TEST_URL` is unset, but the
package init still requires the dylib.

```sh
/private/tmp/mumu-hlsfixture -seconds 30

DYLD_LIBRARY_PATH=/private/tmp/mumu-runtime/lib \
MUMU_HLS_TEST_URL=http://127.0.0.1:18765/index.m3u8 \
CGO_ENABLED=0 go test -tags mumu_mpv -run TestIntegrationHLS -v \
  ./internal/player/libmpv
```

The test verifies load, playing, volume, mute, stop and close over a synthetic
H.264/AAC HLS stream. It does not render video and does not claim universal
codec support or hardware decoding. The recorded run is
[player-controller-smoke.log](evidence/player-controller-smoke.log).

## Limitations

- No native video surface in #5; Watch/PiP presentation belongs to #11/#12.
- No seek/reverse API in v0.1. Live streams are not assumed seekable.
- Native teardown can outlive `Close` when a context deadline expires.
- Event ordering is tested against the pinned libmpv runtime, not every
  version.
