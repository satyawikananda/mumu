# Documentation

Start here for the product and engineering documentation.

## Architecture

- [Overview](architecture/README.md): boundaries, runtime flow, threading,
  storage, trust policy and platform constraints.
- [Library](architecture/library.md): favorites, recent viewing and resume.
- [Playback](architecture/playback.md): libmpv runtime, render bridge and limits.
- [Player controller](architecture/player-controller.md): states, API and
  generations.
- [Picture-in-Picture](architecture/pip.md): one session, one retained surface.
- [Sources](architecture/sources.md): parsing, identity, network policy and
  catalog imports.
- [Storage](architecture/storage.md): SQLite schema, refresh and migrations.
- [Validation](architecture/validation.md): tested environment, verified
  behavior and known gaps.
- [Video surface](architecture/video-surface.md): native attachment and
  lifetime.
- [Watch](architecture/watch.md): playback states and video presentation.

## UX

- [Shell](ux/shell.md)
- [Discover](ux/discover.md)
- [Home](ux/home.md)
- [Library](ux/library.md)
- [Settings](ux/settings.md)

## Release

- [macOS packaging](release/macos.md)
- [Testing and release](release/testing.md)

The visual contract is in [`DESIGN.md`](../DESIGN.md). Engineering rules and
commands are in [`AGENTS.md`](../AGENTS.md).
