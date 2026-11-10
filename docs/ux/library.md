# Library: favorites, recent viewing and sources

Covers [#9](https://github.com/satyawikananda/mumu/issues/9) and
[#13](https://github.com/satyawikananda/mumu/issues/13). The playback and
identity rules live in [`docs/architecture/library.md`](../architecture/library.md).

## Layout

```text
┌─ Library ───────────────────────────────────────────────────────────────┐
│ Library                                          [+ Add Playlist]        │
│ [Favorites]  [Recently watched]  [Playlists]                             │
│                                                                          │
│ Favorites                                                                │
│ ┌──────────────────────────────────────────────────────────────────────┐ │
│ │ (TV) TVRI Nasional          News · Indonesia IPTV  [LIVE] [Remove]   │ │
│ │ (TV) Metro TV               News · Local channels        [Add]       │ │
│ └──────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

- One tab at a time keeps a single virtualized list, so scroll, selection and
  keyboard order stay predictable.
- Every channel row shows the display name, its group, its source name and a
  favorite action. The source name keeps two channels with the same name apart.
- Recent rows add when the channel was watched. Played time is relative for the
  first week, then an absolute date.
- Icons always carry a label: `Add to Favorites` and `Remove from Favorites` are
  distinct accessible names.

## Tabs

| Tab | Rows | Empty state |
|---|---|---|
| Favorites | favorite channels, most recently added first | No favorites yet · Browse Discover |
| Recently watched | channels that genuinely played, newest first | Nothing watched yet · Browse Discover |
| Playlists | user sources only; built-ins never appear | No playlists yet · every import path |

Favorites and recently watched have a search field. It filters the visible rows
by channel or source name on the UI thread. The Playlists tab keeps Refresh,
Remove, enable/disable and the import sheet.

## Channel row actions

- `Enter` or a double click plays the row through the same controller as
  Discover and Watch.
- `Add to Favorites` and `Remove from Favorites` update the row, Watch and
  Discover through one shared favorite cache.
- A disabled source shows `Source disabled. Enable it in Playlists.`
- A channel with no supported stream shows `No supported stream.`
- Rows that cannot play stay readable and keep their favorite action. Activating
  one publishes the guidance as an inline banner instead of starting playback.

## Removal and unavailable sources

- Removing a playlist also removes its channels, favorites and recent entries.
  The confirmation states this before the removal.
- Deleting a source never leaves a dead Library row: storage cascades.
- A refresh that drops a channel removes its rows. A refresh that changes only
  the provider ID remaps the favorite to the new channel when the primary stream
  URL is unchanged.
- A disabled source keeps its channels; rows stay visible but not playable.

## Playlists

Row: display name, kind label, enabled state, item count, last success and the
last error when present.

- Actions: Refresh and Remove. Enable/disable is a switch with an accessible
  label.
- Busy state disables only that row's actions; the rest of the app stays
  interactive.
- Refresh parses into memory first, then replaces the source in one SQLite
  transaction. A failure keeps the previous rows.

## Add Playlist and Add Stream

```text
[Add Playlist]
   ├── M3U URL      TextInput + Validate and Preview
   └── M3U File     Choose File... (native dialog) + Validate and Preview
```

Preview shows:

- detected type: channel list;
- display name and location;
- channel count and up to five sample names;
- skipped lines with reason counts (private host, unsupported scheme,
  credentials, invalid URL) and the parser warnings;
- explicit error text for a single HLS stream, malformed file, unsupported
  scheme, private host from a remote playlist, or an empty list.

Import is enabled only after a successful preview. Import saves immediately and
closes the sheet. A failed import never leaves a partial source row.

Add Stream takes a URL and a display name. Validation is an inspection, not a
full download: HLS manifest or a supported direct media response. A channel-list
URL is rejected with "add it as a playlist". One channel is created under a
`custom_stream` source.

## States

| State | Row/action |
|---|---|
| Never refreshed | "Not refreshed yet", Refresh enabled |
| Refreshing | spinner, actions disabled for that row |
| Success | item count and relative last-success time |
| Failure with cache | error text, "Showing last known channels", Refresh enabled |
| Failure without cache | error text, Refresh and Remove |
| Disabled | dimmed but readable; Discover excludes it |
| Removed | confirmation, row disappears, navigation unaffected |
| Library read failed | full-page error, Retry |

Errors must be actionable and credential-free. Never show a full URL with a
query token, a header value, or a stack trace.

## Keyboard and accessibility

- Tab reaches the tabs, the search field and every row action; Enter/Space
  activates.
- Arrow keys move inside the visible list; Enter opens the focused channel.
- A cancelled dialog returns focus to the control that opened it.
- The file picker is the native macOS dialog.
- Remove confirmation names the source.
- Error banners use text, not colour alone.
- Icons and favorite buttons expose their state, not just their colour.

## Behaviour

- No autoplay, no WebView, no login, no external backend.
- Favorite and recent writes are local and bounded. History keeps at most 100
  entries and records only playback that reaches `playing`.
- Starting an import or refresh is cancellable when the sheet closes or the app
  stops.
