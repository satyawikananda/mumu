#!/usr/bin/env bash
# Release gate for the packaged macOS app.
#
# It proves the app is self-contained: arm64 only, every non-system dependency
# lives in Contents/Resources/libmpv, no Homebrew or build-host path remains,
# the bundle is signed, and no web runtime slipped in.
#
# Usage:
#   packaging/macos/verify-app.sh [path/to/Mumu.app] [path/to/mumu.dmg]

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
app="${1:-$root/build/darwin-arm64/Mumu.app}"
dmg="${2:-}"

fail() {
  echo "verify-app: $*" >&2
  exit 1
}

[[ -d "$app" ]] || fail "missing app bundle: $app"
app="$(cd "$app" && pwd)"
plist="$app/Contents/Info.plist"
[[ -f "$plist" ]] || fail "missing Info.plist: $plist"

exe="$app/Contents/MacOS/$(defaults read "$plist" CFBundleExecutable)"
[[ -x "$exe" ]] || fail "missing executable: $exe"

minimum_os="$(defaults read "$plist" LSMinimumSystemVersion)"
[[ -n "$minimum_os" ]] || fail "Info.plist has no LSMinimumSystemVersion"

archs="$(lipo -archs "$exe")"
[[ "$archs" == "arm64" ]] || fail "executable architectures are $archs, want arm64 only"

libdir="$app/Contents/Resources/libmpv"
[[ -d "$libdir" ]] || fail "missing bundled runtime: $libdir"
[[ -f "$libdir/libmpv.2.dylib" ]] || fail "missing $libdir/libmpv.2.dylib"

count="$(find "$libdir" -type f -name '*.dylib' | wc -l | tr -d ' ')"
[[ "$count" -ge 10 ]] || fail "bundled runtime holds $count libraries, want at least 10"

# MyGo 0.9 loads SQLite through its native module system, so the database
# library is a separate resource next to libmpv rather than inside it. The app
# opens its local library at launch, so a missing module is a broken build.
sqlite_lib="$app/Contents/Resources/libSqlitecore.dylib"
[[ -f "$sqlite_lib" ]] || fail "missing bundled SQLite module: $sqlite_lib"

machos=()
while IFS= read -r -d '' file; do
  machos+=("$file")
done < <(find "$app/Contents" -type f -print0)

for file in "${machos[@]}"; do
  if file "$file" | grep -q "Mach-O"; then
    own_id="$(otool -D "$file" | tail -n +2 | head -1)"
    minos="$(otool -l "$file" | awk '/LC_BUILD_VERSION/{f=1} f && /minos/{print $2; exit}')"
    if [ -n "$minos" ] && awk -v a="$minos" -v b="$minimum_os" 'BEGIN {
      split(a, x, "."); split(b, y, ".");
      exit !(x[1] > y[1] || (x[1] == y[1] && x[2] > y[2]));
    }'; then
      fail "$file needs macOS $minos, above LSMinimumSystemVersion $minimum_os"
    fi
    while IFS= read -r dependency; do
      [[ "$dependency" == "$own_id" ]] && continue
      case "$dependency" in
        @loader_path/*) ;;
        /usr/lib/*|/System/*|/Library/Frameworks/*) ;;
        *) fail "$file depends on $dependency" ;;
      esac
    done < <(otool -L "$file" | tail -n +2 | awk '{print $1}')
  fi
done

if find "$app/Contents" -type f \( -name '*.html' -o -name '*.js' -o -name '*.jsx' -o -name '*.tsx' \) | grep -q .; then
  fail "bundle contains web assets"
fi
if find "$app/Contents" -type d -name '*WebKit*' -o -type d -name '*.framework' | grep -q .; then
  fail "bundle contains a framework directory"
fi
if [[ -e "$app/Contents/Resources/package.json" ]]; then
  fail "bundle contains package.json"
fi

[[ -f "$app/Contents/Resources/LICENSE" ]] || fail "missing LICENSE"
[[ -f "$app/Contents/Resources/THIRD_PARTY_NOTICES.md" ]] || fail "missing THIRD_PARTY_NOTICES.md"
[[ -d "$app/Contents/Resources/licenses" ]] || fail "missing licenses directory"

plutil -lint "$plist" >/dev/null || fail "invalid Info.plist"
version="$(defaults read "$plist" CFBundleShortVersionString)"
[[ -n "$version" ]] || fail "Info.plist has no version"

codesign --verify --deep --strict "$app" 2>/dev/null || fail "codesign verification failed"

if [[ -n "$dmg" ]]; then
  [[ -f "$dmg" ]] || fail "missing disk image: $dmg"
  mountpoint="$(mktemp -d)"
  hdiutil attach -nobrowse -readonly -mountpoint "$mountpoint" "$dmg" >/dev/null
  trap 'hdiutil detach "$mountpoint" >/dev/null 2>&1 || true; rmdir "$mountpoint" 2>/dev/null || true' EXIT
  inside="$mountpoint/$(basename "$app")"
  [[ -d "$inside" ]] || fail "disk image does not hold $(basename "$app")"
  codesign --verify --deep --strict "$inside" 2>/dev/null || fail "disk image app signature failed"
  shasum -a 256 "$dmg"
fi

echo "verify-app: OK $app (mumu $version, $count libraries)"
