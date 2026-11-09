# Architecture evidence and phase benchmarks

Date: 9 November 2026. This document records the maintainer discussion about feasibility, the lightweight principle, and verification throughout development. It does not implement issue #3 or claim that future benchmarks have run.

The engineering constraints remain in [AGENTS.md](../AGENTS.md), and the interaction contract remains in [DESIGN.md](../DESIGN.md). Detailed M0 findings remain authoritative in the [native playback report](architecture/native-playback-feasibility.md), [PiP report](architecture/pip-feasibility.md), and [follow-up QA evidence](architecture/evidence/followup-qa.md).

## 1. Feasibility status and permission to proceed

The feasibility scope of issues #1 and #2 is complete for the tested configuration, with a **CONDITIONAL GO** result. This is distinct from production playback/PiP completion and from closing the GitHub issues.

- Issue #1 established a working native embedding path and recorded initial measurements and limitations.
- Issue #2 established Watch and PiP as presentations of one retained video view and one libmpv session. The maintainer confirmed drag, resize, stacking, Spaces, foreign fullscreen, continuous audio, perceived beep/flash synchronization, and app hide/reopen behavior.
- The tested configuration was macOS 27.0.1 on Apple M3, with MyGo 0.8.3, go-mpv 0.4.0, and mpv 0.41.0. These findings do not establish support across other hardware, OS versions, displays, codecs, or providers.

The result is sufficient to proceed to issue #3, the bootstrap milestone. Additional efficiency profiling is not a prerequisite for starting #3. The remaining risks must travel with the implementation and be revisited as relevant features become available.

The absence of Git metadata in the working copy used for M0 prevented commit/push delivery there. Versioning and GitHub issue administration are separate from runtime feasibility and efficiency evidence.

## 2. What the lightweight principle requires

MyGo remains the chosen native UI framework. The evidence does not identify MyGo itself as a bottleneck. Native UI alone does not prove that the whole application is lightweight.

Application cost includes UI rendering, libmpv and its native dependencies, decoding, video buffers, surface composition, storage, and background work. Assess the complete application rather than only its Go executable or UI framework.

Watch and PiP must continue to share one logical playback session. Do not address a performance problem by introducing a second decoder, external player, or WebView. Any renderer change needs its own evidence and must preserve the project constraints.

## 3. Risks retained from M0

### Presentation gaps

Captured repeated-image intervals reached approximately 400 ms. Some transitions also removed the entire filtered application image, including window chrome. The existing captures cannot separate all player stalls from compositor or recorder behavior.

This is a responsiveness and presentation risk. It is not proof that every observed gap is a player freeze. Reproduce visible problems without a recorder, then correlate presentation changes with render time and lock waits. Do not claim frame-perfect transitions from the current evidence.

### OpenGL and synchronization

The tested bridge places an NSOpenGLView beside MyGo's Metal UI. It does not render libmpv directly into MyGo's Metal renderer. OpenGL deprecation remains a compatibility and maintenance risk; it does not by itself establish excessive resource use.

A captured run exposed a native crash during PiP transfer. The bridge now serializes rendering and drawable changes with a Go mutex and native CGL context locking. Subsequent successful runs support that mitigation, but do not establish recovery from arbitrary native context failures.

Render duration and time waiting for these locks are profiling candidates. They are not yet proven bottlenecks. A future Metal integration would require a separate feasibility experiment; replacing the adapter is not assumed to be trivial or automatically faster.

### Incomplete efficiency attribution

M0 GPU observations describe the whole system, including the recorder, compositor, fixture generator, and other applications. They cannot establish mumu's GPU cost or a causal comparison between resolutions.

Per-process GPU attribution, energy use, thermal behavior, and sustained playback remain open measurements. Initial CPU/RAM observations are useful diagnostic evidence, not a controlled production baseline or a battery-life claim. Hardware decoding and zero-copy rendering must also be verified before being claimed.

## 4. Decision: benchmark at the end of every phase

Run a formal benchmark review at the end of each implementation phase, M1 through M4. Evaluate size, speed/resource use, and flexibility using the capabilities implemented by that phase.

M1 establishes the first application baseline. Preserve M0 measurements as feasibility evidence rather than silently treating them as that baseline. Compare later phases with the M1 baseline and the preceding comparable phase.

Keep targeted checks during development when changing rendering, threading, dependencies, parsing, storage, or other sensitive paths. Phase reviews do not justify postponing a known crash or functional regression until the phase ends.

### Size

- Measure the application executable separately from the runnable application bundle.
- Include libmpv, required native dependencies, and assets in the runnable footprint. Disclose dependencies that are still supplied externally during development.
- Record cache and local data growth separately from installed application size.
- At release, distinguish the download artifact size from the installed bundle size.

### Speed and resource use

