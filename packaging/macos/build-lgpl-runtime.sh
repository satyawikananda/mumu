#!/usr/bin/env bash
# Build the LGPL libmpv runtime that ships inside mumu.
#
# The app is MIT. Every native component here is LGPL-compatible: mpv and
# FFmpeg are built without --enable-gpl, and no GPL-only dependency is linked.
# See packaging/macos/mpv-runtime.json for the pinned versions and hashes.
#
# Usage:
#   packaging/macos/build-lgpl-runtime.sh [--force]
#
# Output:
#   build/runtime/lib  the flat library directory prepare_runtime.py consumes
#
# Requirements: macOS arm64, clang, meson, ninja, cmake, pkg-config, python3.
# Build tools are maintainer-only. A user of a release needs none of them.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="${MUMU_RUNTIME_WORK:-${TMPDIR:-/tmp}/mumu-runtime-build}"
prefix="$work/prefix"
src="$work/src"
out="$root/build/runtime/lib"
force=0

if [[ "${1:-}" == "--force" ]]; then
  force=1
fi

if [[ $force -eq 1 ]]; then
  rm -rf "$src"/build-*
fi

if [[ -f "$out/libmpv.2.dylib" && $force -eq 0 ]]; then
  echo "runtime already built: $out (pass --force to rebuild)"
  exit 0
fi

for tool in meson ninja cmake pkg-config python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "missing build tool: $tool" >&2
    exit 1
  fi
done

mkdir -p "$src" "$prefix"
export PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig"
export PKG_CONFIG_PATH=""
export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-12.0}"

fetch() {
  local url="$1" sha="$2" file="$3"
  if [[ ! -f "$src/$file" ]]; then
    echo "fetching $file"
    curl -fL --retry 3 --retry-all-errors --max-time 900 -o "$src/$file" "$url"
  fi
  echo "$sha  $src/$file" | shasum -a 256 -c -
}

extract() {
  local file="$1" dir="$2"
  if [[ ! -d "$src/$dir" ]]; then
    tar -xf "$src/$file" -C "$src"
  fi
}

# FriBidi
fetch "https://github.com/fribidi/fribidi/releases/download/v1.0.16/fribidi-1.0.16.tar.xz" \
  "1b1cde5b235d40479e91be2f0e88a309e3214c8ab470ec8a2744d82a5a9ea05c" "fribidi-1.0.16.tar.xz"
extract "fribidi-1.0.16.tar.xz" "fribidi-1.0.16"
meson setup "$src/build-fribidi" "$src/fribidi-1.0.16" --prefix="$prefix" --libdir=lib \
  --buildtype=release -Ddocs=false -Dtests=false >/dev/null
ninja -C "$src/build-fribidi" install >/dev/null

# FreeType
fetch "https://github.com/freetype/freetype/archive/refs/tags/VER-2-13-3.tar.gz" \
  "bc5c898e4756d373e0d991bab053036c5eb2aa7c0d5c67e8662ddc6da40c4103" "freetype-2.13.3.tar.gz"
extract "freetype-2.13.3.tar.gz" "freetype-VER-2-13-3"
cmake -S "$src/freetype-VER-2-13-3" -B "$src/build-freetype" \
  -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=ON \
  -DFT_DISABLE_ZLIB=ON -DFT_DISABLE_BZIP2=ON -DFT_DISABLE_PNG=ON \
  -DFT_DISABLE_HARFBUZZ=ON -DFT_DISABLE_BROTLI=ON >/dev/null
cmake --build "$src/build-freetype" -j4 >/dev/null
cmake --install "$src/build-freetype" >/dev/null

# HarfBuzz
fetch "https://github.com/harfbuzz/harfbuzz/archive/refs/tags/15.1.0.tar.gz" \
  "5c057929413b8664592fea67edea5c2c47b4ffb0bf5f4accb5164bc6acc69d94" "harfbuzz-15.1.0.tar.gz"
