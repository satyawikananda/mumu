# Verification record

9 November 2026, Asia/Makassar. Commands ran against the local workspace; no Git metadata was available. Runtime evidence is for macOS 27.0.1 / Apple M3 only.

## Follow-up status

The initial record below predates the missing-criteria follow-up. [Follow-up QA](followup-qa.md) now records calibrated capture analysis, whole-system GPU observations, maintainer-confirmed audio sync/continuity, pointer drag/resize, stacking, Spaces, foreign fullscreen and app hide/unhide. A native crash exposed by capture was mitigated with CGL context locking; three subsequent captured exercises completed 21 transfers with one session/load per run and exit 0. Raw failures remain archived.

[Final command results](followup-final-checks.json), [runtime assertions](followup-assertions.json) and [source hashes](followup-source-manifest.json) describe the follow-up source. M0 acceptance evidence is complete for this configuration under CONDITIONAL GO. OpenGL deprecation, captured visual gaps and broader production QA remain risks.

## Automated checks

| Command | Result |
|---|---|
| `gofmt -l experiments` | PASS, no output |
| `CGO_ENABLED=0 go test ./...` | PASS, root starter, fixture options and transfer-state tests |
| `CGO_ENABLED=0 go vet ./...` | PASS |
| `CGO_ENABLED=0 go vet -tags mumu_spike ./...` | PASS, includes native bridge |
| `CGO_ENABLED=0 go tool mygo vet .` | PASS |
| `CGO_ENABLED=0 go test -c -tags mumu_spike -o /private/tmp/mumu-spike-tests ./experiments/nativeplayback` | PASS |
| `DYLD_LIBRARY_PATH=... /private/tmp/mumu-spike-tests -test.v` | PASS; `raw output` (local run artifact, not committed) |
| `CGO_ENABLED=0 go build -tags mumu_spike ... ./experiments/nativeplayback` | PASS |
| `python3 -m py_compile experiments/nativeplayback/measure.py` | PASS |
| `measure.py ... SOURCE OUTPUT_DIRECTORY` | Live 720p, live 1080p, expanded/final 1080p and unreachable-source runs exited 0 |
| Evidence assertions | PASS: one session/start-file/file-loaded, one retained native view/context, no second load/render error, final teardown in passing live traces |

`GOCACHE=/private/tmp/mumu-go-cache` was set for Go commands in this sandbox. `golangci-lint` was not installed; it was not reported as run. No race detector, per-process GPU profiler, release packaging/signing or long-duration test was performed.

The direct `go test -tags mumu_spike ./...` launcher failed to preserve the runtime lookup environment. Native tests were instead compiled then executed directly with the library directory set. Early binding/runtime incompatibility, threading and close-handler failures are described in the feasibility reports; final passing evidence does not erase those diagnostics.

## Evidence index

- `720-vod-initial.log`: early VOD diagnostic, including the old fullscreen sync regression.
- `live1080-before-threadsplit*`: intermediate diagnostic, not final threading design.
- `live1080-threadsplit*`: dedicated render-thread diagnostic before explicit drawable reattachment.
- `live720/`: rolling live HLS 720p exercise and process samples.
- `live1080*`: rolling live HLS 1080p exercise and process samples.
- `visual1080.log`: manual computer-use session; two explicit Play loads because the first fixture expired. These are not transfer reloads.
- `unreachable/`: loopback connection-refusal load, lifecycle exercise and cleanup.
- `extended1080/`: expanded native lifecycle, including main recreation.
- `final1080/`: final source binary's expanded lifecycle and process samples.
- `measured-summary.json`: derived telemetry for the named runs, not a performance promise.

## Initial observed and unverified UI evidence

Computer-use capture showed the synthetic video within MyGo's main window and exposed native Play/PiP/Fullscreen/Quit/Volume and Return/Close PiP controls. Play, PiP and Return were exercised through native UI. The automated exercise calls the same action functions for the remaining controls/lifecycle operations, and checks source-level/native state rather than proving every pointer interaction.

Maintainer statements in this session: “Okay sudah terdengar” for audio and “Video PiP terlihat bergerak” for moving PiP video. These are human observations; the agent did not hear physical output or establish perceptual synchronization. Later computer-use capture intermittently failed with ScreenCaptureKit errors. Images remain in the conversation; no screenshot file was archived in this folder.

## Anti Slop delivery review (technical spike scope)

- PASS, direction: `DESIGN.md` charcoal surfaces, sparse tomato accent, native system text. No new logo/artwork, navigation or production screen was introduced.
- PASS, purpose/layout: video owns the surface; a reserved control strip keeps controls out of native video. PiP has Return, Close and Volume. No decorative cards, gradients, fake seek bar or availability claims.
- PASS, source/action review: every exposed control has a native handler, volume has an accessible label, and the error/idle/loading/playing/buffering/stopped states come from the worker rather than synthetic success text.
- PASS, measurement honesty: every reported number is tied to raw data; hardware decoding, resource samples and native-window observations are separated from zero-copy, GPU-load, stacking and seamlessness claims.
- PARTIAL, full interaction/quality gate: follow-up QA completed pointer drag/resize and cross-app stacking observations. Complete keyboard/focus, screen-reader and contrast-tool review remain production UI work. Native widget defaults and AX labels are not substitutes. This prototype remains a feasibility experiment.

ENERGY 1 / RHYTHM 1 / MOTION 1 for the diagnostic controls: deliberately quiet to keep attention on real video and native window transitions. Media test-pattern motion is diagnostic content. Production UI remains subject to the full design review in issue #4 and the later Watch/PiP issues.
