# Library, favorites, recent viewing and resume

The Library is the user's own shelf: favorite channels, recently watched
channels and the playlists or custom streams they added. It is not a second
catalog. The community list stays in Discover.

Related code: `internal/library`, `internal/storage` and
`internal/ui/library.go`.

## Purpose

The Library is the user's own shelf: favorite channels, recently watched
channels and the playlists or custom streams they added. It is not a second
catalog. The community list stays in Discover.

## Entries

`internal/library` owns the user's viewing intent over the catalog:

```go
type Entry struct {
    Key       string           // sourceID + ":" + providerID
    Channel   catalog.Channel
    Source    catalog.Source
    PlayedAt  time.Time        // zero for favorites
    PlayCount int              // zero for favorites
    Playable  bool             // source enabled and an http(s) stream exists
}
```

- `Favorites` reads the stored favorites, most recently added first.
- `Recent` reads the bounded history (100 rows), most recently played first.
- `Last` is the resume reference: the newest recent entry, if any.
- `ToggleFavorite` resolves `(sourceID, providerID)`, then adds or removes the
  favorite and returns the resulting state. It is idempotent.

`Playable` is a local policy check: the source must be enabled and at least one
stream variant must parse as an HTTP(S) URL with a host. It never performs
network I/O, so an unavailable upstream still looks playable. Only playback
evidence can prove availability.

## Identity

`Key` is `sourceID + ":" + providerID`. A display name is never an identity.
Two playlists may contain channels with the same name, or even the same explicit
provider ID; their keys stay different because the source ID is part of the key.
The Library shows the source name on every row so the viewer can tell them
apart.

Favorites and recent entries reference the stored channel row. Removing a
source deletes its channels, and the foreign key cascade removes the matching
favorites and recent entries. A removed channel therefore leaves no dead link.

A refresh that keeps a provider ID keeps the local channel row and its
favorites. When a provider changes its ID but keeps the primary stream URL, the
storage layer remaps favorites and recent entries to the new row before deleting
the old one; see [storage](storage.md#atomic-refresh). Remapping matches only an
exact stream URL inside the same source. It never matches a display name and
does not survive a rotated URL token. If one source lists the same URL twice,
each surviving channel receives a copy, so no favorite is lost.

## Page

```text
Library                                    [Add Playlist]
[Favorites] [Recently watched] [Playlists]

Favorites
┌──────────────────────────────────────────────────────────────┐
│ (avatar) TVRI Nasional          [LIVE]  [Remove from Favorites]│
│          News · Indonesia IPTV                                │
└──────────────────────────────────────────────────────────────┘
```

- Three tabs keep one virtualized list visible at a time, so scroll, selection
  and keyboard order stay predictable.
- A row shows the channel, its group, its source and a favorite action. Recent
  rows also show when the channel was watched.
- Enter or a double click plays the row through the same controller as
  Discover and Watch.
- A row whose source is disabled or which has no supported stream says so and
  does not start playback. The app publishes the guidance as an inline banner.
- Favorites and recently watched have their own empty states. Both offer
  Browse Discover; the playlists tab keeps every import path.
- A search field narrows the visible channel section by channel or source name.
  It is local filtering, not a catalog query.

## Favorite consistency

One cache in `internal/app` holds the favorite keys. Watch, Discover and Library
read from it:

- `Discover.SetFavorites` receives a copy of the map. A favorite change never
  re-filters or republishes the catalog.
- `Watch` derives its button label from the same key.
- `Library` rows receive the state when the app publishes its snapshot.

A toggle updates the cache optimistically, publishes all three pages, then
writes through `library.ToggleFavorite`. A failed write reverts the key and
shows an actionable banner. Repeated clicks on one key are ignored while its
write runs.

The key includes the source ID. Comparing only the provider ID would mix up two
sources that publish the same `tvg-id`.

## Resume

`library.Last` supplies the resume card on launch. The card appears only when
the source still exists, is enabled and has a supported stream.

- Nothing autoplays. Resume is a button that calls the same play path as a
  channel selection.
- A missing, removed or unplayable recent entry produces no card; Discover stays
  the current destination.
- `Recent.MarkPlayed` runs once per playback episode, after the controller
  reports `playing`. A click alone is not recent activity.

## Threading and lifetime

- Library reads run in background jobs with a 30 second bound and are cancelled
  when the app stops.
- The UI thread only receives finished snapshots through `Window.Update`.
- The page stores model values only, never elements or parts.
- Toggling a favorite and refreshing a source may run together; the favorite
  write is serialized per key, and storage serializes writes.

## Verification

Automated:

- `internal/library` covers ordering, the recent bound, idempotent toggles,
  missing channels, disabled sources and non-HTTP streams.
- `internal/storage` covers remapping on refresh and the cascade for a channel
  that truly disappeared.
- `internal/ui` covers tabs, empty states, search, duplicate names, unavailable
  rows, favorite labels and row activation.
- `internal/app` covers launch resume, cross-page favorite consistency,
  source-scoped identity and the disabled-source message.
- `go test ./...`, `go test -race ./...`, `go vet ./...`,
  `go tool mygo vet .`, `gofmt -l ./cmd ./internal` pass locally; golangci-lint
  runs in CI.

Native macOS smoke test: favorite a channel from Discover, check Watch and
Library, restart and confirm the resume card appears without audio, disable the
source and confirm the row explains itself, then remove the source and confirm no
row or dead link remains.
