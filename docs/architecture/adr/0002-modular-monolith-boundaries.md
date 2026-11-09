# ADR 0002: Modular monolith boundaries

- Status: accepted
- Date: 2026-11-09
- Issues: #3, #5, #6, #7

## Context

The bootstrap must leave room for a player, catalog, playlist parser and
local storage without building all of them up front. A single binary keeps
packaging and startup simple. The repository is empty of production code,
so the first structure decides where later work lands.

## Decision

One Go module and one application package, with dependency direction:

```text
cmd/mumu -> internal/app -> internal/ui
                         -> internal/player, video, catalog, playlist, storage
```

- `cmd/mumu` only wires and runs `internal/app`.
- `internal/app` is the composition root. It owns the window, cancellation
  and background jobs.
- `internal/ui` builds native views from plain state and callbacks. It does
  not import `internal/app` and does not own playback or I/O.
- `internal/player`, `internal/video`, `internal/catalog`,
  `internal/playlist` and `internal/storage` are boundaries. In #3 they
  contain package documentation only; their real APIs arrive with #5, #6
  and #7.

APIs are declared by the issue that implements them, not invented in the
bootstrap. Adding a `doc.go` now fixes ownership and naming without
committing to signatures that the pinned libmpv or SQLite work may change.

`experiments/` stays in the module so the M0 spikes keep building against
the pinned dependencies. Only `cmd/mumu` is packaged, and the darwin spike
needs the `mumu_spike` build tag.

## Consequences

- Tests of application behavior use fakes; no decoder, window or database
  is needed.
- A future package cannot import `internal/app` and create a second
  lifecycle. Cross-boundary coordination goes through the composition root.
- `doc.go`-only packages look like scaffolding until their issues land.
  That is intentional: deleting a wrong interface costs more than adding
  the right one later.
