# Native playback / PiP spike

Standalone M0 experiment for [#1](https://github.com/satyawikananda/mumu/issues/1) and [#2](https://github.com/satyawikananda/mumu/issues/2). The root starter application is unchanged. These commands launch the spike, not the production mumu application.

Read the [playback decision](../../docs/architecture/native-playback-feasibility.md) and [PiP decision](../../docs/architecture/pip-feasibility.md) before reusing this bridge.

## Requirements

- macOS arm64, Go 1.27.3, MyGo 0.8.3, go-mpv 0.4.0 (pinned in root module).
- A compatible arm64 libmpv with render API and `mpv_get_time_ns`. `nocgo` does not supply the native runtime.
- FFmpeg with libx264 and AAC, for the original test fixture. No third-party TV channel is used.
- Python 3 for the process/GPU samplers; NumPy only for optional capture analysis (tested Python 3.12.6 / NumPy 2.1.2).
- Swift/Apple command-line SDK and Screen & System Audio Recording permission only for the separate diagnostic recorder.

## Isolated runtime used in this experiment

This is a third-party development artifact, not an approved redistribution bundle. Review its provenance and licenses before use. It was downloaded into `/private/tmp`, without upgrading global Homebrew libraries.

```sh
curl -fL --max-time 120 \
  https://github.com/vegidio/binaries-mpv/releases/download/26.10.1/dynamic_osx_arm64.zip \
  -o /private/tmp/mumu-libmpv.zip
echo '37e93dea19e511178b7b6241f275ddefb3f3aea789351c14c4a58ac060e120a3  /private/tmp/mumu-libmpv.zip' | shasum -a 256 -c -
mkdir -p /private/tmp/mumu-runtime
unzip -q /private/tmp/mumu-libmpv.zip -d /private/tmp/mumu-runtime
export DYLD_LIBRARY_PATH=/private/tmp/mumu-runtime/lib
```

Keep all dylibs together. The spike calls `mpv.Load(nil)`, which opens `libmpv.dylib` / `libmpv.2.dylib` through the dynamic loader, then `mpv.New()`; an incompatible library is reported as an error. The application adapter uses the same patched binding but searches more locations (see `docs/architecture/player-controller.md`). The tested bundle reported mpv 0.41.0, client API 2.5, FFmpeg 9.0.2. Media-kit v0.7.3 lacked `mpv_get_time_ns` and was rejected. An attempted Homebrew install was canceled during fetching because it proposed many unrelated upgrades.

## Build and run

From the project root:

```sh
CGO_ENABLED=0 go build -tags mumu_spike -o /private/tmp/mumu-native-spike ./experiments/nativeplayback
CGO_ENABLED=0 go build -o /private/tmp/mumu-hlsfixture ./experiments/hlsfixture
/private/tmp/mumu-hlsfixture -resolution 720 -seconds 180
```

Wait for `READY`. In another terminal, export the same `DYLD_LIBRARY_PATH`, then:

```sh
/private/tmp/mumu-native-spike -source http://127.0.0.1:18765/index.m3u8
```

Click **Play**. Normal launch never starts media automatically. The fixture is a rolling six-segment HLS playlist with no ENDLIST, H.264 30 fps, AAC audio, and a white flash / 880 Hz beep every second. It binds only to `127.0.0.1`, deletes its own temporary directory on exit, and has a maximum duration of 600 seconds. It runs an FFmpeg *fixture generator*, never an external mpv player.

For 1080p, stop the fixture and restart with `-resolution 1080`. `-port 18766` allows a second explicitly chosen local test server. The spike does not validate arbitrary catalogs or implement hardened playlist importing. Use only authorized inputs; network playback is bounded by a 10-second mpv network timeout and 32 MiB demux cache limit.

## Repeatable exercise and resource sampling

```sh
python3 experiments/nativeplayback/measure.py \
  /private/tmp/mumu-native-spike \
  http://127.0.0.1:18765/index.m3u8 \
  build/spike-evidence
```

`-exercise` explicitly starts playback, then exercises main resize, PiP, volume zero/restore, main minimize, Close PiP with main present, main hide, Return, fullscreen, PiP resize/move, main close, Return recreating main, and Close PiP without main. A 36-second quit timer and 75-second sampler limit bound the run. `attachment_ms` measures the attachment operation, not end-to-end interruption or sound continuity. `render-gap-ms` includes initial idle/startup gaps and must be interpreted alongside events.

Test failure handling by passing `http://127.0.0.1:1/missing.m3u8` instead. There is no stream reload in the transfer function; Play is the sole `loadfile` caller. Logs omit source URLs and mpv's raw logs to avoid leaking playlist credentials.

Choose a fresh output directory for every measurement. The sampler refuses to overwrite earlier evidence.

## Capture audio/video and observe GPU

The optional recorder is a diagnostic executable, separate from the Go/MyGo player. It captures the selected spike PID's windows/audio without the microphone. Its normal recording duration is 27 seconds; the direct recorder limits a requested recording to 120 seconds. Full recordings stay under ignored `build/`; the evidence reports preserve hashes and selected synthetic-content frames.

Build the recorder and prepare the optional analysis environment:

```sh
mkdir -p build
swiftc -parse-as-library -target arm64-apple-macosx14.0 \
  experiments/nativeplayback/capture.swift -o build/mumu-capture
python3 -m venv build/qa-venv
build/qa-venv/bin/python -m pip install numpy==2.1.2
```

If recording exits with `CAPTURE_PERMISSION_REQUIRED`, request access with `build/mumu-capture --request-access`, then complete macOS permission/authentication yourself in System Settings and rerun the recorder. The OS permission is broader than the recorder's PID filter. Merely compiling or running the Go player does not require recording permission.

Stop the previous fixture, then start a fresh continuous-tone fixture:

```sh
/private/tmp/mumu-hlsfixture -resolution 720 -seconds 600 -continuous-audio
```

Wait for `READY`. In another terminal with `DYLD_LIBRARY_PATH` exported:

```sh
python3 experiments/nativeplayback/measure.py \
  /private/tmp/mumu-native-spike http://127.0.0.1:18765/index.m3u8 \
  build/capture720 --gpu --capture build/mumu-capture
build/qa-venv/bin/python experiments/nativeplayback/analyze_capture.py \
  build/capture720/capture.mov build/capture720/analysis.json
```

Repeat serially with a restarted `-resolution 1080` fixture and a new output directory. `--gpu` takes six prelaunch baseline samples, samples the exercise, then takes three postexit samples. These `ioreg` counters describe the whole system, including capture and other apps. They do not measure mumu's GPU share. `observe_gpu.py OUTPUT.jsonl --seconds 45` can also sample independently, with a 120-second limit.

The capture analyzer detects the fixture's flash/beep offsets, low-audio intervals and repeated/absent images. Its calibration test generates a known 120 ms audio delay, 200 ms mute and black interval. Interpret startup, intentional volume-zero, native window animations, capture failures and the capture boundary separately. Entire-window absence in a filtered recording cannot establish player-only blackout duration. Physical output latency and per-process GPU usage remain unmeasured.

Manual QA additionally covers drag/resize, another app foreground, Desktop/Spaces, another app's fullscreen, app hide/unhide, Return, Close PiP and main close. Record human observations separately from counters. [Follow-up evidence](../../docs/architecture/evidence/followup-qa.md) documents the completed session.

## Checks

```sh
gofmt -l experiments
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go vet -tags mumu_spike ./...
CGO_ENABLED=0 go tool mygo vet .
CGO_ENABLED=0 go test -c -tags mumu_spike -o /private/tmp/mumu-spike-tests ./experiments/nativeplayback
/private/tmp/mumu-spike-tests -test.v
build/qa-venv/bin/python -m unittest discover \
  -s experiments/nativeplayback -p test_capture_analysis.py -v
```

On this machine, invoking native tests through `go test` lost `DYLD_LIBRARY_PATH` across the toolchain launcher. Compile then execute the test binary directly with the runtime variable exported. The default untagged tests do not load libmpv. `golangci-lint` was unavailable; `.golangci.yml` records the intended configuration.

The native bridge is intentionally limited to arm64. A development `.app` wrapper was used only for computer-use inspection; it is not release packaging, signing, or a bundled runtime solution.
