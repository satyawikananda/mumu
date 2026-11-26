#!/usr/bin/env bash
# Build the CI-only Linux/Windows LGPL runtime from the release source pins.
# No installers or graphical platform support are produced here.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
manifest="$root/packaging/macos/mpv-runtime.json"
work="$root/build/headless-work"
prefix="$root/build/headless-runtime"
src="$work/src"
jobs=4
platform="$(uname -s)"
case "$platform" in
  Linux) target=linux ;;
  MINGW*|MSYS*) target=windows ;;
  *) echo "This builder targets Linux AMD64 and Windows AMD64 only" >&2; exit 1 ;;
esac
test "$(uname -m)" = x86_64

for tool in curl meson ninja cmake make gcc; do
  command -v "$tool" >/dev/null || { echo "missing build tool: $tool" >&2; exit 1; }
done
if ! command -v pkg-config >/dev/null && ! command -v pkgconf >/dev/null; then
  echo "missing build tool: pkg-config" >&2
  exit 1
fi
python="$(command -v python3 || command -v python || true)"
test -n "$python" || { echo "missing build tool: python3" >&2; exit 1; }
mkdir -p "$src" "$prefix"
pkg_config_dir="$prefix/lib/pkgconfig"
if [[ "$target" == windows ]]; then
  pkg_config_dir="$(cygpath -w "$pkg_config_dir")"
fi
export PKG_CONFIG_LIBDIR="$pkg_config_dir"
export PKG_CONFIG_PATH=""

# Source URLs and SHA256 values have one source of truth. Verify even cached
# archives; unavailable or mismatched sources fail instead of selecting latest.
fetch() {
  local component="$1" archive="$2"
  local fields url sha
  fields="$("$python" - "$manifest" "$component" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
items = [dict(data['libmpv'], name='mpv')] + data['components'] + data['build_only']
item = next(item for item in items if item['name'] == sys.argv[2])
print(item['source'], item['sha256'])
PY
  )"
  read -r url sha <<< "$fields"
  if [[ ! -f "$src/$archive" ]]; then
    curl --fail --location --retry 3 --max-time 600 "$url" -o "$src/$archive"
  fi
  "$python" - "$src/$archive" "$sha" <<'PY'
import hashlib, sys
with open(sys.argv[1], 'rb') as source:
    actual = hashlib.file_digest(source, 'sha256').hexdigest()
if actual != sys.argv[2]:
    raise SystemExit('source checksum mismatch: ' + actual)
PY
}

extract() {
  local archive="$1" directory="$2"
  if [[ ! -d "$src/$directory" ]]; then tar -xf "$src/$archive" -C "$src"; fi
}

meson_build() {
  local name="$1" source="$2"
  shift 2
  meson setup "$work/$name" "$source" --prefix="$prefix" --libdir=lib \
    --buildtype=release --default-library=shared "$@"
  ninja -C "$work/$name" -j "$jobs" install
}

fetch FriBidi fribidi.tar.xz
extract fribidi.tar.xz fribidi-1.0.16
meson_build fribidi "$src/fribidi-1.0.16" -Ddocs=false -Dtests=false

fetch FreeType freetype.tar.gz
extract freetype.tar.gz freetype-VER-2-13-3
cmake -S "$src/freetype-VER-2-13-3" -B "$work/freetype" -G Ninja \
  -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=ON \
  -DFT_DISABLE_ZLIB=ON -DFT_DISABLE_BZIP2=ON -DFT_DISABLE_PNG=ON \
  -DFT_DISABLE_HARFBUZZ=ON -DFT_DISABLE_BROTLI=ON
cmake --build "$work/freetype" -j "$jobs"
cmake --install "$work/freetype"

fetch HarfBuzz harfbuzz.tar.gz
extract harfbuzz.tar.gz harfbuzz-15.1.0
meson_build harfbuzz "$src/harfbuzz-15.1.0" -Dglib=disabled -Dgobject=disabled \
  -Dfreetype=disabled -Dcairo=disabled -Dicu=disabled -Dgraphite2=disabled \
  -Dtests=disabled -Ddocs=disabled -Dintrospection=disabled -Dutilities=disabled -Dbenchmark=disabled

