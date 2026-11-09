# Library and source management (#9)

Native MyGo notes for the source-management slice. The full Library
(favorites, recently watched, resume) is #13; this document covers playlists
and custom streams.

## Layout

```text
┌─ Library ───────────────────────────────────────────────────────────────┐
│ Library                                          [+ Add Playlist]        │
│                                                                          │
│ My Playlists                                                            │
│ ┌──────────────────────────────────────────────────────────────────────┐ │
│ │ Indonesia IPTV        M3U URL   Enabled · 214 channels · 2h ago     │ │
│ │ Local channels        File      Disabled · 12 channels · failed     │ │
│ └──────────────────────────────────────────────────────────────────────┘ │
│ [Add M3U URL]   [Import M3U file]   [Add Stream URL]                     │
└──────────────────────────────────────────────────────────────────────────┘
```

- One row per user source. Built-in community sources never appear here.
- Row: display name, kind label, enabled state, item count, last success and
  the last error when present.
- Actions: Refresh and Remove. Enable/disable is a checkbox or switch with an
  accessible label.
- Remove uses a themed in-app confirmation dialog and states that favorites and
  recent entries for its channels are removed too.
- Busy state disables only that row's actions; the rest of the app stays
  interactive.

## Add Playlist

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

## Add Stream

- URL and display name.
- Validation is an inspection, not a full download: HLS manifest or a
  supported direct media response.
- A channel-list URL is rejected with "add it as a playlist".
- One channel is created under a `custom_stream` source.

## States

| State | Row/action |
|---|---|
| Never refreshed | "Not refreshed yet", Refresh enabled |
| Refreshing | spinner, actions disabled for that row |
| Success | item count and relative last-success time |
| Failure with cache | error text, "Showing last known channels", Refresh enabled |
| Failure without cache | error text, Retry and Remove |
| Disabled | dimmed but readable; Discover excludes it |
| Removed | confirmation, row disappears, navigation unaffected |

Errors must be actionable and credential-free. Never show a full URL with a
query token, a header value, or a stack trace.

## Behaviour

- Refresh parses into memory first, then replaces the source in one SQLite
  transaction. A network or parse failure keeps the previous rows.
- Stable provider IDs keep favorites and recent entries across a refresh.
- Enabling/disabling a source changes Discover immediately and never blocks
  playback or the UI thread.
- Starting an import or refresh is cancellable when the sheet closes or the
  app stops.
- No autoplay, no WebView, no login, no external backend.

## Keyboard and accessibility

- Tab reaches every action; Enter/Space activates.
- The file picker is the native macOS dialog.
- Remove confirmation names the source.
- Error banners use text, not colour alone.
- Focus returns to the row after refresh or cancel.
