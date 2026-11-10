# Release evidence — 10 November 2026

Machine: MacBook Air Mac15,12, Apple M3, 8 GB, macOS 27.0.1 (26A434).
Runtime: bundled LGPL build, mpv 0.42.0 / FFmpeg 9.0.2 / libplacebo 7.387.0.

> Log files are local run artifacts. `.gitignore` excludes `*.log`, so they are
> not committed. The commands in `docs/release/qa-matrix.md` reproduce them.

| File | What it records |
| --- | --- |
| `app-sandbox-status.txt` | the app stayed alive for 5 s under the sandbox |
| `idle/summary.txt`, `idle/samples.csv` | idle RSS/CPU sample from `packaging/qa/sample.sh` |

Local, ignored logs from this run: `go-test.log`, `go-test-race.log`,
`go-vet.log`, `mygo-vet.log`, `gofmt.log`, `golangci-lint.log`,
`verify-app.log`, `codesign.log`, `spctl.log`, `make-package.log`,
`libmpv-720.log`, `libmpv-1080.log`, `soak-720-30s.log`,
`soak-1080-180s.log`, `bundle-sandbox.log`, `app-sandbox.log`,
`collect-sources.log`.

Missing: full app playback with the native surface, PiP, 60-minute soak,
sleep/wake, external display, network transitions, failure cases, and
Gatekeeper download flow. See `docs/release/qa-matrix.md`.
