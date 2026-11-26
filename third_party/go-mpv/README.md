# go-mpv (vendored, lazy runtime loading)

This directory is a local copy of pinned `github.com/gen2brain/go-mpv` v0.4.0.
`go.mod` in the repository root replaces the module with this directory.

## Why

Upstream v0.4.0 loads the libmpv runtime from a package `init()`
(`client_nocgo.go`) and panics when no library is found. Every binary that
imports the package therefore needs libmpv before `main` runs, and a machine
without mpv crashes at startup instead of reporting an unavailable player.
Upstream `main` still does this, and v0.4.0 is the newest tag, so the behaviour
cannot be fixed without changing the binding.

## What changed

Only runtime loading is patched, plus the removal of the cgo backend:

| File | Change |
|---|---|
| `load.go` | `Load(candidates)` resolves the runtime, stages every entry point, checks `mpv_client_api_version`, and publishes symbols only after all checks pass. A failure returns an error instead of panicking, releases the native handle and can be retried. `RuntimeInfo` reports the loaded file's base name and client API version. |
| `client_purego.go` | Renamed from `client_nocgo.go`; the `!cgo || nocgo` tag is gone because this copy is purego-only, and the `init()` body moved to `registerClientFuncs`. `New` returns nil before a successful `Load` or when `mpv_create` fails. |
| `render_purego.go` | Renamed from `render_nocgo.go`; tag removed; `init()` body moved to `registerRenderFuncs`. |
| `purego_darwin.go` | `loadLibrary` (panic) replaced by `openLibrary(candidates)` returning `(handle, error)`. Absolute candidates are architecture-checked before `dlopen`. |
| `purego_linux.go` | The same loader contract for ELF, using `libmpv.so.2` / `libmpv.so`. |
| `purego_windows.go` | The same loader contract for PE. Only absolute DLL paths are accepted, opened with `LoadLibraryExW` and `LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR \| LOAD_LIBRARY_SEARCH_SYSTEM32`. The CRT for `malloc`/`free`/`setlocale` comes from System32 only. |
| `architecture.go` | Validates Mach-O, ELF or PE architecture before a file is executed. |
| `abi_unix.go`, `abi_windows.go` | Models C `unsigned long` as LP64 on Unix and LLP64 on Windows, so `mpv_client_api_version` returns the right width. |
| cgo backend (`client.go`, `render_cgo.go`, `render_export_cgo.go`) | Not copied. The purego backend is the only one, so building never needs CGO, mpv headers or `-lmpv`. |

Everything else (`events.go`, `node.go`, `render.go`, `errors.go`) is
unmodified. The copy adds `load_test.go`, `abi_test.go`, `abi_unix.go`,
`abi_windows.go`, `architecture.go` and `search_windows_test.go`; none of them
changes the public client API used by mumu.

## Platform support

| Target | Loader |
|---|---|
| `darwin/arm64` | `libmpv.dylib`, then `libmpv.2.dylib`; bare names use the dynamic loader, so `DYLD_LIBRARY_PATH` applies |
| `linux/amd64` | `libmpv.so.2`, then `libmpv.so`; bare names use the dynamic loader, so `LD_LIBRARY_PATH` applies |
| `windows/amd64` | `libmpv-2.dll`, `libmpv.dll`, `mpv-2.dll`, `mpv-1.dll`; absolute paths only, no PATH or working-directory search |

`mumu` calls `Load` with candidates chosen by
`internal/player/libmpv/library`; this package never guesses a system location
on Windows. A successful runtime is resident for the process. A failed
registration frees the library and the CRT handle, clears staged symbols and
leaves a later `Load` able to succeed.

## Verification

`go test ./...` in this directory covers the API-version gate. Set
`MUMU_MPV_TEST=1` and point `MUMU_MPV_LIBRARY` at the pinned runtime for the
gated native tests:

- `TestIntegrationLoaderRecovery` compiles wrong-API and missing-symbol
  libraries, checks wrong architecture, proves two failed attempts leave no
  state, then loads the real runtime in the same process.
- `TestIntegrationABI` compiles a C probe against `MUMU_MPV_INCLUDE` and
  compares C sizes and offsets with the Go structs.
- `TestIntegrationWindowsSearch` builds a DLL plus a dependency and proves that
  the working directory and `PATH` cannot supply the dependency, while the
  directory containing the DLL can.

The repository CI runs these through `packaging/qa/test-headless.sh`, which
fails when a required native test is skipped.

## Updating

Re-copy the upstream files listed above, re-apply the loader patch, and keep
the licences (`LICENSE`, `LICENSE.LGPL`) with the copy. Do not add features
here: this copy exists to make the runtime dependency explicit.
