# Home: resume hero and rails

Covers [#35](https://github.com/satyawikananda/mumu/issues/35). The playback
rules live in [`docs/architecture/watch.md`](../architecture/watch.md); the
identity and history rules live in
[`docs/architecture/library.md`](../architecture/library.md).

## Layout

```text
┌─ Home ───────────────────────────────────────────────────────────────────┐
│ [top bar: transparent at the top, canvas scrim once scrolled]            │
│ [LIVE] LAST WATCHED                                                      │
│ Metro TV                                                                 │
│ Indonesia · News · 480p · 12 minutes ago                                 │
│ [Resume] [Other channels]                                                │
│                                                                          │
│ Recently watched                                              4 channels  See all
│ [card] [card] [card] [card]                                              │
│                                                                          │
│ Favorites                                                     8 channels  See all
│ [card] [card] [card] [card] [card] [card]                                │
└──────────────────────────────────────────────────────────────────────────┘
```

- The art layer is synthesized from the hero channel: a 145° gradient from its
  bounded tint into the page canvas, a soft glow from the same tint, and a
  bottom fade into the canvas. No brand-colored wash is added, so two channels
  with different artwork never fight the app accent. The art reaches the window
  edge behind the transparent top bar. The layer is decorative and never takes
  input.
- The hero carries only real facts. Country, group, quality and relative time
  are joined when present; an absent value is omitted, never replaced by a
  placeholder.
- Rails use the shared `ChannelTile` and `Rail` components from #33. Card count
  follows `layoutFor`: 6 wide, 5 medium, 4 compact.
- Compact keeps **both** rails and scrolls the page. This is a deliberate
  deviation from the compact Pencil frame `R7ZZ2`, which shows Favorites only:
  the maintainer preferred keeping Recent reachable without leaving Home.
- `See all` opens Library on the matching tab.
- Empty Home offers Discover and the playlist import sheet. There is no welcome
  wizard and no automatic playback.

## Data contract

`internal/app/home.go` builds `ui.HomeState` from the shared `libraryCache` and
the playback state. It reads no storage and performs no network work on the UI
thread.

| Field | Source |
|---|---|
| `Hero` | the most recent entry when `library.Entry.Playable`, or the active session |
| `Recent` | recent entries, capped at `homeRailLimit` |
| `Favorites` | favorite entries, capped at `homeRailLimit` |
| `RecentCount`, `FavoriteCount` | full saved counts |
| `LogoURL` / artwork | `internal/logos`, published through the shared cache |

- `HomeActive` means the session is loading, buffering or playing; the hero
  action becomes `Open Watch`.
- `HomeLoading` covers a pending library read, `HomeFailed` a read error with
  Retry. Neither is an empty feed.
- The same channel can appear in both rails. Focus targets are prefixed per rail
  (`recent:`, `favorite:`) so identity stays unique.
- Artwork warm-up covers Home and the Library rows, capped by
  `homeArtworkLimit`; a missing or rejected logo keeps the monogram in a fixed
  box.

## Resume and stale references

- Launch with a valid recent channel opens Home and never creates a media load
  or audio.
- An unusable last entry (missing, disabled source, no supported stream) leaves
  Discover current. Home, when opened explicitly, still lists saved rails.
- `Resume` resolves the stored key against the current library cache at click
  time. A removed or disabled channel reports an actionable banner and never
  starts playback.
- Recording history still happens only after playback reaches `playing`; a
  failed attempt never becomes recent or a resume target.

## Keyboard

- Focus is app-owned (`ui.FocusBind`): `resume`, `other`, then one target per
  card.
- `←`/`→` move inside the focused rail; `Tab` reaches the hero buttons and the
  rails.
- `Enter` activates the focused card, or the hero action when no card has focus.
  The hint reads `Resume` on the hero and `Open channel` on a card.
- `/` focuses Discover search, `Esc` opens Control Center.

## Verification

`internal/ui/home_test.go` covers the resume contract, wide and compact rails,
reference-size card caps, empty/loading/failed states, `See all`, duplicate
channels across rails, disabled cards and keyboard movement.
`internal/app/home_test.go` covers the persisted feed, capped rails, favorite
sync, real-playback history, disabled/removed resume targets and source
provenance.

Set `MUMU_CAPTURE_DIR` to write `home-wide.png`, `home-medium.png` and
`home-compact.png` while `TestHomeReferenceSizes` runs. Captures are evidence
only and stay out of git.
