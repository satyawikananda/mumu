#!/usr/bin/env bash
# Stage the built LGPL runtime for mygo build.
#
# mygo copies resources/darwin-arm64/* into Contents/Resources of a darwin/arm64
# app, so the payload lands in Contents/Resources/libmpv. MyGo then signs every
# Mach-O file it copied with the app.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source_dir="$root/build/runtime/lib"
dest="$root/resources/darwin-arm64/libmpv"

if [[ ! -f "$source_dir/libmpv.2.dylib" ]]; then
  echo "missing runtime: run packaging/macos/build-lgpl-runtime.sh first" >&2
  exit 1
fi

mkdir -p "$dest"
python3 "$root/packaging/macos/prepare_runtime.py" --source "$source_dir" --dest "$dest"

echo "staged $(ls "$dest" | wc -l | tr -d ' ') libraries in $dest"
