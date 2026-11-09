# M0 follow-up acceptance evidence

9 November 2026, Asia/Makassar. Tested configuration remains macOS 27.0.1 / Apple M3, Go 1.27.3, MyGo 0.8.3, go-mpv 0.4.0 and the previously identified mpv 0.41.0 runtime. The decision remains **CONDITIONAL GO**. The missing M0 acceptance evidence is now recorded for this configuration; production playback and PiP remain separate milestones.

## Implementation and reproduction

The Go application still uses one libmpv client, one render context and one retained native video view. A recording exposed a native SIGSEGV after first PiP attachment. The bridge now uses native CGL locking alongside its Go mutex and makes the renderer's context current before drawing. The failed [trace](capture-locking-crash.log) is retained. Three subsequent captured exercises completed normally; this supports the mitigation without proving recovery from arbitrary driver/context failure.

The fixture's optional `-continuous-audio` adds a quiet 220 Hz tone to the existing one-second 880 Hz beep/white flash. This makes interruptions during the space between beeps measurable. The separate Swift recorder uses ScreenCaptureKit to select only the spike PID. It does not capture the microphone or add recording UI to mumu. The maintainer approved the OS capture permission and completed OS authentication themselves.

Build/run instructions, including capture permission handling and analysis, are in the [spike README](../../../experiments/nativeplayback/README.md). The measured invocations were:

```sh
/private/tmp/mumu-hlsfixture -resolution 720 -seconds 600 -port 18765 -continuous-audio
DYLD_LIBRARY_PATH=/private/tmp/mumu-libmpv-modern/lib python3 experiments/nativeplayback/measure.py \
  '/private/tmp/Mumu Feasibility.app/Contents/MacOS/Mumu Feasibility' \
  http://127.0.0.1:18765/index.m3u8 build/spike-captures/continuous720-cgl \
  --gpu --capture /private/tmp/mumu-capture
```

The fixture was stopped before replacing it with `-resolution 1080`. The same measurement command used `build/spike-captures/continuous1080-cgl`. A further 1080p run used `build/spike-captures/repeat1080-cgl --capture /private/tmp/mumu-capture`, without GPU sampling. Manual runs launched the same native binary without `-exercise`; Play was clicked explicitly. The normal launch remained idle.

## Recorded outcomes

- [720p](continuous720-cgl/run.json), [1080p](continuous1080-cgl/run.json) and [repeated 1080p](repeat1080-cgl/run.json): app and recorder exit 0, seven transfers each, one session/load/view/context per run, and completed teardown. [Assertions](followup-assertions.json) check these properties against the raw traces.
- [Manual trace](manual-followup.log): native pointer move/resize callbacks, one session/load, Return and Close PiP with main present, then actual main-window close and completed teardown. The agent observed the native controls return to playing after transfer. The maintained screenshot evidence contains only mumu and the original synthetic fixture.
- [App hide trace](app-hide-followup.log): one session/load and completed teardown. The trace does not itself prove window visibility; the maintainer observations below supply that evidence.
- [Source hashes](followup-source-manifest.json): current diagnostic sources and native binaries after QA. Earlier named traces precede some logging/fatal-error-only changes; without Git metadata this is not a historical commit identifier.

The maintainer explicitly reported:

| Manual scenario | Reported observation |
|---|---|
| Drag titlebar and resize corner | Both work; video keeps moving |
| Another application foreground, Desktop/Space switch, another application's fullscreen | PiP remains visible above and moving in all three cases |
| Return/PiP actions | Audio continues and beep/flash feels synchronized |
| Cmd+H, reopen via Dock | Windows hide, audio continues, moving PiP returns |

These are human observations on one configuration, not independent agent listening or a hardware latency measurement.

## Measurement interpretation

[Summary](followup-summary.json), [720p analysis](continuous720-cgl/analysis.json) and [1080p analysis](continuous1080-cgl/analysis.json) preserve all detected intervals and paired flash/beep offsets. The content analyzer samples video at 60 Hz and audio in 10 ms RMS windows. It distinguishes the 880 Hz beep from the 220 Hz tone spectrally, so volume changes do not become false beep events.

A generated calibration with +120 ms audio delay measured median +118 ms. The [automated calibration](capture-calibration-test.log) additionally verifies detection of a known 200 ms mute and black interval. This tests the analysis pipeline, not the native player's behavior.

Captured audio-minus-video medians are -15.313 ms at 720p and -44.188 ms at 1080p. Ranges, including transitions, are -131.980 to -15.313 ms and -60.855 to -27.521 ms. The captures detect loading silence, the approximately 990 ms intentional mute, and a final 10 ms audio boundary interval. No further low-audio interval of at least 20 ms appears during the analyzed playing portion. OS capture, AAC and physical output latency limit interpretation.

Visible repeated-image intervals reach approximately 317 ms at 720p and 400 ms at 1080p. Some resize/fullscreen transitions remove the entire filtered application image for up to 2.783 s / 1.483 s. The absence of even the window chrome prevents attributing these intervals solely to the player. These observations remain evidence of capture/presentation limitations; they do not support a frame-perfect transition claim.

`ioreg` reports whole-system GPU device utilization: prelaunch/exercise medians 2%/29% at 720p and 20%/35% at 1080p, with exercise maxima 48% and 56%. The recorder, compositor, fixture generator and other applications were present. Six prelaunch samples, variable exercise sample counts and different baselines do not support causal attribution or resolution comparisons. Per-process GPU profiling remains unavailable because `xctrace` is not installed.

## Local recordings and limits

Full recordings are kept locally under the ignored `build/spike-captures/` directory. The two analyzed recordings' paths and SHA-256 hashes are in [the summary](followup-summary.json). Archived still frames are [main](continuous720-cgl/main.png), [720p PiP](continuous720-cgl/pip.png) and [1080p PiP](continuous1080-cgl/pip.png). Older failed/black captures remain diagnostic failures, not passing visual evidence.

OpenGL remains deprecated. Broader macOS versions, displays/scales, sleep/wake, long sessions, codec/provider breadth and runtime redistribution require later QA. CGL lock/unlock failure during final cleanup is explicitly fatal to avoid destroying mpv while its render context is live; that unrecoverable native-fault path was not injected. No race detector, per-process GPU benchmark or release signing/notarization was performed. `golangci-lint` remains unavailable.

The authoritative command outcomes for the final source are recorded in [final checks](followup-final-checks.json). GitHub issue checkboxes, commits and PRs were not changed from this workspace.
