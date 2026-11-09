# IPTV sources, parsing and network policy

Issue: [#7](https://github.com/satyawikananda/mumu/issues/7). Follow-ups:
[#8](https://github.com/satyawikananda/mumu/issues/8) built-in catalog,
[#9](https://github.com/satyawikananda/mumu/issues/9) source management,
[#11](https://github.com/satyawikananda/mumu/issues/11) slice 11a per-source
policy.

This document defines how mumu turns M3U sources and direct streams into the
shared channel model, how it distinguishes a channel list from an HLS media
manifest, and which network destinations it refuses.

## Packages

```text
internal/catalog    domain types, identity, validation, headers, filters
internal/catalog/iptvorg  per-country community catalog and blocklist
internal/netguard   URL policy and bounded HTTP client
internal/playlist   parser, content detection, loader, stream inspection
internal/sources    catalog-to-storage mapping, built-in catalog, import service
internal/logos      bounded logo fetch, decode and cache
internal/storage    SQLite rows, migrations, atomic refresh
```

The domain package owns no I/O. `netguard` owns all untrusted HTTP. The parser
is pure and receives bytes that a bounded loader already read.

## Domain model

```go
type SourceKind string

const (
    SourceBuiltin        SourceKind = "builtin"
    SourceRemotePlaylist SourceKind = "remote_playlist"
    SourceLocalPlaylist  SourceKind = "local_playlist"
    SourceCustomStream   SourceKind = "custom_stream"
)

type Channel struct {
    SourceID   string
    ProviderID string
    Name       string
    Group      string
    Country    string
    Language   string
    LogoURL    string
    EPGID      string
    Streams    []Stream
}

type Stream struct {
    ProviderID string
    URL        string
    Quality    string
    HLS        bool
    Live       bool
    Headers    Headers // User-Agent and Referer only
}

type Source struct {
    // ...
    AllowPrivate   bool     // stored consent for private/local stream hosts
    AllowedSchemes []string // normalized allowlist; http and https always present
}
```

A channel keeps **every** playable variant. A display name is never an
identity. The pair `(source_id, provider_id)` is unique, so two playlists may
contain the same channel name or URL without collapsing.

### Identity

`catalog.ProviderID(explicit, streamURL)` returns:

1. the trimmed explicit provider ID when the playlist supplies one
   (`tvg-id`), or
2. `url:<sha256>` of the canonical stream URL.

Canonicalization lowercases the scheme and host, drops the fragment and keeps
the query. Local paths are made absolute and cleaned. Userinfo in a URL is
rejected, never stored.

Two entries with the same provider ID become one channel with multiple
streams. Different provider IDs with the same display name stay separate.

## Playlist parsing

The parser accepts:

- `#EXTM3U` (missing header produces a warning, not a failure);
- `#EXTINF:<duration> <attributes>,<title>`;
- quoted and unquoted attributes; quoted values may contain commas;
- `tvg-id`, `tvg-name`, `tvg-logo`, `tvg-country`, `tvg-language`,
  `group-title`;
- `#EXTGRP:`;
- `#EXTVLCOPT:http-referrer=...` and `#EXTVLCOPT:http-user-agent=...`;
- `#PLAYLIST:` as the source display name;
- UTF-8 with or without BOM, CRLF, and Windows-1252 fallback;
- local paths and `file://` paths **only** for a local playlist.

Limits and failure behavior:

| Limit | Default |
|---|---|
| Playlist size | 8 MiB |
| Remote JSON (for #8) | 512 KiB |
| Channels | 20,000 |
| Warnings | 100 |
| Line length | 8,192 bytes |
| Redirects | 5 |
| Request timeout | 10 s |

A broken entry is skipped with a warning; the rest of the playlist survives.
A playlist with no usable channel returns `ErrNoChannels`. A malformed remote
source never deletes the previous channel rows.

## Channel list or HLS manifest?

Classification uses content first and `Content-Type` second. The `.m3u8`
extension is not evidence by itself.

| Evidence | Kind |
|---|---|
| `#EXT-X-` tag plus `#EXTM3U` | HLS manifest |
| `#EXTINF`, no `#EXT-X-` | channel list |
| `mpegurl` content type, body only `#EXTM3U` | HLS manifest |
| `#EXTINF` body with an `.m3u` or `.m3u8` URL | channel list |
| `video/*`, `audio/*`, MPEG-TS, MP4, ID3, Matroska | direct stream |
| HTML or plain text | unsupported |

An HLS body is a **single stream**, not a playlist. `Loader.Inspect` returns
`ErrPlaylistNotStream` for the reverse case, so Add Playlist and Add Stream
cannot silently accept the wrong thing.

## Network policy

`netguard.Client` is the only path to an untrusted URL.

- schemes: `http` and `https` only; local files only through a local playlist
  or a file dialog;
- embedded credentials are rejected;
- `localhost`, `.local`, `.internal` and `.home.arpa` names are refused;
- loopback, private, link-local, multicast and unspecified addresses are
  refused in `ValidateURL` and again in the dialer after DNS resolution, so a
  public name that resolves privately is still blocked;
- environment proxies are disabled: they would bypass the address policy;
- every redirect is revalidated by the same policy;
- the response body is read through a hard limit, including a `Content-Length`
  precheck;
- cookies are never stored or sent.

`AllowPrivate` is opt-in for a URL the user typed or a playlist the user chose
in the file dialog. A catalog-provided source keeps it off. Without a stored
consent, remote playlists that reference private or local stream addresses are
skipped and counted in the preview warnings.

## Per-source stream policy (slice 11a)

The document policy above is global; the stream policy is **per source** and
stored, because a user playlist may legitimately point at a LAN origin while
the community catalog never may. See
[ADR 0007](adr/0007-per-source-stream-policy.md).

| Concern | Rule |
|---|---|
| Migration | `0003_source_policy.sql` adds `sources.allow_private` (default `0`) and `sources.allowed_schemes` (default `http,https`) |
| Default for a new source | strict: no private hosts, `http`/`https` only |
| Consent | `preview.NeedsPrivate` asks the import sheet; `preview.AllowPrivate` records the answer and is the only way `Save` stores `allow_private = 1` |
| Reading the playlist document | the user's own location, so a private document host is reachable |
| Streams inside it | `playlist.Options.AllowPrivateHosts` and `AllowedSchemes` from the stored policy |
| Preview | parses with private hosts allowed so it can *report* them; it never fetches a stream |
| Save | revalidates every channel, drops channels with no allowed stream, and reports the count |
| Refresh | reuses the stored policy, so a consent (or its absence) survives |
| Built-in | always strict at parse, mapping, storage and service level |

`catalog.NormalizeSchemes` is the single source of truth for the allowlist: it
trims, lowercases, deduplicates, sorts, always includes `http` and `https`, and
always removes `file`, `udp`, `rtp`, `multicast`, `rtsp`, `rtmp` and `srt`.
`file` remains a local-playlist decision (`playlist.Options.AllowLocalPaths`)
and a stored policy can never add it.

Only `http` and `https` are enabled. `rtsp`, `rtmp` and `srt` stay rejected
until a recorded macOS/libmpv smoke test proves them, and the evidence gap is
tracked in the ADR.

## Playback headers

Only `User-Agent` and `Referer` are accepted, including the M3U spellings
`http-user-agent`, `http-referrer` and the `#EXTVLCOPT` forms. Each value is
limited to 1,024 bytes and control characters are rejected. `Cookie`,
`Authorization` and `Proxy-*` are ignored.

Headers are stored with the stream row so playback can use them later. They
never contain credentials.

`player.Source` carries `Scheme` and `Headers`; the controller accepts only
`http`/`https` and fails with `player.ErrUnsupportedScheme` otherwise. The
libmpv adapter builds one `http-header-fields` value from the sanitized
`User-Agent` and `Referer`, sets it on the pump goroutine immediately before
`loadfile` for that source, and clears it on stop. A value containing a comma
or a backslash is dropped, because mpv parses that option as a comma-separated
list escaped with backslash. The option value is never logged.

Whether those headers actually reach a server through libmpv is verified by
`internal/player/libmpv/headers_integration_test.go`, gated by
`MUMU_MPV_HEADER_TEST=1` on `darwin/arm64` with the `mumu_mpv` tag. It has not
been run in this change; see the ADR evidence gap.

## Storage mapping

`internal/sources` maps the domain to SQLite:

- `Channel.Streams[0]` remains the primary `channels.stream_url` for existing
  queries;
- every variant is stored in `channel_streams` with a stable provider ID;
- `AllowPrivate` and `AllowedSchemes` map to `sources.allow_private` and
  `sources.allowed_schemes` through `EffectivePolicy`, so a built-in is always
  written strict and a stored built-in row is always read strict;
- `ReplaceForSource` upserts channels, replaces streams, removes channels that
  disappeared and records refresh metadata in **one transaction**.

A refresh that fails validation or network access leaves the previous rows
untouched. Channels whose provider ID is unchanged keep their local ID, so
favorites and recent entries survive. A channel that truly disappeared loses
its favorite through the foreign-key cascade; there is no dead link.

## Community catalog (#8)

`internal/catalog/iptvorg` reads the public, per-country playlists:

```text
https://iptv-org.github.io/iptv/countries/id.m3u
https://iptv-org.github.io/api/countries.json
```

The catalog is stored as a `builtin` source with the playlist URL as identity.
The database is the offline cache; there is no separate cache table. Launch
loads cached rows synchronously, then refreshes asynchronously. Refresh uses
`ETag`, a single-flight per source and a six-hour minimum interval.

The community list is labelled **not verified**. It is not a rights clearance.
See [ADR 0006](adr/0006-iptv-org-discovery.md) for the policy decision, the
blocklist and the setting that hides it.

Two blocklists apply before channels are stored:

- the reviewed local `blocklist.json` (channels and hosts), and
- the upstream `https://iptv-org.github.io/api/blocklist.json` (NSFW/DMCA
  exclusions), cached for 24 hours.

A failed blocklist fetch uses the cached list or the local list; it never fails
the catalog refresh. This is risk reduction, not a rights review.

Provider loading reuses the same parser and policy: private stream addresses
are dropped, HTTP is allowed, unsupported schemes are dropped, and a failed
refresh never deletes cached rows. The built-in refresh additionally applies the
strict policy to the parsed channels before storage, so a playlist or a cached
row cannot widen it.

## Import diagnostics

A skipped channel is counted and, up to 20 entries, described with line,
display name, reason, scheme and host. The import preview turns the reasons
into readable lines such as:

```text
3 entries with a private or local host
1 entries with an unsupported scheme
```

No full URL, query string or credential is kept in the preview. A channel list
that skips entries still imports the usable channels; a list with none returns
`ErrNoChannels`.

## Source management (#9, slice 11a)

`internal/sources.Service` owns the user flows:

```go
preview, err := service.PreviewRemote(ctx, url, "My list")
// preview.NeedsPrivate asks the sheet for consent; preview.AllowPrivate
// records the answer.
source, err := service.Save(ctx, preview)

result, err := service.SaveSource(ctx, preview) // also reports Dropped
result, err := service.Refresh(ctx, source.ID)
err = service.SetEnabled(ctx, source.ID, false)
err = service.Remove(ctx, source.ID)
```

Preview parses without storing and reports `NeedsPrivate` plus the distinct
`Schemes` it saw. Save writes the source and replaces its channels in one
transaction, revalidating every channel against the effective policy; a channel
whose streams are all outside it is dropped and counted in `SaveResult.Dropped`,
and a save that keeps nothing returns `ErrEmptyPreview`. Refresh is atomic, uses
the stored policy, and keeps the last known good rows on failure. Built-in
sources are protected from Library saving, removal and enabling; they are
managed by Discover.

## UI surfaces

Discover shows the community list with a visible **not verified** attribution,
a region selector, and rows built from the normalized model. A logo is fetched
through the bounded logo loader; a failed or unsupported image falls back to
the channel initials. `LIVE` describes the stream format, never upstream
availability or legality.

Library owns user playlists and custom streams: a row per source with kind,
enabled state, item count, last success and the last safe error. Import and
Add Stream validate a draft, show a summary and warnings, then save atomically.
Refresh, enable/disable and remove run in the background; a failed refresh keeps
the last known channels. Discover merges enabled user sources with the community
catalog and excludes disabled ones.

## Example

```go
policy := netguard.Policy{} // public destinations only
loader := playlist.NewLoader(policy)

loaded, err := loader.Load(ctx, "https://example.com/list.m3u", playlist.LoadOptions{
    DefaultCountry: "ID",
})
if err != nil {
    return err
}

batch, err := sources.ToStorageBatch(loaded.Channels)
if err != nil {
    return err
}

_, err = store.Channels().ReplaceForSource(ctx, sourceID, batch, storage.RefreshMeta{
    ETag: loaded.ETag,
})
```

Local file:

```go
loaded, err := loader.Load(ctx, "/Users/me/list.m3u", playlist.LoadOptions{})
```

Direct stream validation:

```go
info, err := loader.Inspect(ctx, "https://example.com/live.m3u8", true)
if err != nil {
    return err
}
switch info.Kind {
case playlist.KindHLSManifest, playlist.KindDirectStream:
    // acceptable Add Stream target
}
```

## Not implemented here

- full Discover search, country/category/source filters and channel selection (#10);
- Watch playback, channel drawer and resume (#11/#13);
- `rtsp`, `rtmp` and `srt` playback: rejected, no recorded evidence yet;
- a runtime UI for editing a stored policy after import (re-import asks again);
- probing every stream to claim it is online;
- DRM, authentication, proxying or re-hosting media;
- EPG schedules and time-shift.

## Verification

```sh
go test ./internal/catalog ./internal/netguard ./internal/playlist ./internal/sources ./internal/storage ./internal/player
go test -race ./internal/catalog ./internal/netguard ./internal/playlist ./internal/storage ./internal/player
go vet ./...
```

The libmpv header path additionally has a gated integration test:

```sh
MUMU_MPV_HEADER_TEST=1 go test -tags mumu_mpv ./internal/player/libmpv -run TestIntegrationPlaybackHeaders
```

`BenchmarkParse` on the Apple M3 test machine parses 1,000 channels in about
2.0–2.2 ms per operation with roughly 1.5 MB and 11,000 allocations. It is a
regression baseline, not a product performance budget.
