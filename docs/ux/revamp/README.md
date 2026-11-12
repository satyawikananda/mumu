# Mumu M5 design handoff — v1

Portable handoff for the native UI revamp tracked by
[#30](https://github.com/satyawikananda/mumu/issues/30). This version was
recorded on 12 November 2026 from the approved Pencil document.

- Source: `mumu-design.pen` (Pencil document)
- SHA-256: `f39be56bb96600ab0f12047ef2174a771c880757433ad048099b130237013c2a`
- Spec sheet: `E8GrO` — Spec Sheet — Mumu PS5 v1
- Go skeleton `oAQ4J` remains a reference only. It is not part of the app.
- Implementation source of truth: `internal/ui/tokens.go`, `internal/ui/fonts.go`,
  `internal/ui/components.go`, `internal/ui/channel.go`,
  `internal/ui/controlcenter.go`.

This handoff covers the foundation delivered by
[#33](https://github.com/satyawikananda/mumu/issues/33), the native shell
delivered by [#34](https://github.com/satyawikananda/mumu/issues/34), the Home
feed delivered by [#35](https://github.com/satyawikananda/mumu/issues/35) and
the Discover page delivered by [#36](https://github.com/satyawikananda/mumu/issues/36).
Watch, PiP and Settings page integration remain owned by #37–#40.

## Scope

In scope:

- Semantic appearance tokens for Dark, Light and System.
- Typography, bundled fonts, Lucide icon subset and licenses.
- Shared native components and their interaction states.
- Responsive layout policy for 1470x956, 1280x720 and 1024x640.
- Motion, reduced motion and keyboard-focus behaviour.
- Portable frame IDs and implementation checks. Screenshots stay out of git.

Not in scope:

- Curated playlist collections and their creation, validation and queue
  behaviour. Those frames are marked **v0.2.0** below.
- Building the final Home feed or Discover page. #35 and #36 own those screens.
- Watch/PiP visual completion. #38/#39 own those screens.
- Claiming Watch/PiP runtime completion. #37–#39 and #41 own that evidence.

## Appearance tokens

These are semantic values. Components take them from `ui.Themed`; views do
not contain raw hex values.

| Token | Dark | Light | Use |
| --- | --- | --- | --- |
| `bg` | `#0A0A0C` | `#F7F7F8` | Window and page canvas |
| `surface` | `#17171C` | `#FFFFFF` | Fields, rows, cards |
| `surface-2` | `#23232B` | `#E7E7EA` | Hover and raised surfaces |
| `ink` | `#F5F5F7` | `#18181B` | Primary text |
| `muted` | `#9298A1` | `#5C5C63` | Secondary text and metadata |
| `brand` | `#F82028` | `#D93636` | Brand dot, decorative accent |
| `brand-fill` | `#D93636` | `#C22B2B` | Fill behind white text |
| `ring` | `#FFFFFF` | `#18181B` | Keyboard focus ring |
| `border` | `#3A3A3F` | `#D5D5DA` | Control outlines and separators |
| `video` | `#09090B` | `#09090B` | Letterbox behind native video |
| `success` | `#4DC19A` | `#157F5F` | Confirmed success |
| `warning` | `#F1B85B` | `#A16207` | Limited capability, buffering |
| `danger` | `#F36B68` | `#C22B2B` | Error and unavailable |

Measured WCAG contrast:

- white on `brand-fill` dark: **4.63:1**
- white on `brand-fill` light: **5.72:1**
- muted on `surface-2` dark: **5.37:1**
- muted on `surface-2` light: **5.37:1**
- `brand` on `bg` dark: **4.93:1**
- `ring` on `bg` dark: **19.78:1**
- `ring` on `bg` light: **16.55:1**

### Approved contrast override

`#F82028` with white text measures **4.02:1**, below the required 4.5:1.
Text-bearing fills therefore use `brand-fill` (`#D93636` dark,
`#C22B2B` light). `#F82028` remains the brand, dot and decorative accent.
Do not place small white text directly on `#F82028`.

## Typography

The issue text named Inter and JetBrains Mono. The maintainer approved this
replacement for the revamp:

| Role | Family | Weight | Size |
| --- | --- | --- | --- |
| Display / hero | Lexend | 700 | 76 / 60 / 48 |
| Title / section | Lexend | 600–700 | 19–34 |
| Wordmark | Lexend | 700 | 22 |
| Body / channel name | Inter Tight | 400–600 | 15 |
| Metadata | Inter Tight | 400 | 12 |
| Label / LIVE caps | Inter Tight | 600–700 | 10, tracking 0.7 |
| Keycaps and hints | Inter Tight | 600 | 11 |

- `Theme.Font` is Inter Tight.
- Display text requests `Lexend, Inter Tight`.
- Sizes are scaled from the 14 pt body base with `Theme.Rem`; desktop text
  scaling therefore grows the whole hierarchy.
- Static Latin faces are bundled under `internal/ui/fonts/`. There is no
  runtime font download and no custom font requirement on the host.
- Licenses: SIL Open Font License 1.1 for both families in
  `resources/licenses/`. The static Latin faces came from Fontsource 5.3.0;
  the icon subset is Lucide 1.72.0 (ISC).

## Components

| Component | Source | States | Notes |
| --- | --- | --- | --- |
| `ChannelTile` | `internal/ui/channel.go` | default, hover, focus, selected, disabled, loading, error | 206/214/186 height; gradient artwork, monogram fallback, cropped logo card |
| `ChannelRow` | `internal/ui/channel.go` | default, hover, focus, selected, disabled | 48 pt row for virtualized search and playlist results |
| `ChannelGrid` | `internal/ui/channel.go` | responsive | 5/4/4 columns, caller caps a page at 20 items |
| `Rail` | `internal/ui/channel.go` | responsive | 6/5/4 cards from `layoutFor` |
| `ControlCenterCard` | `internal/ui/controlcenter.go` | default, hover, focus, disabled | 262x158 reference; action hint sits outside the card |
| `ControlCenter` | `internal/ui/controlcenter.go` | open, closed, focus, outside click, Escape | Five approved icons; cards use real Watch/Library state |
| `Home` | `internal/ui/home.go` | loading, empty, resume, active, failed | Data-backed feed: synthesized art, explicit Resume, recent and favorites rails, `See all`, keyboard cursor |
| `ShortcutReference` | `internal/ui/shortcuts.go` | static | Player/Browse/Control Center groups from one registry |
| `Badge` | `internal/ui/components.go` | live, neutral, warning, danger | Non-interactive; text always carries the state |
| `Chip` | `internal/ui/components.go` | default, active, hover, focus, disabled | Active changes fill, border and weight |
| `Keycap` | `internal/ui/components.go` | static | 19 pt high, radius 5 |
| `HintBar` | `internal/ui/components.go` | static | 34 pt high, max 5 hints |
| `FocusHint` | `internal/ui/components.go` | shown, hidden | Attaches below the focused item |
| `BrandLockup` | `internal/ui/components.go` | static | Tomato mark plus lowercase wordmark |
| `Status` | `internal/ui/status.go` | empty, loading, error, offline | Icon plus text plus explicit action |

Interaction rules:

- Hover scales or changes color. It never draws the focus ring.
- Keyboard focus uses the theme ring. Components set the ring inside the
  element (`FocusRingOffset(-2)`) so clipped cards still show it.
- One focus ring is visible at a time; the page owns focus movement.
- Stable identity comes from `Context.Key` or `ListState.ItemKey`.
- No element or widget part is stored in app state.
- Reduced motion is handled by MyGo: values and poses go to their target at
  once. Components do not run their own timer.

## Motion

| Token | Value |
| --- | --- |
| Hover | 160 ms, `EaseOut` |
| Overlay | 220 ms, smooth spring |
| Focus movement | `ui.Snappy` |
| Reduced motion | immediate target, no custom override |

No perpetual shimmer, entrance animation or decorative motion. Scrims are
black gradients. Backdrop blur is not required.

## Layout policy

`layoutFor(width, height)` uses both dimensions. A wide window with a short
height falls back to medium.

| Window | Padding | Hero title | Rail cards | Discover columns | Tile height | Settings rail | Hints |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1470x956 | 64 | 76 | 6 | 5 | 206 | 286 | 4 |
| 1280x720 | 48 | 60 | 5 | 4 | 214 | 260 | 4 |
| 1024x640 | 28 | 48 | 4 | 4 | 186 | 220 | 3 |

Compact Discover hides the category column and keeps the filter chips.
The #35 Home page keeps both rails at compact size and scrolls; this is an
approved deviation from frame `R7ZZ2`, which shows Favorites only. Remaining
library content stays reachable through the page. Long names use two lines with
an ellipsis in a fixed tile, and one line in a row.

## Artwork

- Channel art is a decoded logo plus a bounded dominant color.
- `internal/logos.DominantColor` samples at most 32x32 points, ignores
  transparent pixels, rejects near-gray results and darkens the tint until
  white text passes 4.5:1.
- Decoding and color extraction happen off the UI thread. `ui.NewBitmap`
  happens on the UI thread.
- The logo cache is bounded and keeps the color beside the bitmap.
- A missing, loading or rejected logo shows the monogram in the same fixed
  box. Layout never changes when the image arrives.

## Frame index

Design screenshots are intentionally not committed; export them from the
Pencil document when a reviewer needs them. The frame IDs below are the
portable reference.

| Frame | Screen | Scope |
| --- | --- | --- |
| `m79WoJ` | Discover — no country selected | #36 |
| `C1I6bE` | Home feed — wide | #35 |
| `R7ZZ2` | Home — compact 1024x640 | #35 |
| `NYvSY` | Discover — wide | #36 |
| `xztVf` | Discover — compact 1024x640 | #36 |
| `QTxsy` | Control Center over player | #34 |
| `S9lsLR` | Watch player | #38 |
| `MItGy` | Watch channel drawer | #38 |
| `NGQIO` | Watch paused | #37/#38 |
| `H371ba` | Stream unavailable | #37/#38 |
| `wBRZA` | Buffering | #37/#38 |
| `At5Co` | Settings | #40 |
| `NdqX5` | Settings — shortcuts | #40 |
| `Ndkhq` | Settings — About | #40 |
| `YpkzY` | PiP | #39 |
| `wgA0r` | PiP states | #39 |

Implementation layout checks are covered by `TestGalleryReferenceSizes` in
`internal/ui/gallery_test.go`. The test can write temporary PNGs when
`MUMU_CAPTURE_DIR` is set; those files are evidence only and are not committed.

## Deferred to v0.2.0

These Pencil frames are intentionally out of M5 scope:

- `DFNHY` — new playlist collection
- `M0qobV` — collection validation
- `pIUUr` — collection grid
- `F7vWm` — collection queue

Do not add collection storage, creation or queue behaviour under M5.

## Other approved decisions

- The permanent application sidebar is removed. Control Center is the primary
  navigation surface; five approved icons plus the top-bar Settings entry are
  implemented in #34.
- Fresh install opens Discover with **no country selected**. No built-in
  country playlist is fetched before an explicit choice. #36 implements it.
- A later launch with a valid recent channel opens Home, never autoplay.
- PiP **Close/Esc stops playback**. **Return/P** moves the same session to the
  main window and preserves pause. #39 implements it.
- Watch and PiP remain one logical playback session and one libmpv surface.
- The live status line is non-interactive. No playhead, elapsed time, seek or
  timeshift is promised.
- No mock channel counts, logos, versions, resolution or update timestamps
  ship as real data.

## Verification

Run from the repository root:

```sh
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
go vet ./...
go tool mygo vet .
gofmt -l ./cmd ./internal
golangci-lint run ./cmd/... ./internal/...
```

Native checks still require macOS: font rendering, Light/System appearance,
Reduce Motion, hover versus Tab focus, logo loading and long names at all
three reference sizes.
