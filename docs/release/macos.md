# macOS packaging and release

Issue: [#14](https://github.com/satyawikananda/mumu/issues/14). Decision:
[ADR 0008](adr/0008-bundled-libmpv-runtime.md). QA matrix:
[qa-matrix](../release/qa-matrix.md).

Status: the app bundles its own LGPL libmpv runtime, builds reproducibly on
Apple Silicon, and is distributed unsigned through GitHub Releases. Signing
and notarization are out of scope for v0.1.

## Requirements

- macOS 13 or later on Apple Silicon (arm64). The Go binary carries a 13.0
  minimum; the bundled runtime is built for 12.0 or later, so the app minimum
  is 13.0.
- Intel Macs are not supported in v0.1. Other targets compile the libmpv stub
  and are not release artifacts.

## What ships

The app bundle is arm64 only:

```text
Mumu.app/Contents/MacOS/Mumu
Mumu.app/Contents/Resources/libmpv/     13 dylibs, about 25 MiB
Mumu.app/Contents/Resources/licenses/
Mumu.app/Contents/Resources/THIRD_PARTY_NOTICES.md
Mumu.app/Contents/Resources/libSqlitecore.dylib
```

`libSqlitecore.dylib` is MyGo 0.9's SQLite native module, built from the Zig
sources in the pinned module. `mygo build` copies it next to `libmpv` and signs
it with the app; the app loads it from `Contents/Resources` when it opens its
local library, so a packaged app never downloads native code at run time.

`Mumu.app` is about 40 MB and the DMG about 14 MB with the pinned runtime.

## Pinned versions

| Component | Version | License |
| --- | --- | --- |
| Go | 1.27.3 | BSD-3-Clause |
| MyGo | 0.9.0 | MIT |
| go-mpv | 0.4.0, vendored in `third_party/go-mpv` | MIT |
| libmpv | 0.42.0, `-Dgpl=false` | LGPL-2.1-or-later |
| FFmpeg | 9.0.2, `--disable-gpl --disable-version3` | LGPL-2.1-or-later |
| libplacebo | 7.387.0, OpenGL only | LGPL-2.1-or-later |
| libass | 0.17.5, CoreText, no fontconfig | ISC |
| FreeType | 2.13.3 | FTL or GPL-2.0-or-later |
| FriBidi | 1.0.16 | LGPL-2.1-or-later |
| HarfBuzz | 15.1.0 | MIT |

`packaging/macos/mpv-runtime.json` records sources, SHA-256 hashes and the exact
build flags. `packaging/macos/licenses/` holds the license texts.

## Build the runtime

Maintainer-only. A user of a release needs none of these tools.

```sh
brew install meson pkg-config cmake ninja
make runtime
```

`packaging/macos/build-lgpl-runtime.sh` downloads the pinned sources, builds
them in a temporary prefix without spaces, and writes `build/runtime/lib`.
`packaging/macos/prepare-runtime.sh` then copies the closure into
`resources/darwin-arm64/libmpv`, rewrites every install name to
`@loader_path/<name>`, and signs each copy ad hoc.

The runtime build deliberately disables GPL-only components: no x264, x265,
rubberband, libbluray, or GPL FFmpeg parts. It also disables Lua, JavaScript,
libarchive, Vulkan and the Cocoa window backend: the app supplies its own OpenGL
context and only uses libmpv's render API.

### Known build limitation

This build has no VideoToolbox interop: `videotoolbox-gl` needs mpv's Cocoa
backend and its Swift/Metal layer, and that backend makes
`mpv_terminate_destroy` hang from a libmpv render client. `hwdec=auto-safe`
therefore settles on software decoding. The M0 spike used a Homebrew runtime
with hardware decoding; that property is **not** inherited by this bundled
runtime. QA records the decoder actually reported by
`hwdec-current` instead of claiming hardware decoding.

## Package and verify

```sh
make package
```

This runs the runtime build, `go tool mygo build`, and
`packaging/macos/verify-app.sh`. The gate checks:

- `lipo -archs` is arm64 only;
- `Contents/Resources/libmpv` exists with at least 10 dylibs;
- `Contents/Resources/libSqlitecore.dylib` exists, since the app opens its
  local library at launch;
- every Mach-O dependency is `@loader_path`, `/usr/lib`, `/System` or
  `/Library/Frameworks`; Homebrew, `/usr/local` and `/Users` paths fail;
- no HTML, JavaScript, TypeScript, `package.json`, WebKit or framework
  directories ship;
- notices and licenses are present;
- `codesign --verify --deep --strict` passes;
- the DMG mounts and the app inside also passes signature verification.

Run it against an existing build:

```sh
./packaging/macos/verify-app.sh \
  build/darwin-arm64/Mumu.app \
  build/darwin-arm64/mumu-0.1.0-arm64.dmg
```

`mygo build` obtains `libSqlitecore.dylib` for the build host from the pinned
native module, so packaging needs network access or a pre-staged library named
by `MYGO_NATIVE_SQLITECORE`. `go test` uses the same download and caches it in
`~/Library/Caches/mygo/natives`. The packaged app itself never downloads native
code.

## Release assets

A tag `v*` runs `.github/workflows/release.yml` on `macos-15` (arm64, free for
public repositories). It builds the runtime, packages the app, verifies it, and
uploads a draft release:

- `mumu-<version>-arm64.dmg`
- `mumu-libmpv-<version>-arm64.tar.gz`
- `mumu-libmpv-<version>-arm64-src.tar.gz` (pinned upstream sources, build script and flags)
- `SHA256SUMS`
- `latest.json`
- `THIRD_PARTY_NOTICES.md`

`latest.json` is for the landing page:

```json
{
  "version": "0.1.0",
  "arch": "arm64",
  "minimumSystemVersion": "13.0",
  "signed": false,
  "dmg": "https://github.com/satyawikananda/mumu/releases/download/v0.1.0/mumu-0.1.0-arm64.dmg",
  "sha256": "<dmg sha256>",
  "notes": "https://github.com/satyawikananda/mumu/releases/tag/v0.1.0"
}
```

### Prerelease tags

A suffix marks a prerelease, and CI creates the draft with `--prerelease`:

| Tag | Meaning |
| --- | --- |
| `v0.1.0-alpha.1` | First alpha, blockers fixed, full QA matrix still open |
| `v0.1.0-rc.1` | Release candidate, matrix expected to be complete |
| `v0.1.0` | Stable |

`mygo.json` keeps the bundle version (`0.1.0`), because macOS reads
`CFBundleShortVersionString` as period-separated integers. The tag carries the
suffix instead, and asset names keep the bundle version:
`mumu-0.1.0-arm64.dmg` inside the `v0.1.0-alpha.1` release.

`latest.json` from a prerelease must **not** be published to the landing page as
the current download: it points at a release the maintainer has not accepted as
stable yet. Publish it with the stable tag.

## Unsigned distribution

There is no Developer ID and no notarization for v0.1. Ad-hoc signing is still
required on Apple Silicon and happens automatically.

A DMG downloaded through a browser carries `com.apple.quarantine`. On first
launch macOS refuses the app until the user approves it:

1. Try to open Mumu once and dismiss the warning.
2. Open System Settings > Privacy & Security.
3. Click **Open Anyway** and confirm.

Right-click > Open is not a reliable bypass on macOS 15 and later. A user who
has verified the SHA-256 may instead run:

```sh
xattr -dr com.apple.quarantine /Applications/Mumu.app
```

Do not disable Gatekeeper globally. A build made locally with
`go tool mygo build` has no quarantine and shows no warning.

## LGPL compliance

The runtime is dynamically linked and unmodified. Users may replace it with a
compatible build by setting `MUMU_MPV_LIBRARY` to a directory of dylibs, or by
replacing `Contents/Resources/libmpv` and re-signing the app ad hoc. The
corresponding source, build script and flags ship with every release as
`mumu-libmpv-<version>-arm64-src.tar.gz`; the same material is in this
repository.

mumu's MIT license does not cover the runtime. See
[THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md).
