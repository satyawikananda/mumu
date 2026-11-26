# Validation

This page records what has actually been exercised and what remains unverified.
Raw capture files, logs and machine-specific artifacts are not distributed with
the repository.

## Tested configuration

| Item | Value |
|---|---|
| Hardware | Apple Silicon, M3 |
| OS | macOS 27.0.1 |
| UI toolkit | MyGo 0.9.0 |
| Playback | libmpv via the bundled LGPL runtime |
| Go | 1.27.3 |
| Video output | retained OpenGL view under MyGo's surface |

Results apply to this configuration. They are not a claim about Intel Macs,
other Apple Silicon models, other macOS versions or every display setup.

## Headless engine verification (#54)

The headless subset was exercised on the same Mac. It uses the bundled LGPL
runtime, `vo=null`/`ao=null` and an original synthetic MPEG-2/MP2 HLS fixture;
it does not open a window.

| Item | Value |
|---|---|
| OS / architecture | macOS 27.0.1 / arm64 |
| Go | 1.27.3 |
| Runtime | mpv 0.42.0, FFmpeg 9.0.2, client API 2.6 (`libmpv.2.dylib`) |
| Command | `MUMU_MPV_LIBRARY=... MUMU_MPV_INCLUDE=... bash packaging/qa/test-headless.sh` |
| Fixture | `internal/player/libmpv/testdata/synthetic.mpegts` |

Executed and passed:

- `TestIntegrationEngineCreateLoadStop`: real decode, video and audio metadata,
  volume, mute, pause holding the position, resume progress, cancellation,
  stop, idempotent close and closed-engine errors.
- `TestIntegrationPlaybackHeaders`: the sanitized `User-Agent` and `Referer`
  reached the server; a `Cookie` header did not.
- `TestIntegrationPlaybackAddressPolicy`: the strict policy refused loopback
  without a request, explicit consent reached the fixture, and consent did not
  leak to the next source.
- `TestIntegrationLoaderRecovery`: wrong architecture, incompatible API and a
  missing-symbol runtime each failed without publishing state; the same process
  then loaded a valid runtime and created a client.
- `TestIntegrationABI`: a C probe compiled against the pinned headers matched
  `unsigned long`, event, node and byte-array sizes and offsets.

Observed fixture metadata: `video=MPEG-2 video`, `audio=mp2`,
`dimensions=160x90`, `vo=null`, `ao=null`. The ABI probe reported
`[8 24 16 16 8 24 8 16 32 8]` for the ten sizes and offsets.

The runtime log and failure messages identify only `libmpv.2.dylib`, not its
absolute directory, and the test output above was reduced accordingly.

The Linux AMD64 and Windows AMD64 jobs in
[`.github/workflows/headless.yml`](../../.github/workflows/headless.yml) build the
same pinned sources with SHA-256 verification and run the same required tests.
Their evidence is uploaded as `headless-evidence-*` artifacts. No Linux or
Windows run is recorded in this document yet, so native behavior on those
targets remains unverified here.

## Verified behavior

- Native MyGo window with embedded libmpv video; no WebView and no external mpv
  window.
- One logical playback session, one retained native view and one render context
  across Watch and PiP transfers.
- Watch playback with video and audio, pause, live-edge resume and bounded
  recovery.
- PiP enter, return and close without a deliberate stream reload.
- Resize, fullscreen, drag, minimize/hide, Spaces and another app's fullscreen
  observed on the tested Mac.
- Successful package build with the bundled runtime and SQLite native module.
- Local SQLite persistence for sources, favorites, recent channels and settings.
- Source import, validation and atomic refresh on the tested configuration.

## Known gaps

- No 60-minute or longer soak run with recorded results.
- No per-process GPU, energy or thermal attribution.
- No frame-perfect presentation or zero-copy rendering claim.
- No broad codec, provider, DRM or authenticated-source coverage.
- Sleep/wake and multi-display behavior are not established.
- The bundled runtime has no VideoToolbox interop and decodes in software.
- Native macOS accessibility, contrast and reduced-motion checks remain part of
  release QA.
- Downloaded-DMG Gatekeeper flow and a clean second Mac are not recorded.

## Reproducing checks

The repository keeps reproducible commands in [Testing](../release/testing.md).
A skipped integration test is not a passing result. Claims about playback
require a real macOS run with a supported runtime and an authorized fixture.