extract "harfbuzz-15.1.0.tar.gz" "harfbuzz-15.1.0"
meson setup "$src/build-harfbuzz" "$src/harfbuzz-15.1.0" --prefix="$prefix" --libdir=lib \
  --buildtype=release -Dglib=disabled -Dgobject=disabled -Dfreetype=disabled \
  -Dcairo=disabled -Dicu=disabled -Dgraphite2=disabled -Dtests=disabled \
  -Ddocs=disabled -Dintrospection=disabled -Dutilities=disabled -Dbenchmark=disabled >/dev/null
ninja -C "$src/build-harfbuzz" install >/dev/null

# libass, with CoreText as its font provider and no fontconfig
fetch "https://github.com/libass/libass/releases/download/0.17.5/libass-0.17.5.tar.xz" \
  "2dca25c0e0c837ddf00b52011b3f82cac1e4ddd3ad018227806b0c2288864acc" "libass-0.17.5.tar.xz"
extract "libass-0.17.5.tar.xz" "libass-0.17.5"
(
  cd "$src/libass-0.17.5"
  ./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-static --enable-shared \
    --disable-fontconfig --enable-coretext --disable-test --disable-compare >/dev/null
  make -j4 >/dev/null
  make install >/dev/null
)

# libplacebo, OpenGL only
fetch "https://github.com/haasn/libplacebo/archive/refs/tags/v7.387.0.tar.gz" \
  "f9e62d3774a9c4cfd198ec3abaa88294aaade2e31c04d5ab02bcf4abc98b1df5" "libplacebo-7.387.0.tar.gz"
extract "libplacebo-7.387.0.tar.gz" "libplacebo-7.387.0"

# libplacebo needs glad and fast_float headers, which its git checkout carries
# as submodules but its release tarballs do not.
python3 -m venv "$work/venv"
"$work/venv/bin/pip" install --quiet glad2
fetch "https://github.com/fastfloat/fast_float/archive/refs/tags/v8.0.0.tar.gz" \
  "f312f2dc34c61e665f4b132c0307d6f70ad9420185fa831911bc24408acf625d" "fast_float-8.0.0.tar.gz"
extract "fast_float-8.0.0.tar.gz" "fast_float-8.0.0"
mkdir -p "$src/libplacebo-7.387.0/3rdparty/fast_float"
cp -R "$src/fast_float-8.0.0/include" "$src/libplacebo-7.387.0/3rdparty/fast_float/"

# libplacebo compiles Vulkan stubs even when Vulkan is disabled, so it needs
# the headers the release tarball does not carry.
fetch "https://github.com/KhronosGroup/Vulkan-Headers/archive/refs/tags/v1.4.363.tar.gz" \
  "cbaf687d3c59b9666fe080e7f8396b6b4f4d344a768ee6b337c59852f4526b68" "vulkan-headers.tar.gz"
extract "vulkan-headers.tar.gz" "Vulkan-Headers-1.4.363"
mkdir -p "$src/libplacebo-7.387.0/3rdparty/Vulkan-Headers"
cp -R "$src/Vulkan-Headers-1.4.363/include" "$src/libplacebo-7.387.0/3rdparty/Vulkan-Headers/"

glad_site="$("$work/venv/bin/python" -c 'import site; print(site.getsitepackages()[0])')"
export PYTHONPATH="$glad_site"
meson setup "$src/build-libplacebo" "$src/libplacebo-7.387.0" --prefix="$prefix" --libdir=lib --buildtype=release -Dvulkan=disabled -Dopengl=enabled -Dd3d11=disabled -Dglslang=disabled -Dshaderc=disabled -Dlcms=disabled -Ddovi=disabled -Dlibdovi=disabled -Ddemos=false -Dtests=false -Dbench=false -Dfuzz=false >/dev/null
ninja -C "$src/build-libplacebo" install >/dev/null

# libplacebo.pc names vulkan in Requires.private even when it is disabled, and
# pkg-config then refuses to report its flags. It is not linked here.
sed -i '' '/^Requires.private: vulkan$/d' "$prefix/lib/pkgconfig/libplacebo.pc"

