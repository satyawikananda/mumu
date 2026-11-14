# Discover — implementation notes (#36)

Status: implemented on 12 November 2026 for the M5 revamp. This note records
what Discover shows, which state owns which copy, and what stays unverified.

Design references, all in `mumu-design.pen`:

| Frame | Screen |
| --- | --- |
| `m79WoJ` | Discover — no country selected |
| `NYvSY` | Discover — wide 1470x956 |
| `xztVf` | Discover — compact 1024x640 |

Implementation source of truth:

- `internal/ui/discover.go` — page state, layout, infinite-scroll card grid, search grid,
  country picker, prompts and keyboard cursor.
- `internal/app/discover.go` — catalog index, country persistence, paging and
  generation guards.
- `internal/sources` — region preference and the no-region contract.
- `internal/catalog` — `Filter.LiveOnly` over real stream data.

## First launch

Fresh install opens Discover with **no country chosen**. The page never fetches
an Indonesia or other country playlist until the viewer picks one:

- `sources.Service.Region` returns an empty string when nothing is saved, and
  reports an unknown saved code as empty instead of substituting a country.
- `BuiltinSource` and `RefreshBuiltin` reject an empty region with
  `sources.ErrNoRegion` before any provider or network call.
- `ui.NewDiscover` starts in `DiscoverNoCountry`, so the first frame is already
  the approved state rather than a loading or empty one.
- Only the country metadata list and enabled user sources load. The metadata
  list is allowed before a choice; a country playlist is not.

## States

| Kind | Copy | Actions |
| --- | --- | --- |
| `DiscoverNoCountry` | "Choose a country to get started"; frame description; categories rail waits with its own line | Select a country; header Add Playlist |
| `DiscoverDisabled` | "Community catalog is off" | Open Settings |
| `DiscoverLoading` | skeleton grid in the responsive column count, labelled "Loading channels" | none |
| `DiscoverOffline` | cached rows with a refresh banner, or "You are offline" when nothing is cached | Retry |
| `DiscoverEmpty` | "No channels yet" | Retry |
| `DiscoverError` | "Channels unavailable" | Retry |
| `DiscoverReady` | card grid | Read a channel |

A zero-match narrowing is not an empty catalog: it shows "No channels match"
with **Clear filters**.

## Layout

- Header: page heading and Add Playlist, which keeps the existing
  `OpenAddPlaylist` path (preview, native file picker, validation).
- Filter row: the native search field, a Country label, and the country chip
  (`Choose a country` or the saved country). The glass search pane was
  reverted: it made the field read as disabled in the window. The source and Live only chips were removed on maintainer request;
  `catalog.Filter.LiveOnly` stays available in the domain layer but the page no
  longer exposes it.
- Discover tiles draw LIVE as a brand tag attached to the tile's top-right
  corner. Home rails keep the bottom-left badge, so the shared `ChannelTile`
  only changes where `LiveCorner` is set.
- Wide body: the category rail takes a third and the channel grid takes the
  rest. Medium keeps a 236 pt rail. Compact hides the rail and shows category
  chips that scroll sideways.
- Responsive columns stay 5 / 4 / 4 from `layoutFor`, with tile heights
  206 / 214 / 186.
- Results show a title, a truthful subtitle (`348 results · grid view`) and the
  community attribution line above the grid.
- The grid scrolls; more rows load as it nears the end, so every match stays
  reachable without pagination.

Deviation from the frames: neither frame shows infinite scroll or a loading
skeleton. The approved behaviour now loads 40 rows per batch, returns every
match reachable by scrolling, and caps one publish at 2000 rows for safety.

## Browsing and search

- Browsing (`Query == ""`) is an infinite scroll. The first publish loads 40
  rows; when the grid comes within 400 DIPs of its end, `OnLoadMore` adds the
  next batch from the index, with no network call. `Total` counts every match,
  and `HasMore` says whether more rows exist than were loaded.
- The first catalog load shows a skeleton grid in the current column count.
  MyGo has no skeleton widget, so it follows the documented `ui.Box` and
  `Loop` pulse pattern; the same shape is drawn below loaded rows while the
  next batch arrives.
- Search (`Query != ""`) uses the same card grid and the same 40-row batches.
  A click opens the channel; keyboard arrows move the grid cursor. The grid is
  capped at 2000 rows and reports the real total; `Truncated` adds "first N
  shown".
- Source, category and Live only filters narrow both paths. The page now offers
  the country filter and the category rail; the source menu and the Live only
  chip were removed as no longer relevant. Category counts are recomputed for
  the current query narrowing, and ignore the selected category so its own
  count stays honest.
- Duplicate display names stay separate: identity remains the source-scoped
  provider key, not the name.

## Country picker

The country chip opens a modal with a search field and a bounded list of the
published countries. It shows loading and error states with Retry, closes on
Escape or a click outside, and reports the chosen code through `OnRegion`. The
app persists the choice, cancels the previous catalog and filter runs, and
starts again from the saved country. A saved code the catalog no longer lists
returns to the no-country state with an explicit message; it is never replaced
with another country.

## Imported sources

Enabled user playlists and custom streams stay usable before a country is
chosen. When they exist, Discover shows them instead of the empty prompt, with
"Channels" as the title and no category rail until a country is selected. The
country chip keeps offering the catalog.

## Keyboard

| Key | Action |
| --- | --- |
| `/` | Focus search |
| arrows | Move the grid cursor in browse and search |
| Enter | Watch the focused tile |
| `F` | Toggle the focused channel's favorite |
| Esc | Shell behaviour (back or Control Center) |

Hover scales a tile; keyboard focus uses the theme ring. The page never
claims the keyboard on load: the search shortcut, a click or the arrow keys
choose the first focus target, so typing in search is never stolen. Hints come from the one
registry in `internal/ui/shortcuts.go`: `Search`, `Move`, `Watch`,
`Favorites`, `Close`.

## Verification

Automated, from the repository root:

```sh
gofmt -l ./cmd ./internal
go vet ./...
go tool mygo vet .
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
golangci-lint run ./cmd/... ./internal/...
```

Reference captures are written only when `MUMU_CAPTURE_DIR` is set and are not
committed:

```sh
MUMU_CAPTURE_DIR=/private/tmp/mumu-captures go test ./internal/ui/ -run 'TestDiscoverReferenceSizes|TestDiscoverNoCountryCapture'
```

Native macOS checks still required by a human: logo bitmaps from the real
catalog, Light/System appearance, Reduce Motion, hover versus Tab focus, the
country modal over a real window, and the compact scroll range. Nothing here
claims stream availability, legality or that a country playlist plays.
