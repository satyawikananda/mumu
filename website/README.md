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
- `src/components/landing/Landing.astro` composes the hero, screenshot gallery, PiP demonstration, benefits, download section, and footer. Header, platform actions, gallery, and PiP are separate components because the starter had no equivalents.
- `src/layouts/Layout.astro` owns metadata, local font preloads, and an inline theme initializer. Set Astro's `site` once the production domain is known to emit canonical and alternate-language links.
- `src/styles/global.css` owns semantic light/dark tokens, typography, responsive layouts, and interaction states. Lexend and Inter Tight are copied from the app's bundled faces; their OFL licenses ship in `public/licenses`.
- Product images are original screenshots from `docs/screenshots` and the app's existing mascot. Astro emits responsive WebP sizes. Screenshots retain their original English app content in every locale.

The page follows the system theme, with persistent light/dark/system overrides when browser storage is available. Language links preserve the current section hash; the URL determines the locale.

The desktop header follows the portfolio reference: full-width and transparent at the top, then a centered 760-pixel capsule after 96 pixels of scrolling. Below 768 pixels, the MyGo Music layout uses a 60-pixel sticky bar with the mascot, icon-only language disclosure, and three theme radios in one row. It gains a blurred background when scrolling; there is no hamburger or mobile drawer. Download remains available in the hero and download section. `HeaderPreferences.astro` supplies the same controls across breakpoints, retaining Mumu's fonts, mascot, and semantic colors. The latest mobile canvas variant is **Mumu Navbar Mobile MyGo**.

## Interaction and motion

`src/scripts/interactions.ts` handles header scroll state, theme controls, accessible tabs, native screenshot dialog, favorite toggle, and PiP simulation. Tabs support arrows/Home/End. The dialog supports Escape and restores focus. PiP supports dragging within its stage, four corner buttons, arrow keys, and return to the full window. Favorite and PiP interactions are explicitly labelled demonstrations and do not alter the desktop app.

The language disclosure supports arrows/Home/End, Escape, and outside-click dismissal. Theme radios support native keyboard selection. Reduced motion disables the navbar transitions; reduced transparency uses solid surfaces.

`src/scripts/motion.ts` synchronizes Lenis with GSAP through one ticker. The hero screenshot settles in perspective, the gallery settles into place, and desktop scrolling demonstrates the PiP transition once. Manual PiP controls take priority. Smaller screens retain short reveals without perspective effects. Reduced motion disables these effects and uses native scrolling. Motion cleanup handles media changes, HMR, and browser back/forward restoration.

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
