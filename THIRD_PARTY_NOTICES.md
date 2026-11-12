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

## Bundled SQLite module

`Contents/Resources/libSqlitecore.dylib` is MyGo 0.9's SQLite native module. It
is built from the pinned MyGo module's Zig sources and compiles SQLite's
amalgamation.

| Component | Version | License | Source |
| --- | --- | --- | --- |
| SQLite | amalgamation pinned by hash in the MyGo module | Public domain | https://sqlite.org/copyright.html |
| MyGo `Sqlitecore` native module | ships with MyGo 0.9.0 | MIT | https://github.com/egoist/mygo/tree/v0.9.0/plugins/sqlite/internal/sqlitecore |

SQLite is public domain, so it carries no redistribution obligation. The
module is delivered by `mygo build` and signed with the app; the packaged app
never downloads native code at run time.

## Bundled UI assets

The app embeds two OFL font families and a subset of Lucide icons. They are
compiled into the binary rather than fetched at run time.

| Component | Version | License | Source |
| --- | --- | --- | --- |
| Inter Tight (400/500/600, Latin) | Fontsource 5.3.0 | SIL OFL 1.1 | https://github.com/rsms/inter |
| Lexend (400/500/600/700, Latin) | Fontsource 5.3.0 | SIL OFL 1.1 | https://github.com/googlefonts/lexend |
| Lucide icons | lucide-static 1.72.0 | ISC | https://github.com/lucide-icons/lucide |

License texts are in `resources/licenses/InterTight-OFL-1.1.txt`,
`resources/licenses/Lexend-OFL-1.1.txt` and
`resources/licenses/Lucide-ISC.txt`. The font files are in
`internal/ui/fonts/`; the icon subset is in `internal/ui/icons/`.

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

`go.digitalxero.dev/rpm`, `github.com/ProtonMail/go-crypto`,
`github.com/cloudflare/circl`, `github.com/klauspost/compress` and
`github.com/ulikunitz/xz` are pulled in by the pinned `github.com/egoist/mygo/cmd/mygo`
tool for Linux package building. They are build-time only and are not linked
into the macOS app.

## Content

mumu does not bundle channels, video, logos, or playlists. The built-in catalog
is a community list from iptv-org (CC0) that points at third-party streams;
those streams remain the property of their owners. See
[ADR 0006](https://github.com/satyawikananda/mumu/blob/main/docs/architecture/adr/0006-iptv-org-discovery.md).
