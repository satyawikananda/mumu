#!/usr/bin/env bash
# MUMU_MPV_LIBRARY must identify the pinned runtime staged by the runner.
# MUMU_MPV_INCLUDE must identify the matching headers for the ABI probe.
set -euo pipefail
: "${MUMU_MPV_LIBRARY:?set MUMU_MPV_LIBRARY to the pinned runtime directory}"
: "${MUMU_MPV_INCLUDE:?set MUMU_MPV_INCLUDE to the pinned runtime headers}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
python="$(command -v python3 || command -v python || true)"
if [[ -z "$python" ]]; then
  echo "python3 is required to verify native test results" >&2
  exit 1
fi
mkdir -p build/native-evidence
if [[ -d "${MUMU_MPV_LIBRARY}" ]]; then
  case "$(uname -s)" in
    Linux) export LD_LIBRARY_PATH="$MUMU_MPV_LIBRARY:${LD_LIBRARY_PATH:-}" ;;
    Darwin) export DYLD_LIBRARY_PATH="$MUMU_MPV_LIBRARY:${DYLD_LIBRARY_PATH:-}" ;;
  esac
fi
export MUMU_MPV_TEST=1 MUMU_MPV_HEADER_TEST=1
go version
go test -json -count=1 -timeout=3m ./internal/player/libmpv \
  -run '^TestIntegration(EngineCreateLoadStop|PlaybackHeaders|PlaybackAddressPolicy)$' \
  | tee build/native-evidence/engine.jsonl
"$python" packaging/qa/check-native-tests.py build/native-evidence/engine.jsonl \
  TestIntegrationEngineCreateLoadStop TestIntegrationPlaybackHeaders TestIntegrationPlaybackAddressPolicy
(
  cd third_party/go-mpv
  go test -json -count=1 -timeout=3m ./... | tee ../../build/native-evidence/binding.jsonl
)
"$python" packaging/qa/check-native-tests.py build/native-evidence/binding.jsonl \
  TestIntegrationLoaderRecovery TestIntegrationABI
if [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]]; then
  "$python" packaging/qa/check-native-tests.py build/native-evidence/binding.jsonl TestIntegrationWindowsSearch
fi
