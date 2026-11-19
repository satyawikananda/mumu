# Mumu website

Astro landing page for the native Mumu TV desktop app. The visual specification lives in [Mumu Website on Wonder](https://app.wonder.so/satya-wikananda/files/01a1e49c-ffc9-79f5-b8b9-05f3539ba748), with desktop and mobile artboards.

## Development

Requires Node.js >=22.23.4 and Bun. From `website`:

```sh
bun install --frozen-lockfile
bun run dev --background
```

Open `http://localhost:4321`. No backend, account, or local credentials are needed. Manage the background server with `bun run astro dev status`, `bun run astro dev logs`, and `bun run astro dev stop`.

```sh
bun run test
bun run lint
bun run check
bun run build
```

## Content and components

- `/`: English, `/id/`: Indonesian, `/zh-cn/`: Simplified Chinese, `/ja/`: Japanese. Dictionaries include metadata, visible content, image descriptions, and accessible control labels.
- `src/components/landing/Landing.astro` composes the hero, screenshot gallery, PiP demonstration, benefits, download section, and footer. Header, platform actions, gallery, PiP, and footer are separate components because the starter had no equivalents.
- `src/layouts/Layout.astro` owns metadata, local font preloads, and an inline theme initializer. Set Astro's `site` once the production domain is known to emit canonical and alternate-language links.
- Landing components use Tailwind CSS 4 utilities for layout, responsive sizing, colors, and interaction states. `src/styles/global.css` contains Tailwind theme tokens, variants, bundled font declarations, and shared base/accessibility rules. It has no component class selectors or `@apply` component wrappers. Lexend and Inter Tight are copied from the app's bundled faces; their OFL licenses ship in `public/licenses`.
- Product screenshots come from `docs/screenshots`. Astro emits responsive WebP sizes. Screenshots retain their original English app content in every locale.
- `Mascot.astro` renders the header, benefits, and download mascots with `assets/lottie/mumu-happy-dance.json`, using `astro-integration-lottie` and the light SVG player from `lottie-web`. The animation URL is emitted by Vite; these three instances share one downloaded JSON file. Square containers reserve space, with 48/36-pixel desktop/mobile header marks and larger responsive benefits/download mascots. Lottie exports stay outside ESLint formatting.

The page follows the system theme, with persistent light/dark/system overrides when browser storage is available. Language links preserve the current section hash; the URL determines the locale.

The desktop header follows the portfolio reference: full-width and transparent at the top, then a centered 760-pixel capsule after 96 pixels of scrolling. Below 768 pixels, the MyGo Music layout uses a 60-pixel sticky bar with the mascot, icon-only language disclosure, and three theme radios in one row. It gains a blurred background when scrolling; there is no hamburger or mobile drawer. Download remains available in the hero and download section. `HeaderPreferences.astro` supplies the same controls across breakpoints, retaining Mumu's fonts, mascot, and semantic colors. The latest mobile canvas variant is **Mumu Navbar Mobile MyGo**.

## Footer

`Footer.astro` receives the localized `Copy` dictionary and preserves the repository, releases, issues, license, and Back to top links. The Doodle Crew reference informs the oversized “Mumu TV” wordmark and character arrangement. The footer uses the locally bundled Finger Paint Regular font with its OFL license in `public/licenses`; Chinese and Japanese characters use the existing system font fallbacks. The rest of the site retains Lexend and Inter Tight. Tomato artwork and light/dark theme tokens remain shared. Three tomatoes and six TV/play/star doodles become two tomatoes and three doodles below 761 pixels. The footer uses optimized static images, including without JavaScript, rather than adding Lottie loops.

`src/scripts/footer.ts` runs inside the existing GSAP media context. The entrance plays once per page, followed only by hover feedback and one jump per mascot activation. Native buttons support click, tap, Enter, and Space with localized labels. Repeated activation during a jump is ignored. Reduced motion disables the entrance and removes the decorative interaction buttons from focus order. The character artwork and its button use separate stacking layers, so letters cannot block the peeking mascot’s click target. Page cleanup aborts listeners and reverts the GSAP context; restoring a revealed page does not replay its entrance.

## Interaction and motion

`src/scripts/interactions.ts` handles header scroll state, theme controls, accessible tabs, native screenshot dialog, favorite toggle, and PiP simulation. Tabs support arrows/Home/End. The dialog supports Escape and restores focus. PiP supports dragging within its stage, four corner buttons, arrow keys, and return to the full window. Favorite and PiP interactions are explicitly labelled demonstrations and do not alter the desktop app.

Scripts select `data-*` hooks and IDs rather than styling classes. Header compact mode, PiP floating mode, and dragging use data attributes; Tailwind attribute/group variants render those states. ARIA attributes drive tab, favorite, and corner-button styling. Component sizing and colors use utilities such as `py-section`, `bg-canvas`, and `text-secondary` generated from the shared theme.

The language disclosure supports arrows/Home/End, Escape, and outside-click dismissal. Theme radios support native keyboard selection. Reduced motion disables the navbar transitions; reduced transparency uses solid surfaces.

`src/scripts/motion.ts` synchronizes Lenis with GSAP through one ticker. The hero screenshot settles in perspective, the gallery settles into place, and desktop scrolling demonstrates the PiP transition once. Manual PiP controls take priority. Smaller screens retain short reveals without perspective effects. Reduced motion disables these effects and uses native scrolling. Motion cleanup handles media changes, HMR, and browser back/forward restoration.

Each Lottie mascot loops automatically when visible, at 1.25× speed (4.8 seconds per cycle). Offscreen mascots and hidden tabs pause playback. Reduced motion shows the first frame and responds to preference changes. The download mascot uses only Lottie motion, without the former GSAP rotation.

`Cloudscape.astro` and `src/scripts/cloudscape.ts` adapt the supplied 21st.dev cloud shader to a decorative WebGL hero background without React. Semantic CSS tokens provide sky-blue clouds in light mode and subdued charcoal-blue clouds in dark mode. The full-width background fades into the page, with a canvas-colored overlay to preserve text contrast. The renderer caps drawing at 30 fps, DPR 1.5, and two million pixels; it pauses offscreen and in hidden tabs. Reduced motion keeps a static frame. A CSS gradient remains available without JavaScript, when WebGL initialization fails, or during context loss. Observers and GPU resources are released on navigation, page hide, and HMR; browser history restoration initializes one renderer again.

`Faq.astro` renders the localized FAQ below the download section. Each answer is a native `<details>`/`<summary>` disclosure, so it works without JavaScript, exposes its expanded state, and keeps keyboard focus visible. On wide screens the section uses an asymmetric heading rail beside the full disclosure column; narrow layouts collapse to one column and the heading stops sticking. Shared tokens, one top border plus row dividers, and a text link to GitHub Issues for unanswered questions complete the block. All four dictionaries provide the same eight questions; the content test enforces parity.

Without JavaScript, all gallery screenshots and navigation remain visible; enhancement-only controls are hidden. Images reserve their space, and the Home hero image loads eagerly with high priority.

## Release and recording

`src/data/site.ts` intentionally starts with an empty `release.macDownloadUrl`, `release.version`, and `demoVideo.src`.

Before the public launch:

1. Publish the real macOS Apple Silicon DMG and set its direct HTTPS `.dmg` URL and release version. The download button stays disabled until a qualifying URL is configured; prerelease pages emit `noindex, nofollow`.
2. Supply the approved MP4 recording through `demoVideo.src`. The optional native video player then appears with controls, a screenshot poster, and a load-error message. Until then, the screenshot simulation is used.
3. Set Astro's production `site`, verify the DMG link and install guide, and review translations with native speakers.

Windows and Linux buttons remain disabled and labelled coming soon. Copy avoids unmeasured performance claims or promises about third-party stream availability.

## Cloudflare

Astro 7, Tailwind 4, Lucide SVG icons, GSAP, and Lenis are already installed. `@astrojs/cloudflare` prerenders the four pages with compile-time image optimization; sessions are disabled. Wrangler targets the `mumu-website` Worker.

```sh
bun run cf:types
bun run deploy:check
```

`deploy:check` packages a dry run; it does not publish. Deployment is a separate maintainer action. For Workers Builds, use `website` as the root, `bun install --frozen-lockfile` to install, `bun run build` to build, and `bunx wrangler deploy` to deploy. No domain, account ID, or credentials are committed here.

See [verification notes](docs/verification.md) for checks actually performed and remaining release inputs.
The subsequent [navbar verification](docs/navbar-verification.md) covers the portfolio-style header revision.

### Hero cloud verification (2026-11-17)

`bun run test` (2 tests), `bun run lint`, `bun run check`, and `bun run build` passed. Local Chromium headless checks covered shader animation, theme changes without creating another context, reduced motion, offscreen pause/resume, context loss/restoration, 375/768/1440-pixel layouts, the drawing-buffer budget, all four locales, history navigation, and WebGL-disabled/no-JavaScript fallbacks. Light/dark desktop and light mobile screenshots were inspected. The cloud visibility revision uses a 35% canvas overlay, a tighter noise pattern, and brighter charcoal-blue clouds. The palette bounds give minimum secondary-text contrast of 4.65:1 in light mode and 5.40:1 in dark mode. Safari, Firefox, and physical-device GPU performance remain unverified.

### Lottie mascot verification (2026-11-17)

`bun run test` (2 tests), `bun run lint`, `bun run check` (0 diagnostics), and `bun run build` passed. Chromium headless checks verified four SVG mascots in every locale, one shared JSON request, visible playback and offscreen pause, one-shot completion, initial and changed reduced-motion preferences, simulated document visibility changes, history navigation, square sizing, and no horizontal overflow at 375/768/1440 pixels. Desktop, mobile, download, and dark benefits screenshots were inspected; no browser errors were observed. The build reports an upstream direct-`eval` warning in the integration's full-player dependency; mascots select the light player. Safari and Firefox remain unverified. Without JavaScript, decorative mascot containers remain empty.

The subsequent autoloop revision enables looping for all four mascots. `bun run lint` and `bun run check` passed. Chromium observed a `loopComplete` event and continued playback after a full cycle, with offscreen pause and reduced-motion behavior preserved.

### Tailwind migration verification (2026-11-17)

Layout and component states now use Tailwind utilities; all scoped component style blocks and component class selectors were removed. `bun run test` (2 tests), `bun run lint`, `bun run check` (0 diagnostics), and `bun run build` passed. Before/after Chromium measurements matched within one pixel for the sampled elements across seven combinations of 375/768/1024/1440-pixel widths, all four locales, and light/dark themes. Desktop and mobile screenshots were inspected. Runtime checks covered theme persistence, language keyboard controls, gallery tabs, screenshot dialog and restored focus, favorite styling, PiP dragging/corners/return, compact/mobile headers, skip-link focus, no-JavaScript gallery fallback, and mascot looping/reduced motion. Safari and Firefox remain unverified.

Additional Chromium checks verified the WebGL cloud renderer, GSAP hero/gallery animations, automatic PiP presentation, and an opaque header with reduced transparency enabled. No browser errors were observed.


### Footer verification (2026-11-18)

`bun run lint`, `bun run check` (27 files, zero diagnostics), `bun run test` (2 tests), `bun run build` (four pages), and `git diff --check` passed. The build required a permitted rerun because the sandbox blocks Cloudflare's local inspector listener and Wrangler log writes. The existing upstream `lottie-web` direct-`eval` build warning remains.

Local Chromium checks covered all 32 combinations of 320/390/768/1440-pixel widths, four locales, and light/dark themes: four repository links, the responsive mascot/doodle counts, reduced-motion controls, visible image loading, and no horizontal overflow. Additional checks verified click and Enter for all three mascots, Space, rapid repeated activation, touch activation at 390 pixels, preference changes in both directions, Back to top, and simulated pagehide/persisted-pageshow restoration without replaying the entrance. The right mascot's button now sits above the lettering while its artwork stays behind it. Desktop and mobile light/dark screenshots were inspected; the top mascot was repositioned to sit against the wordmark.

A 320-pixel Japanese page with JavaScript disabled retained the visible wordmark, images, links, and no horizontal overflow. A 200% CSS zoom check at 1440 pixels also had no horizontal overflow; native browser zoom was not tested. The local QA scripts and screenshots are in `/private/tmp/mumu-footer-check.mjs`, `/private/tmp/mumu-footer-extra.mjs`, and `/private/tmp/mumu-footer-*.png`. Safari, Firefox, physical-device touch, and production deployment remain unverified.


### Footer font revision (2026-11-18)

The footer brand and animated wordmark now read “Mumu TV”. Finger Paint Regular is bundled locally from the Google Fonts repository with its OFL license; it applies throughout the footer, with system fallbacks for Chinese and Japanese text. The wordmark uses a responsive 20cqw size, preserves the space before TV, and keeps the existing character interactions.

Lint, Astro check (zero diagnostics), both content tests, and the four-page build passed. Chromium verified the loaded Finger Paint face and wordmark bounds at 320/390/768/1440 pixels without horizontal overflow; final desktop and mobile screenshots were inspected. The footer regression checks also passed across four locales and both themes, including mascot click/Enter, reduced-motion preference changes, and Back to top. The existing Lottie build warning remains. No deployment was performed.

### FAQ verification

`bun run lint`, `bun run check` (28 files, zero diagnostics), `bun run test` (2 tests), and `bun run build` (four pages) passed. Chromium checks covered all four locales at 320 pixels, and the English page at 390, 760, 1024, and wide desktop widths in light and dark themes. The heading rail and disclosure column split at wide widths, collapsed to one column at 1024 and 760 pixels, and produced no FAQ overflow. Eight native `<details>` items rendered per page. Space opened and Enter closed the focused summary, and the focus ring stayed visible. Light-mode secondary text measured 6.63:1 against the canvas and dark-mode secondary text measured 8.67:1. The 320-pixel Indonesian page had a pre-existing 2-pixel overflow from a benefits mascot, unrelated to the FAQ. Safari, Firefox, and physical-device checks remain unverified.
