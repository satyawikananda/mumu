# Release checklist

Issue: [#14](https://github.com/satyawikananda/mumu/issues/14). Packaging:
[macos](macos.md). QA: [qa-matrix](qa-matrix.md).

## Before tagging

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `go tool mygo vet .`
- [ ] `gofmt -l ./cmd ./internal` prints nothing
- [ ] `golangci-lint run ./cmd/... ./internal/...`
- [ ] `make package`
- [ ] `packaging/macos/verify-app.sh` passes
- [ ] `spctl -a` is expected to reject the unsigned app
- [ ] QA matrix items that can run are recorded with evidence
- [ ] The v0.1.0 blocker regression checks in [qa-matrix](qa-matrix.md) are
      recorded: PiP with a closed main window, PiP repaint, closing PiP or
      quitting while the main window is gone, audible launch, and a failed
      switch without a stale frame
- [ ] `THIRD_PARTY_NOTICES.md` matches the runtime manifest
- [ ] `packaging/macos/mpv-runtime.json` hashes still match the sources
- [ ] No Homebrew path appears in `otool -L` output
- [ ] The DMG mounts and the app inside passes `codesign --verify --deep --strict`

## Artifacts

- [ ] `mumu-<version>-arm64.dmg`
- [ ] `mumu-libmpv-<version>-arm64.tar.gz`
- [ ] `mumu-libmpv-<version>-arm64-src.tar.gz`
- [ ] `SHA256SUMS`
- [ ] `latest.json`
- [ ] `THIRD_PARTY_NOTICES.md`

## Release notes

- [ ] Version and minimum macOS (13.0)
- [ ] Apple Silicon only; Intel not supported in v0.1
- [ ] Unsigned: System Settings > Privacy & Security > Open Anyway
- [ ] SHA-256 verification instructions
- [ ] LGPL runtime source link
- [ ] Known limitation: no VideoToolbox interop in this runtime
- [ ] Known limitation: no notarization

## Acceptance criteria

- [x] Reproducible Apple Silicon build exists; documented requirements and
      verified launch. Evidence: `verify-app.log`, `app-sandbox.log`.
- [x] App runs without the user installing libmpv. Evidence: bundled runtime,
      `bundle-sandbox.log`, `otool -L` closure gate.
- [x] No WebView, HTML/JS, separate mpv window, backend or auto-login. The
      bundle gate rejects web assets and framework directories.
- [ ] One-player native Watch and PiP. Basic Watch/PiP confirmed by the
      maintainer on 10 November 2026; stress toggles and cleanup still need a
      recorded run. See `qa-matrix.md`.
- [ ] 60-minute 720p/1080p, offline and failure cases. Not run; see
      `qa-matrix.md`.
- [ ] CPU/RAM/GPU measurements. Not run; see `qa-matrix.md`.
- [x] Security/licensing/attribution audit and release checklist. This file,
      `THIRD_PARTY_NOTICES.md`, ADR 0008.
- [x] Unsigned distribution documented; signing/notarization steps are out of
      scope for v0.1 and recorded as a manual follow-up.

Log files are local artifacts and are ignored by git; commit summaries and
reproduce the commands instead.

Unfinished boxes block calling #14 complete. They do not block publishing a
pre-release alpha, but the limitation must be stated in the release notes.
