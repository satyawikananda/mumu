# 0006 — iptv-org as a community discovery list

- Status: accepted
- Date: 2026-11-09
- Issues: [#8](https://github.com/satyawikananda/mumu/issues/8),
  [#7](https://github.com/satyawikananda/mumu/issues/7)

## Context

mumu must open Discover with a usable public catalog and no setup, while
`AGENTS.md` §9 says the built-in catalog may contain only reviewed and
authorized sources. iptv-org is a discovery dataset released under CC0; it is
not a content licence for the streams it lists. Channel owners keep their
rights, and individual streams may be geo-blocked, offline or unauthorized.

## Decision

mumu ships iptv-org as an explicitly labelled **community list**, not as a
cleared catalog:

- Discover shows the label "Community channel list · iptv-org · not verified".
- The list is fetched per country, starting with Indonesia, and cached as a
  `builtin` source in SQLite.
- A local blocklist can remove channel IDs and hosts at parse and refresh time.
- The upstream iptv-org blocklist (`api/blocklist.json`, NSFW/DMCA reasons) is
  cached and merged with the local list. It is a policy input, not a rights
  review.
- A setting can hide the community catalog entirely; user playlists are
  unaffected.
- mumu never claims a stream is online, legal or available. Channel format and
  stream availability stay separate.
- mumu never re-hosts, proxies or transcodes third-party video.
- Attribution and the rights statement appear in Settings/About and the
  repository documentation.

The owner accepted #8 with this scope. The decision must be revisited if this
stops being acceptable, not silently widened.

## Consequences

- The app can offer a no-setup catalog without pretending to clear rights.
- Some channels are skipped: private-network hosts, unsupported schemes and
  blocklisted entries.
- Upstream data changes daily. The last successful refresh remains visible
  offline, with an explicit refresh action.
- A blocklist is a mitigation, not a legal review.
- Removing the community catalog from the app is a single setting and a
  code deletion; user imports do not depend on it.

## Alternatives considered

- **Manual curation only.** Safest but too slow for a usable first launch and
  duplicates work already published by iptv-org.
- **`api/channels.json` plus `api/streams.json`.** Eight and four megabytes for
  the full index, and still no rights review. Per-country playlists are smaller
  and match the product's region-first discovery.
- **Require the user to add the list.** Contradicts the no-setup launch flow in
  `DESIGN.md` and #8.
- **Present only verified streams.** Verification is not feasible without
  probing every stream, and probing is misleading and impolite. The product
  does not make that promise.
