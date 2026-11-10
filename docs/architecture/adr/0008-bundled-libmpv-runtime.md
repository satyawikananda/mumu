# ADR 0008 — Bundle an LGPL libmpv runtime and distribute unsigned

Date: 10 November 2026 · Issue: [#14](https://github.com/satyawikananda/mumu/issues/14)

## Context

Earlier milestones required a separately installed libmpv. The feasibility
spikes used Homebrew or a third-party build. Both are GPLv3 builds: FFmpeg was
configured with `--enable-gpl --enable-version3`, and `libavutil` reports
"GPL version 3 or later". Shipping that inside an MIT app would make the
distributed whole GPL.

Issue #14 requires an app that runs without a user installing libmpv. It also
requires a licensing audit based on the actual distribution build, not the Go
binding's license.

## Decision

1. mumu stays MIT.
2. The macOS app bundles a runtime built from pinned sources with GPL-only
   components disabled:
   - mpv 0.42.0 with `-Dgpl=false`
   - FFmpeg 9.0.2 with `--disable-gpl --disable-version3 --disable-nonfree`
   - libplacebo 7.387.0 with OpenGL only
   - libass, FreeType, FriBidi, HarfBuzz from pinned sources
3. The runtime is built by `packaging/macos/build-lgpl-runtime.sh`, flattened
   to `@loader_path` by `packaging/macos/prepare_runtime.py`, and placed in
   `Contents/Resources/libmpv`.
4. Distribution is unsigned: ad-hoc signature only, GitHub Releases, no
   Developer ID and no notarization for v0.1. Gatekeeper steps and checksum
   verification are documented for users.
5. The source, build flags and licenses ship with each release.

## Alternatives considered

- **Bundle the GPLv3 Homebrew or M0 runtime and relicense mumu as GPLv3.**
  Fastest, but forces the whole app under GPL and changes the project's stated
  license.
- **Keep requiring a separate libmpv install.** Fails the release criterion and
  makes a fresh download unusable without Homebrew.
- **Sign and notarize with a Developer ID.** Best user experience, but needs a
  paid Apple Developer Program membership. The pipeline leaves room for it:
  set `macos.signingIdentity` and `macos.notarize` and add CI secrets later.
- **Build a universal binary.** Intel is untested; the issue only asks for
  Apple Silicon. The app remains arm64, and other targets compile the stub.

## Consequences

- The download is about 40 MB of app (25 MiB runtime) instead of 14 MB.
- The bundled mpv build has no VideoToolbox interop, so decoding falls back to
  software. This differs from the M0 hardware-decoding observation and is
  recorded in the QA matrix rather than claimed as a regression-free change.
- Without notarization, every browser download hits Gatekeeper once. This is a
  documented pre-release limitation, not a bug.
- LGPL replacement is possible: `MUMU_MPV_LIBRARY` overrides the bundled
  directory, and the runtime source ships with the binary.
- Rebuilding the runtime needs meson, ninja, cmake, pkg-config and python3.
  Release users need none of them.
