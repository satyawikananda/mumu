# Live pause and live-edge resume — 13 November 2026

Issue: [#37](https://github.com/satyawikananda/mumu/issues/37). Decision:
[ADR 0010](../../adr/0010-live-pause-and-recovery.md).

## Environment

- macOS 27.0.1, Apple arm64, Go 1.27.3.
- MyGo 0.9.0, pinned `go-mpv` 0.4.0 patch.
- libmpv from Homebrew mpv 0.42.0 (`/opt/homebrew/lib/libmpv.dylib`).
- Synthetic H.264/AAC HLS fixture from `experiments/hlsfixture`, 720p, bound to
  `127.0.0.1`, no third-party channel.

```sh
CGO_ENABLED=0 go build -o /private/tmp/mumu-hlsfixture ./experiments/hlsfixture
/private/tmp/mumu-hlsfixture -resolution 720 -seconds 120

MUMU_HLS_TEST_URL=http://127.0.0.1:18765/index.m3u8 MUMU_MPV_TEST=1 \
  go test -run TestIntegrationHLS -v ./internal/player/libmpv
```

## Result

`TestIntegrationHLS` passed. It exercises the real engine through the
controller:

1. load a live HLS stream and reach `playing`;
2. `Pause` reaches `paused`; the sampled `time-pos` did not move across two
   1.2-second samples, so the media clock is held;
3. `Resume` reloads the same source, passes through `buffering` and returns to
   `playing`;
4. volume, mute, stop and close still work.

`TestIntegrationEngineCreateLoadStop` also passed on the same runtime.

## Limitations

- The test uses `vo=null`: it proves the controller/engine pause and reload
  path, not a presented frame. Visual frame hold and audio stop need the native
  app run under Watch evidence.
- The synthetic stream is local; it says nothing about a provider's live edge,
  DRM, codec coverage or every libmpv build.
- Pause asks for `demuxer-readahead-secs=0`, but #32 measured a residual live
  read from playlist refresh and in-flight segments. Do not quote this run as a
  zero-network pause.
- The earlier Homebrew runtime is a development artifact, not the packaged
  LGPL bundle.
