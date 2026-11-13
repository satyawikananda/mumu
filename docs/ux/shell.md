# Application shell and Control Center

Native MyGo implementation notes for the M5 revamp. The visual contract is
[`DESIGN.md`](../../DESIGN.md); tokens, components and frame IDs are in the
[M5 handoff](revamp/README.md).

## Navigation

The permanent sidebar and 72 pt rail are removed. Every page uses one top bar:

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ tomato mumu  HOME/DISCOVER/...          [Control Center]  [Settings]     │
├──────────────────────────────────────────────────────────────────────────┤
│ page content                                                             │
│                                                                          │
│ shortcut strip                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

- The top bar is inside the main window below the native titlebar. It does not
  create custom traffic lights or a custom window shape.
- The bar has no background while the current page is at its start, so Home's
  synthesized art passes behind it. Once the page scrolls under it, the bar
  draws a translucent canvas scrim with a bottom hairline.
- The scrim is not a native blur. MyGo has no backdrop filter; claiming glass
  would be false. The translucency is a canvas color at 86% opacity.
- Scroll position comes from the page's own `ui.ScrollState`
  (`TrackScroll`): Home, Discover, Library sections and Settings each keep one.
- The brand lockup and the labelled **Control Center** action both open the
  overlay. Control Center is never keyboard-only.
- **Home**, **Discover** and **Library** are primary destinations.
- **Watch** is reached from Resume/active-session cards and the Control Center
  channel action. It is not a permanent sidebar item.
- **Settings** is reachable from the top-bar gear and is secondary. Its
  Appearance/Shortcuts/About rail and panel scroll are documented in
  [Settings](settings.md).
- Fresh install opens Discover. A later launch with a valid recent channel
  opens Home. Neither path autoplays.
- Home is a feed, not active playback. Watch is the only page that owns the
  native video box.
- Screen-local Settings and category rails remain page content; they are not a
  permanent shell.

## Control Center

Control Center is a native `ui.Overlay` built after the page. It does not change
the destination, so opening it over Watch leaves the retained video surface
attached and playback running.

- Black gradient scrim from the lower page toward the bottom; text areas stay
  above 60% opacity.
- Three context cards from real state:
  - Resume or Watch, depending on the active session.
  - Favorites with the published count and up to three real names.
  - Playlists with the published user-source count and names.
  - With no resume/session, the first card is Browse Discover.
- Five navigation/action icons, as approved:
  - Home
  - Discover
  - Watch
  - Picture-in-Picture
  - Close Control Center
- Library stays reachable from the Favorites and Playlist cards. Settings stays
  reachable from the top-bar gear.
- `FocusGroup` moves the cards and icons with the arrows. Enter presses the
  focused control. Escape, outside click and the Close icon close the overlay.
- `Modal` traps the focus inside the overlay. Closing with Escape restores the
  keyboard focus to the element that opened it.
- The base page hint bar is hidden while the overlay is open; Control Center
  renders its own hint strip.

## Home shell (#34)

Home is deliberately minimal until #35:

- `LAST WATCHED` with the real recent channel and source.
- Explicit **Resume** and **Other channels**.
- `NOW PLAYING` with **Open Watch** when a session is active.
- `Nothing to resume` with **Browse Discover** when there is no recent channel.
- No hero rails, recent/favorite rails or curated collection cards yet.

## Shortcuts

One registry in `internal/ui/shortcuts.go` serves handlers, hint bars and
Settings.

| Context | Keys | Action |
| --- | --- | --- |
| Player | `M` | Mute |
| Player | `↑` / `↓` | Volume |
| Player | `F` | Fullscreen |
| Player | `P` | PiP / Return |
| Player | `C` | Channel drawer |
| Player | `Esc` | Control Center |
| Browse | `/` | Search |
| Browse | `←` / `→` | Move between cards |
| Browse | `⏎` | Open |
| Browse | `F` | Favorite |
| Browse | `Esc` | Back |
| Home | `/` | Search |
| Home | `⏎` | Resume |
| Home | `Esc` | Control Center |
| Control Center | `←` / `→` | Move |
| Control Center | `⏎` | Open |
| Control Center | `Esc` | Close |

- Single-key shortcuts wait while `Context.FocusTakesText()` is true.
- `Space` is defined and advertised because live pause is implemented.
- No global media keys and no app-wide quit shortcut.

## Escape order

1. A native dialog or sheet (Library import) closes itself first.
2. Control Center closes and restores focus.
3. An open Watch channel drawer closes.
4. Home and Watch open Control Center.
5. Discover, Library and Settings go back to the previous destination.
6. No step quits the app.

## Appearance

- Source of truth: `internal/ui/tokens.go`.
- Dark is the default. Settings offers Dark, Light and System.
- `themeForContext` follows desktop text scale and contrast, then restores the
  mumu brand accent and focus ring.
- Display: Lexend. Body and controls: Inter Tight. Both OFL families are
  embedded; no runtime font download.

## Verification

- Shell tests cover the removed sidebar, pointer and keyboard navigation,
  Home/Watch separation, Control Center over playback, Escape layers, focus
  restoration, input-field conflicts, compact layout and shared shortcut
  labels.
- App attach tests cover `Control Center` over playing Watch: attach count and
  `media.attached` stay unchanged; leaving Watch for Home detaches.
- Still required on macOS: overlay above actual libmpv video, Reduce Motion,
  Light/System, focus ring visibility with keyboard, outside click, and the
  three reference sizes.
- Screenshots stay out of git. Frame IDs remain in the handoff.
