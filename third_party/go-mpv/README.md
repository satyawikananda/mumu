# go-mpv (vendored, lazy runtime loading)

This directory is a local copy of pinned `github.com/gen2brain/go-mpv` v0.4.0.
`go.mod` in the repository root replaces the module with this directory.

## Why

Upstream v0.4.0 loads the libmpv runtime from a package `init()`
(`client_nocgo.go`) and panics when no dylib is found
(`purego_darwin.go`). Every binary that imports the package therefore needs
libmpv before `main` runs, and a machine without mpv crashes at startup instead
of reporting an unavailable player. Upstream `main` still does this, and v0.4.0
is the newest tag, so the behaviour cannot be fixed without changing the
binding.

## What changed

Only runtime loading is patched, plus the removal of the cgo backend:

| File | Change |
|---|---|
| `load.go` | New. `Load(candidates)` resolves the runtime, registers entry points, checks `mpv_client_api_version`, and returns an error instead of panicking. Retryable. |
| `client_purego.go` | Renamed from `client_nocgo.go`; the `!cgo || nocgo` tag is gone because this copy is purego-only, and the `init()` body moved to `registerClientFuncs`. `New` returns nil before a successful `Load`. |
| `render_purego.go` | Renamed from `render_nocgo.go`; tag removed; `init()` body moved to `registerRenderFuncs`. |
| `purego_darwin.go`, `purego_linux.go`, `purego_windows.go` | `loadLibrary` (panic) replaced by `openLibrary(candidates)` returning `(handle, error)`. |
| cgo backend (`client.go`, `render_cgo.go`, `render_export_cgo.go`) | Not copied. The purego backend is the only one, so building never needs CGO, mpv headers or `-lmpv`. |

Everything else (`events.go`, `node.go`, `render.go`, `errors.go`) is
unmodified.

## Updating

Re-copy the upstream files listed above, re-apply the loader patch, and keep
the licences (`LICENSE`, `LICENSE.LGPL`) with the copy. Do not add features
here: this copy exists to make the runtime dependency explicit.
