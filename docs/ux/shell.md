# Application shell and design system (#4)

Native MyGo implementation notes for the shell, theme tokens and interaction
states. Tokens are defined in `DESIGN.md`; this document records how the app
maps them and what remains a placeholder.

## Layout

Wide window (content width >= 860 pt):

```text
┌──────────────────────────────────────────────────────────────────┐
│ native macOS title bar with traffic lights                       │
├──────────────────┬───────────────────────────────────────────────┤
│ [tomato] Mumu    │                                               │
│                  │  Page heading                                 │
│ Browse           │                                               │
│  ▶ Watch         │  Native MyGo content                          │
│  ◈ Discover      │                                               │
│  ♡ Library       │                                               │
│                  │                                               │
│ Settings         │                                               │
│  ☰ Settings      │                                               │
└──────────────────┴───────────────────────────────────────────────┘
```

Narrow window (content width < 860 pt):

```text
┌──────────────────────────────────────────────────────────────────┐
│ native title bar                                                 │
├──────┬───────────────────────────────────────────────────────────┤
│ [🍅] │                                                           │
│  ▶   │  Page heading                                             │
│  ◈   │                                                           │
│  ♡   │  Native MyGo content                                      │
│      │                                                           │
│  ☰   │                                                           │
└──────┴───────────────────────────────────────────────────────────┘
```

- Sidebar: 208 pt, brand row, Browse section, App section with Settings.
- Rail: 72 pt, icon-only 48x48 targets, tooltip and accessible label per item,
  Settings pushed to the bottom, 3 pt tomato indicator on the active item.
- Default destination is Discover. No welcome, onboarding or autoplay.
- Window minimum stays 640×420; the rail and content must not clip there.

## Theme modes

Dark is the default. Settings offers Dark, Light and System.

- Dark and Light use the token tables in `DESIGN.md`.
- System reads the root window theme, which follows the macOS appearance, and
  selects the matching palette. The app never calls `Context.SetTheme` on the
  root, so this signal keeps working.
- The choice is in-memory until local settings arrive in #6.
- Menus, tooltips, popovers and dialogs inherit the app theme through
  `ui.Themed`.

## Type scale

Sizes derive from the theme's font size with `Theme.Rem`, so desktop text
scaling is respected.

| Role | Rem | Approx. 13 pt |
|---|---|---|
| Page heading | 2.0 | 26 pt, weight 600 |
| Section heading | 1.4 | 18 pt, weight 600 |
| Body | 1.08 | 14 pt, weight 400 |
| Control label | 1.0 | 13 pt, weight 500 |
| Metadata | 0.85 | 11 pt, weight 400 |
| Quiet placeholder | 1.7 | 22 pt, weight 500 |

Spacing uses a 4 pt grid through `Theme.Space`.

## States

Reusable `Status` view:

| State | Use | Action |
|---|---|---|
| Empty | Watch, Discover and Library empty states | Browse Discover on Watch |
| Loading | async catalog, source or playback work | none until data exists |
| Error | failed work, explicit failure text | Retry or relevant action |
| Offline | missing network with cached data | Retry |

Errors always include text; color is never the only signal. Loading uses a
spinner named for assistive technology plus a text label.

Source rows are busy one at a time: only that row's Refresh, Remove and
enable switch disable, so the rest of the page stays interactive.

## Keyboard and focus

- Wide sidebar: one Tab stop; Up, Down, Home, End and first-letter typing choose
  items.
- Rail: each destination is a button in the Tab order; Enter or Space activates.
  Arrow-key sidebar behavior is not available in rail mode.
- Segmented appearance control: one Tab stop; arrows change the segment.
- Focus rings come from the theme (`#FF4D4D` dark, `#D93636` light), at least
  3:1 against the canvas.
- No global media keys and no new app-wide shortcuts.

## Contrast evidence

`internal/ui/theme_test.go` computes WCAG ratios for both palettes. Current
minimums:

- primary text on canvas: 16.7:1 dark, 16.6:1 light
- secondary text on canvas: 9.3:1 dark, 7.2:1 light
- muted text on canvas: 5.9:1 dark, 5.6:1 light
- white on action red: 4.6:1
- danger on canvas: 6.2:1 dark, 5.3:1 light
- warning on canvas: 10.2:1 dark, 4.6:1 light
- success on canvas: 8.2:1 dark, 4.6:1 light
- focus ring on canvas: 5.6:1 dark, 4.3:1 light

## Known limits

- The native titlebar follows macOS, so an explicit Light or Dark theme can
  differ from the titlebar; the content itself is consistent.
- Settings is the last sidebar item, not pinned to the bottom in wide mode.
- The rail loses native sidebar arrow navigation; buttons remain keyboard
  reachable.
- The wordmark uses the system font until final brand artwork review.
- Discover loads the cached catalog at launch and shows loading, empty, offline
  and error states. Search, filters and channel selection remain #10.
- Library manages playlists and custom streams; favorites, recently watched and
  resume remain #13.
- Theme preference is not persisted yet.
- Sheet height and native file-dialog behaviour need a manual macOS pass at the
  minimum window size.
