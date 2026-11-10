#!/usr/bin/env bash
# Collect the exact sources of the bundled LGPL runtime for a release.
#
# The app ships binaries, so LGPL requires the corresponding source. This
# script downloads the pinned archives, verifies their hashes, and writes one
# tarball next to the app's build script and license texts.

set -euo pipefail

version="${1:?usage: collect-sources.sh <version> <outdir>}"
outdir="${2:?usage: collect-sources.sh <version> <outdir>}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="${MUMU_SOURCE_WORK:-${TMPDIR:-/tmp}/mumu-runtime-sources}"
mkdir -p "$work" "$outdir"

fetch() {
  local url="$1" sha="$2" file="$3"
  if [[ ! -f "$work/$file" ]]; then
    echo "fetching $file"
    curl -fL --retry 3 --retry-all-errors --max-time 900 -o "$work/$file" "$url"
  fi
  echo "$sha  $work/$file" | shasum -a 256 -c -
}

fetch "https://github.com/mpv-player/mpv/archive/refs/tags/v0.42.0.tar.gz" \
  "bcfebbfa2b65a6d88dc97ea18117831072373e38b5785a2afb874ed4df5070dd" "mpv-0.42.0.tar.gz"
fetch "https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz" \
  "8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e" "ffmpeg-9.0.2.tar.xz"
fetch "https://github.com/haasn/libplacebo/archive/refs/tags/v7.387.0.tar.gz" \
  "f9e62d3774a9c4cfd198ec3abaa88294aaade2e31c04d5ab02bcf4abc98b1df5" "libplacebo-7.387.0.tar.gz"
fetch "https://github.com/libass/libass/releases/download/0.17.5/libass-0.17.5.tar.xz" \
  "2dca25c0e0c837ddf00b52011b3f82cac1e4ddd3ad018227806b0c2288864acc" "libass-0.17.5.tar.xz"
fetch "https://github.com/freetype/freetype/archive/refs/tags/VER-2-13-3.tar.gz" \
  "bc5c898e4756d373e0d991bab053036c5eb2aa7c0d5c67e8662ddc6da40c4103" "freetype-2.13.3.tar.gz"
fetch "https://github.com/fribidi/fribidi/releases/download/v1.0.16/fribidi-1.0.16.tar.xz" \
  "1b1cde5b235d40479e91be2f0e88a309e3214c8ab470ec8a2744d82a5a9ea05c" "fribidi-1.0.16.tar.xz"
fetch "https://github.com/harfbuzz/harfbuzz/archive/refs/tags/15.1.0.tar.gz" \
  "5c057929413b8664592fea67edea5c2c47b4ffb0bf5f4accb5164bc6acc69d94" "harfbuzz-15.1.0.tar.gz"
fetch "https://github.com/KhronosGroup/Vulkan-Headers/archive/refs/tags/v1.4.363.tar.gz" \
  "cbaf687d3c59b9666fe080e7f8396b6b4f4d344a768ee6b337c59852f4526b68" "vulkan-headers.tar.gz"
fetch "https://github.com/fastfloat/fast_float/archive/refs/tags/v8.0.0.tar.gz" \
  "f312f2dc34c61e665f4b132c0307d6f70ad9420185fa831911bc24408acf625d" "fast_float-8.0.0.tar.gz"

stage="$work/bundle"
mkdir -p "$stage"
cp "$work"/*.tar.gz "$work"/*.tar.xz "$stage/"
mkdir -p "$stage/build"
cp -R "$root/packaging/macos" "$stage/build/"
cp "$root/go.mod" "$root/go.sum" "$stage/"
cp -R "$root/third_party/go-mpv" "$stage/"
cp "$root/docs/architecture/adr/0008-bundled-libmpv-runtime.md" "$stage/"

tar -czf "$outdir/mumu-libmpv-${version}-arm64-src.tar.gz" -C "$stage" .

echo "collected runtime sources in $outdir"
