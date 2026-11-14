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
