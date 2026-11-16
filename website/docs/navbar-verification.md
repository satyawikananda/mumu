# Navbar revision verification

Checked on 2026-11-17 in `website`, updated for the subsequent MyGo Music mobile request. Scope: navbar and shared preference controls. The rest of the landing page and native desktop app were preserved.

## Design and implementation

References inspected in the live browser: [Satya's portfolio](https://hi-satya.my.id/) and [MyGo Music](https://music.egoist.dev/). The desktop header retains the portfolio's transparent expanded state, 96-pixel scroll threshold, centered 760-pixel capsule, and 16-pixel top offset. The latest mobile request replaces the drawer with MyGo Music's 60-pixel sticky bar, 16-pixel side padding, 28-pixel mascot, 15-pixel brand label, 14-pixel control gap, and inline language/theme controls. Mobile scrolling adds a translucent background and blur instead of a capsule. Download remains in the hero and download section; its desktop navbar link stays available. Features and GitHub remain removed from the header.

The current canvas specifications are **Mumu Navbar Desktop Revised** and **Mumu Navbar Mobile MyGo** in [Mumu Website](https://app.wonder.so/satya-wikananda/files/01a1e49c-ffc9-79f5-b8b9-05f3539ba748). The original full-page artboards and former mobile drawer variant remain as prior versions. The new mobile variant shows the top bar, scrolled state, and language popup; static neutral surfaces illustrate the runtime blur.

Existing `Header.astro`, `HeaderPreferences.astro`, interaction handlers, and semantic styles were reused. One preference group now serves all viewports. The removed drawer's markup, CSS, handlers, close labels, and icon import were removed. No new component or dependency was needed. Reference dimensions remain local CSS values because this repository has no corresponding spacing or header-size tokens; colors and fonts use existing semantic tokens.

Direction: ENERGY 2 / RHYTHM 3 / MOTION 3, retaining the landing page's previously agreed direction. Navbar movement communicates its compact state. Blur separates navigation from scrolling content; the popup uses a small shadow to identify its layer. The download capsule remains the primary navigation action. Neutral preference controls keep emphasis on the Mumu mascot and download action. No new imagery or marketing claims were added.

## Checks performed

- PASS: `bun run lint` and `bun run test` (2 tests, zero failures).
- PASS: `bun run check` (20 files, zero errors, warnings, or hints).
- PASS: `bun run build` (4 prerendered routes, 28 optimized image variants).
- PASS: `git diff --check`.
- Initial sandbox build failed on Wrangler log access and its internal port. Check/build were rerun with the required permission and exited successfully.

Browser checks used the production `dist/client` output on a temporary static server at `localhost:4322`, independently of the running Wonder Preview. Brave's 90% origin zoom was accounted for by verifying `innerWidth` rather than assuming the requested device width.

- All four locale routes at actual CSS widths 320, 390, 768, and 1440 pixels: no horizontal document overflow after fonts/layout settle. Inline language and theme controls remain visible. An immediate read during resize/font loading produced transient overflow; the check was repeated after font readiness and two rendering frames. Mobile has no hamburger or drawer. Desktop retains its Download link.
- Desktop header is expanded before the scroll threshold, compacts after it, and expands again at the top. Its compact state measures approximately 760 pixels wide with a 16-pixel top offset; subpixel borders vary with browser zoom.
- Dark selection persists after reload. Native ArrowRight changes Dark to System. System responds to emulated light/dark changes. Theme persists across locale navigation.
- Language disclosure opens; End focuses Japanese; Escape closes it and restores summary focus. Clicking outside closes it. Every locale link was clicked and resolved to the matching document language. Mobile language navigation retains `#download`.
- At 320 pixels, the mobile bar measures 320 by 60 pixels at the top and after scrolling; its corners remain square. The 176-pixel language popup stays within the viewport. ArrowUp opens the popup and focuses Japanese; Escape closes it and returns focus to the disclosure. Desktop still compacts to approximately 760 pixels at a 16-pixel top offset.
- Reduced motion sets the header transition duration to 0.01 milliseconds. Emulated reduced transparency removes the mobile backdrop filter. With script execution disabled, the inline mobile language control remains visible at 390 pixels, its native disclosure works, theme controls disappear, and no overflow occurs. The header remains 60 pixels tall.
- Browser automation reported zero console errors and zero page errors during these checks. The current mobile layout was visually compared with MyGo Music and its revised canvas specification. The earlier desktop visual review remains applicable because that layout was preserved.

These checks do not claim pixel identity across operating systems, font rasterizers, or zoom levels. The reference structure and behavior are preserved; Mumu branding is intentional. Production release inputs listed in the original verification remain outstanding.

## Antislop delivery gate

This gate covers the navbar revision only. The owner's explicit request to reproduce the reference navbar takes precedence over R-30's general anti-clone preference.

### Hard gate

- PASS R-02: no em dash was introduced in navbar labels or new documentation.
- PASS R-03: 16 locale/width combinations have no horizontal document overflow.
- PASS R-17: no statistics were added.
- PASS R-18: no testimonials were added.
- PASS R-23: existing mascot and Lucide icons were reused; inline mobile navigation was explicitly requested.
- PASS R-24: Download targets the existing section; every locale link was clicked successfully.
- PASS R-25: source-token contrast calculations are 17.72:1 for light primary text and 18.17:1 for dark primary text. Muted control colors remain above 5.8:1 on the neutral control surfaces checked. Selected language remains text-colored, with red limited to its check icon.
- PASS R-26: language links, theme radios, and desktop/hero Download links have verified behavior; removed Menu and Close controls leave no dead handlers.
- PASS R-27: navigation introduces no remote data, form, or loading/error lifecycle; storage access remains guarded.
- PASS R-28: no FAQ was added.
- PASS R-32: radio arrows, disclosure navigation, and Escape focus restoration were checked.
- PASS R-33: source changes were authored with patches; ESLint performed formatting only.
- PASS R-34: light/dark/system changes and persistence across reload/locale changes were verified.
- PASS R-35: build and changed-control browser checks completed successfully.
- PASS R-36: no security, performance, or customer claims were added.
- PASS R-37: direction follows the explicit portfolio and MyGo Music references plus the existing Mumu design.
- PASS R-38: no product features, people, or destinations were invented.
- PASS R-39: controls use native semantics, localized accessible names, pointer states, keyboard focus, and reduced-motion styles.
- PASS R-40: mascot dimensions remain authored; no new media, script dependency, or performance claim was introduced.

### Purpose gate

- PASS R-01: no gradient or glow was added.
- PASS R-04: globe, chevron, check, sun, moon, monitor, and close icons identify their actual controls.
- PASS R-06: existing Lexend/Inter Tight typography preserves Mumu identity.
- PASS R-07: no background grid was added.
- PASS R-08: no decorative arrows were added.
- PASS R-09: no badges were added.
- PASS R-10: blur is limited to the scrolled navbar and its language disclosure.
- PASS R-12: small popup/selected-segment shadows indicate control layering, not section elevation.
- PASS R-13: no glow was added.
- PASS R-14: no feature cards were added or changed.
- PASS R-19: navbar background and disclosure transitions communicate navigation changes; reduced motion was checked.
- PASS R-22: no illustration was added.

### Liveliness

- PASS: explicit ENERGY 2 / RHYTHM 3 / MOTION 3 direction is retained.
- PASS: restrained navbar movement supports the existing page rhythm without changing sections.
- PASS: Download is the clear action; the mascot anchors the opposite side.
- PASS: expanded spacing and compact spacing distinguish reading from scrolling.
- PASS: the existing Mumu red remains the deliberate identity accent.
- PASS: the mascot, wordmark, and rounded controls retain the repeated identity motif.
- PASS: reference direction was settled before the revised canvas variants and code.

### Craftsmanship and quality locks

- PASS C-1: visual choices follow verified references and existing brand tokens.
- PASS C-2: changed controls have verified behavior.
- PASS C-3: no sections were added.
- PASS C-4: responsive, theme, keyboard, and script-disabled checks passed.
- PASS C-5: no testimonials, statistics, or marketing claims were fabricated.
- PASS R-05: existing landing composition was preserved.
- PASS R-11: capsule navigation, small rounded controls, and popup radii serve distinct roles.
- PASS R-15: Download names the destination; no generic CTA was added.
- PASS R-16: no marketing buzzwords were added.
- PASS R-20: Mumu's real mascot, wordmark, and fonts remain.
- PASS R-21: System, Light, and Dark all work; no fixed theme was imposed.
- PASS R-29: existing semantic neutral palette and red accent were reused.
- PASS R-30: reference matching was explicitly requested; retained Mumu branding documents the intended difference.
- PASS R-31: layout, spacing, blur, and control choices have written reasons above.
