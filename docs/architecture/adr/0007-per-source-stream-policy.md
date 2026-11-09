# 0007 — Per-source stream policy and consent

- Status: accepted
- Date: 2026-11-10
- Issues: [#11](https://github.com/satyawikananda/mumu/issues/11) (prerequisite
  slice 11a), follow-up to [#7](https://github.com/satyawikananda/mumu/issues/7),
  [#9](https://github.com/satyawikananda/mumu/issues/9)

## Context

Slices #7/#9 shipped a single, global network policy: a remote playlist could
only reference public `http`/`https` streams, and anything else was skipped with
a diagnostic. That is the right default for a community catalog, but it also
made two legitimate cases impossible:

- a user playlist pointing at a LAN source (a home IPTV box, a local HLS
  origin), which cannot work without an opt-in; and
- any future non-HTTP transport, which needs a per-source decision rather than
  a code-wide constant.

`AGENTS.md` §9 requires bounded, cancellable network access, no automatic fetch
of untrusted private addresses, and no claim of support without evidence.
Slice 11a needs the policy to be stored per source, asked for explicitly, and
reused by every refresh, so the Watch UI can rely on it.

## Decision

1. **The policy is per source and stored.** Migration 0003 adds
   `sources.allow_private INTEGER NOT NULL DEFAULT 0` and
   `sources.allowed_schemes TEXT NOT NULL DEFAULT 'http,https'`. Existing
   databases upgrade in place; a row that predates 0003 reads as strict.

2. **Every new source starts strict.** `allow_private` is off and the scheme
   allowlist is `http,https`. Nothing is enabled implicitly.

3. **Consent is explicit and narrow.** A preview reports `NeedsPrivate` when
   any accepted stream URL has a private, loopback or link-local host, and
   never fetches those streams. Only the import sheet can record consent, and
   `Save` stores `allow_private = 1` only when the preview both reported
   private hosts and carried the consent. `PreviewLocal` and `PreviewStream`
   keep their existing behaviour, but their channels are also dropped by `Save`
   unless consent was recorded.

4. **Built-in sources are always strict.** `catalog.Source.EffectivePolicy`
   returns the strict policy for `SourceBuiltin`, the parser runs the built-in
   refresh with private hosts off, `sources.ToStorageSource` writes the strict
   values, the refresh applies the policy again before storage, and
   `Save`/`SetEnabled`/`Refresh` refuse a built-in source. A stored row that
   claims a consent is read back as strict, so a corrupted or hand-edited
   database cannot widen the community catalog.

5. **The scheme allowlist is normalized and cannot widen by accident.**
   `catalog.NormalizeSchemes` trims, lowercases, deduplicates, sorts, always
   includes `http` and `https`, and always removes `file`, `udp`, `rtp`,
   `multicast`, `rtsp`, `rtmp` and `srt`. `file` stays a local-playlist
   decision (`playlist.Options.AllowLocalPaths`) and is never produced by a
   stored policy. The column exists so a proven transport can be added per
   source later without a migration.

6. **Playback validates the scheme again.** `player.Source` carries `Scheme` and
   `Headers`; the controller rejects anything but `http`/`https` with
   `player.ErrUnsupportedScheme` before it reaches an engine, and a declared
   scheme must agree with the URL. The controller performs no network access, so
   a private host or a header remains a source-policy decision made earlier.

7. **Only sanitized headers reach libmpv.** The adapter builds a
   `http-header-fields` value from `User-Agent` and `Referer` alone, case
   insensitively, and drops empty values, control characters, over-long values
   and values containing `,` or `\` (mpv parses the option as a comma-separated
   list escaped with backslash). The value is set immediately before `loadfile`
   on the pump goroutine, cleared on stop, and never logged.

## Evidence gap — schemes and headers

This change enables **only `http` and `https`**. `rtsp`, `rtmp` and `srt` stay
rejected everywhere: `catalog.SchemeAllowed`, `catalog.SchemeSupported`,
`player.sourceScheme` and the parser all refuse them. They may only be enabled
by a later ADR that records a real macOS/libmpv smoke test.

No new scheme was enabled for this slice. The header-forwarding path was
verified on a real libmpv runtime:

- Tested 10 November 2026 on macOS 27.0.1 (26A434), darwin/arm64, Apple M3;
  Go 1.27.3; MyGo 0.8.3; libmpv runtime 26.10.1 (`mpv 0.41.0`, client API 2.5)
  from `/private/tmp/mumu-runtime/lib`.
- Command that passed:

  ```sh
  MUMU_MPV_LIBRARY=/private/tmp/mumu-runtime/lib MUMU_MPV_HEADER_TEST=1 \
    go test ./internal/player/libmpv -run TestIntegrationPlaybackHeaders -v
  ```

- Observed: `--- PASS: TestIntegrationPlaybackHeaders (0.30s)`. The local HTTP
  server received `User-Agent: mumu-header-test/0.1` and the expected
  `Referer`; the `Cookie` header in the source was not forwarded.
- `go test` removes `DYLD_LIBRARY_PATH` from the binaries it runs in this
  shell, so the run above uses `MUMU_MPV_LIBRARY` instead; building the test
  binary with `go test -c` and exporting `DYLD_LIBRARY_PATH` also works. Before
  10 November 2026 the adapter needed the `mumu_mpv` tag and panicked at init
  without a dylib; it is now the default `darwin/arm64` build with a lazy
  loader (ADR 0005 amendment).
- Log: [header-forwarding.log](../evidence/header-forwarding.log).

The M0 findings in [native-playback-feasibility.md](../native-playback-feasibility.md)
remain the recorded evidence for the renderer and for `http`/`https` playback.

| Scheme | Status | Evidence |
|---|---|---|
| `http` | enabled | M0 spike playback; header test above |
| `https` | enabled | M0 spike playback |
| `rtsp` | rejected | none; requires a recorded smoke test |
| `rtmp` | rejected | none; requires a recorded smoke test |
| `srt` | rejected | none; requires a recorded smoke test |
| `udp`, `rtp`, multicast | rejected | not a supported transport |
| `file` | local playlist only | parser path, never a stored policy |

## Consequences

- A LAN or loopback stream now needs one explicit decision per source, and the
  decision survives every refresh; the refresh re-validates channels against it
  as well, so a hand-edited playlist cannot slip past the saved policy.
- Re-importing the same playlist replaces the stored consent with the new
  preview's answer. A user who re-imports must confirm again.
- `sources.SaveSource` reports how many channels the policy dropped, so the
  sheet can explain the count instead of silently losing rows.
- The community catalog is strict at four layers, which is more code but keeps
  the one place a regression would matter covered.
- Adding a scheme is a documented, tested change, not an option edit.

## Alternatives considered

- **One global setting for private hosts.** Simpler, but it would silently
  widen every imported playlist and the built-in catalog at once, which
  contradicts "Default off for every new user source".
- **Consent by "the user typed the URL".** Already the rule for reading the
  playlist document; it is not enough for the streams inside a third-party
  file the user may not have inspected.
- **Store `file` in the allowlist for local playlists.** Blurs the boundary
  between a file the user picked and a remote playlist; the parser's
  `AllowLocalPaths` is the precise gate.
- **Enable `rtsp` because libmpv supports it.** mpv's option surface is not
  evidence that this build, surface and packaging work. Slice 11a explicitly
  forbids advertising an unproven scheme.