# FFmpeg, LGPL
fetch "https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz" \
  "8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e" "ffmpeg-9.0.2.tar.xz"
extract "ffmpeg-9.0.2.tar.xz" "ffmpeg-9.0.2"
(
  cd "$src/ffmpeg-9.0.2"
  ./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-static --enable-shared \
    --disable-programs --disable-doc --disable-gpl --disable-version3 --disable-nonfree \
    --disable-autodetect --enable-securetransport --enable-videotoolbox \
    --enable-audiotoolbox --enable-avfoundation --enable-appkit --enable-coreimage \
    --disable-debug >/dev/null
  make -j4 >/dev/null
  make install >/dev/null
)

# mpv, LGPL and libmpv-only
fetch "https://github.com/mpv-player/mpv/archive/refs/tags/v0.42.0.tar.gz" \
  "bcfebbfa2b65a6d88dc97ea18117831072373e38b5785a2afb874ed4df5070dd" "mpv-0.42.0.tar.gz"
extract "mpv-0.42.0.tar.gz" "mpv-0.42.0"
# The Cocoa backend cannot be used from a libmpv render client, and building it
# pulls in Swift and Metal. Without it, CoreAudio still needs the cfstr helpers
# that mpv keeps in the Cocoa source list; apply the small upstream fix.
if ! grep -q "Needed by CoreAudio and AVFoundation" "$src/mpv-0.42.0/meson.build"; then
  patch -d "$src/mpv-0.42.0" -p1 --forward <"$root/packaging/macos/patches/mpv-utils-mac.patch"
fi
meson setup "$src/build-mpv" "$src/mpv-0.42.0" --prefix="$prefix" --libdir=lib \
  --buildtype=release -Dgpl=false -Dlibmpv=true -Dcplayer=false -Dbuild-date=false \
  -Dlua=disabled -Djavascript=disabled -Dcplugins=disabled -Dlibarchive=disabled \
  -Dlibbluray=disabled -Ddvdnav=disabled -Ddvda=disabled -Dcdda=disabled \
  -Drubberband=disabled -Dzimg=disabled -Duchardet=disabled -Djpeg=disabled \
  -Dlcms2=disabled -Dlibcurl=disabled -Dsubrandr=disabled -Dvapoursynth=disabled \
  -Dvulkan=disabled -Dx11=disabled -Dwayland=disabled -Dcaca=disabled \
  -Dsdl2-video=disabled -Dsdl2-audio=disabled -Dsdl2-gamepad=disabled \
  -Dshaderc=disabled -Dspirv-cross=disabled -Dvdpau=disabled -Dvaapi=disabled \
  -Dcuda-hwaccel=disabled -Dcuda-interop=disabled -Dd3d-hwaccel=disabled \
  -Dd3d9-hwaccel=disabled -Dgl-dxinterop-d3d9=disabled -Dios-gl=disabled \
  -Dandroid-media-ndk=disabled -Degl=disabled -Dmacos-cocoa-cb=disabled \
  -Dmacos-media-player=disabled -Dmacos-touchbar=disabled -Dswift-build=disabled \
  -Dplain-gl=enabled -Dgl=enabled -Dgl-cocoa=disabled -Dvideotoolbox-gl=disabled \
  -Dvideotoolbox-pl=disabled -Dcocoa=disabled -Dcoreaudio=enabled -Davfoundation=enabled >/dev/null
ninja -C "$src/build-mpv" install >/dev/null

# Copy the flat library set the app ships. Symlinks stay beside the files they
# name; prepare_runtime.py follows each dependency it rewrites.
mkdir -p "$out"
cp -R "$prefix/lib/"*.dylib "$out/"

strings "$prefix/lib/libavutil.61.1.102.dylib" | grep -q "LGPL version 2.1 or later" || {
  echo "FFmpeg is not an LGPL build" >&2
  exit 1
}
if strings "$prefix/lib/libavcodec.63.1.102.dylib" | grep -q -- "--enable-gpl"; then
  echo "FFmpeg carries GPL build flags" >&2
  exit 1
fi

echo "built LGPL runtime in $out"