fetch libass libass.tar.xz
extract libass.tar.xz libass-0.17.5
(
  cd "$src/libass-0.17.5"
  ./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-static --enable-shared \
    --disable-fontconfig --disable-coretext --disable-directwrite \
    --disable-require-system-font-provider --disable-test --disable-compare
  make -j "$jobs"
  make install
)

fetch libplacebo libplacebo.tar.gz
extract libplacebo.tar.gz libplacebo-7.387.0
fetch fast_float fast_float.tar.gz
extract fast_float.tar.gz fast_float-8.0.0
fetch Vulkan-Headers vulkan-headers.tar.gz
extract vulkan-headers.tar.gz Vulkan-Headers-1.4.363
mkdir -p "$src/libplacebo-7.387.0/3rdparty/fast_float" "$src/libplacebo-7.387.0/3rdparty/Vulkan-Headers"
cp -R "$src/fast_float-8.0.0/include" "$src/libplacebo-7.387.0/3rdparty/fast_float/"
cp -R "$src/Vulkan-Headers-1.4.363/include" "$src/libplacebo-7.387.0/3rdparty/Vulkan-Headers/"
meson_build libplacebo "$src/libplacebo-7.387.0" -Dauto_features=disabled \
  -Dvulkan=disabled -Dopengl=disabled -Dd3d11=disabled -Dglslang=disabled \
  -Dshaderc=disabled -Dlcms=disabled -Ddovi=disabled -Dlibdovi=disabled \
  -Ddemos=false -Dtests=false -Dbench=false -Dfuzz=false
# The source pin lists Vulkan headers as a private requirement even without a
# Vulkan backend. No Vulkan library is linked by this headless runtime.
"$python" - "$prefix/lib/pkgconfig/libplacebo.pc" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
path.write_text(path.read_text().replace('Requires.private: vulkan\n', ''))
PY

fetch FFmpeg ffmpeg.tar.xz
extract ffmpeg.tar.xz ffmpeg-9.0.2
ffmpeg_platform=()
if [[ "$target" == windows ]]; then ffmpeg_platform=(--target-os=mingw32 --arch=x86_64 --cc=gcc --cxx=g++); fi
(
  cd "$src/ffmpeg-9.0.2"
  ./configure --prefix="$prefix" --libdir="$prefix/lib" --disable-static --enable-shared \
    --disable-programs --disable-doc --disable-gpl --disable-version3 --disable-nonfree \
    --disable-autodetect "${ffmpeg_platform[@]}"
  make -j "$jobs"
  make install
)

fetch mpv mpv.tar.gz
extract mpv.tar.gz mpv-0.42.0
meson_build mpv "$src/mpv-0.42.0" -Dauto_features=disabled \
  -Dgpl=false -Dlibmpv=true -Dcplayer=false -Dbuild-date=false -Dlibass=enabled

if [[ "$target" == windows ]]; then
  # Keep transitive non-system dependencies beside libmpv. LoadLibraryEx never
  # consults PATH, including when resolving these dependencies.
  for library in "$prefix/bin/"*.dll; do cp "$library" "$prefix/lib/"; done
  for name in libgcc_s_seh-1.dll libstdc++-6.dll libwinpthread-1.dll; do
    location="$(gcc -print-file-name="$name")"
    if [[ -f "$location" ]]; then cp "$location" "$prefix/lib/"; fi
  done
fi

"$python" - "$manifest" "$target/amd64" "$prefix/source-manifest.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as source:
    pins = json.load(source)
document = {
    "target": sys.argv[2],
    "build_script": "packaging/qa/build-headless-runtime.sh",
    "source_pins": pins,
}
with open(sys.argv[3], "w", encoding="utf-8") as output:
    json.dump(document, output, indent=2)
    output.write("\n")
PY
echo "Built CI-only $target/amd64 LGPL runtime; no video presentation claim"