- Startup: process launch until the UI is ready for interaction. Startup must remain idle, without automatic playback.
- Playback start: explicit play intent until the first presented video frame and audible output, where measurable. Separate controlled local fixtures from network-dependent sources.
- Watch/PiP transitions: action latency, visible continuity, and audio continuity. Native attachment completion alone does not prove a frame was presented.
- CPU and process memory: idle, steady playback, transitions, and sustained sessions. Use total process measurements alongside Go-specific profiles because Go heap size excludes native allocations.
- Energy and GPU: measure when suitable tools are available, with the attribution and limitations stated. Whole-system values must not be labeled as per-process values.
- When a gap or regression appears, profile render time, native calls, and lock waits before choosing an optimization.

### Flexibility

Evaluate flexibility as a scenario matrix, not a single numerical score:

- Resize, move, hide, reopen, and close windows while preserving the intended session lifecycle.
- Exercise Watch/PiP transitions, Spaces, stacking, and foreign fullscreen where those features are implemented.
- Test supported source/format variations, playlist sizes, cancellation, failures, and recovery with authorized fixtures.
- Review whether player lifetime remains separate from video presentation, and whether a platform adapter can change without rewriting controller behavior. Use existing boundaries and focused tests; do not build speculative adapters just to claim flexibility.

Passing this matrix on macOS does not establish Windows or Linux support.

## 5. Phase coverage

| Phase | Benchmark focus |
| --- | --- |
| M1 — Foundation, #3–#6 | Establish the baseline: startup, executable and available runtime footprint, idle CPU/RAM, basic integrated playback, and relevant storage operations. |
| M2 — Sources, #7–#9 | Repeat applicable M1 scenarios; measure large-playlist parsing/import, cancellation, cache/data growth, and UI responsiveness during source operations. |
| M3 — Product UX, #10–#13 | Repeat comparable scenarios; cover Watch/PiP transitions, resize and window lifecycle, channel switching, and sustained playback. Investigate reproducible presentation gaps. |
| M4 — Release, #14 | Measure the actual distribution bundle, final dependency footprint, startup/playback regressions, sustained stability, and energy/GPU where measurable. Review remaining evidence gaps before release. |

Only evaluate implemented capabilities. Mark unavailable features or tools as not measured, with a reason and follow-up phase. Do not replace missing runtime evidence with a compile result.

## 6. Repeatable measurement procedure

Use the same hardware, fixture, resolution, and procedure for comparisons. Record OS, architecture, dependency versions, display configuration, build mode, and power conditions. If these change, identify the result as a new or non-comparable baseline.

Record the source revision, or source hashes when revision metadata is unavailable. Retain exact commands, tool versions, fixture provenance, sample counts, sampling intervals, warm-up periods, and run durations.

Compare an idle UI with controlled 1080p playback when available. Keep the fixture codec, frame rate, delivery method, and decoder settings fixed. Record the decoder actually used rather than inferring it from a requested setting.

Measure CPU/RAM/energy without screen recording. Keep capture-based continuity diagnostics separate because capture changes system load. Disclose fixture generation and other background workloads; avoid concurrent benchmark runs on the same machine.

Repeat measurements and retain raw results. Report typical values, peaks, and variation with explicit units. Define whether startup is a cold or warm launch, and document the CPU/memory metric used. Do not compare differently defined measurements as if they were equivalent.

Keep automated observations, human confirmations, and interpretations separate. A synchronization check from a person is useful UX evidence but is not a calibrated physical output-latency measurement.

## 7. Targets, review outcomes, and follow-up

No numerical performance budget has been approved yet. Set initial targets from the M1 baseline and product needs; do not invent pass thresholds or treat the first measurement as inherently acceptable. Define later comparison criteria before evaluating a candidate change.

At each phase review, record:

1. Implemented scenarios, environment, commands, and raw evidence locations.
2. Size, speed/resource measurements, and the flexibility scenario results.
3. Changes against comparable baselines, including uncertainty and instrumentation overhead.
4. Failures, regressions, and measurements that remain unavailable.
5. The decision to proceed, proceed with explicit follow-up, or fix a blocker before dependent work continues.

A crash, broken single-session invariant, or reproducible functional failure requires attention when discovered. For efficiency regressions, identify the cause and impact before selecting a fix. Optimize one demonstrated bottleneck at a time and remeasure using the same procedure.

Place future phase reports and raw evidence under `docs/architecture/`, alongside the existing feasibility reports. Link each report to the relevant implementation issues. Release review must explicitly evaluate remaining presentation, efficiency, compatibility, and redistribution constraints; missing measurements cannot support unqualified lightweight or battery-efficiency claims.

This policy allows foundation work to proceed now while making evidence collection recurring work throughout development, rather than a single validation exercise at the end of the project.
