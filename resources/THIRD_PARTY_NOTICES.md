# Third-party notices

mumu itself is MIT licensed (see [LICENSE](https://github.com/satyawikananda/mumu/blob/main/LICENSE)). The macOS app also ships
native media libraries that are not MIT. This file records them and the
obligations that come with redistributing them.

## Bundled media runtime

The app carries libmpv and its dependencies in
`Contents/Resources/libmpv`. They are built by
`packaging/macos/build-lgpl-runtime.sh` from the pinned sources in
`packaging/macos/mpv-runtime.json`. None of them is built with GPL-only
components.

| Component | Version | License | Source |
| --- | --- | --- | --- |
| mpv (libmpv) | 0.42.0 | LGPL-2.1-or-later | https://github.com/mpv-player/mpv/tree/v0.42.0 |
| FFmpeg | 9.0.2 | LGPL-2.1-or-later | https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz |
| libplacebo | 7.387.0 | LGPL-2.1-or-later | https://github.com/haasn/libplacebo/tree/v7.387.0 |
| libass | 0.17.5 | ISC | https://github.com/libass/libass/tree/0.17.5 |
| FreeType | 2.13.3 | FTL or GPL-2.0-or-later | https://github.com/freetype/freetype/tree/VER-2-13-3 |
| FriBidi | 1.0.16 | LGPL-2.1-or-later | https://github.com/fribidi/fribidi/tree/v1.0.16 |
| HarfBuzz | 15.1.0 | MIT | https://github.com/harfbuzz/harfbuzz/tree/15.1.0 |

Full license texts are in [`packaging/macos/licenses`](https://github.com/satyawikananda/mumu/tree/main/packaging/macos/licenses)
and are copied into the app at `Contents/Resources/licenses`.

### LGPL source and replacement

The libraries are dynamically linked. mumu does not modify them, and a user may
replace any of them with a compatible build: place the replacement dylibs in a
directory and start mumu with `MUMU_MPV_LIBRARY=<dir>`, or replace the files in
`Mumu.app/Contents/Resources/libmpv` and re-sign the app ad hoc
(`codesign --force --sign - Mumu.app`). The bundled runtime is not covered by
mumu's MIT license.

The corresponding source for each pinned version, the exact build flags, and
the build script are published with every release as
`mumu-libmpv-<version>-arm64-src.tar.gz`. The same material is in this
repository at `packaging/macos/`, so the source stays available with the
binary.

### Build-only tools

`glad` 2.0.8 (MIT or Apache-2.0) and `fast_float` 8.0.0
(Apache-2.0 or MIT or BSL-1.0) are used while building libplacebo. They are
not part of the shipped runtime.

## Go modules

| Module | License |
| --- | --- |
| github.com/egoist/mygo | MIT |
| github.com/gen2brain/go-mpv (vendored, patched) | MIT |
| github.com/ebitengine/purego | Apache-2.0 |
| github.com/go-text/typesetting | MIT |
| golang.org/x/image, golang.org/x/text, golang.org/x/net, golang.org/x/sys | BSD-3-Clause |

The full dependency graph, including indirect modules, is pinned in `go.mod`
and `go.sum`. Their license texts remain in each module's repository and in the
Go module cache of a build.

## Content

mumu does not bundle channels, video, logos, or playlists. The built-in catalog
is a community list from iptv-org (CC0) that points at third-party streams;
those streams remain the property of their owners. See
[ADR 0006](https://github.com/satyawikananda/mumu/blob/main/docs/architecture/adr/0006-iptv-org-discovery.md).
