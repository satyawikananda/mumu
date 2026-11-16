# Landing page verification

Checked on 2026-11-16 in the Mumu `viper` worktree. Scope: `website` and the two Wonder artboards. Native desktop code was not changed.

This records the initial implementation. The subsequent [navbar verification](navbar-verification.md) covers the final desktop header, inline mobile controls, and removal of the earlier mobile menu.

## Design direction

The desktop and mobile light specifications are in [Mumu Website](https://app.wonder.so/satya-wikananda/files/01a1e49c-ffc9-79f5-b8b9-05f3539ba748). The implementation adds the requested system/light/dark behavior and interactive feature demonstrations.

The direction is spacious, friendly, and editorial: large Lexend headlines, Inter Tight copy, neutral surfaces, sparse Mumu red, original screenshots, and the existing tomato mascot. ENERGY 2, RHYTHM 3, MOTION 3: quiet reading areas alternate with large product imagery and a PiP demonstration. The three reference sites inform type, whitespace, and hands-on feature explanations rather than copied layouts.

Rounded download controls distinguish actions from rectangular screenshots. Shadows indicate app windows and the modal, rather than elevating every section. Lucide icons identify download, menu, theme, screenshot expansion, PiP positions, favorite, and scroll directions. Motion demonstrates window presentation and section progression. The PiP desktop illustration is source-authored CSS and explicitly labelled as a screenshot simulation.

## Automated checks

- PASS: `bun run lint`, including Astro, TypeScript, and the Node test import exception.
- PASS: `bun run test`: 2 tests, no failures. All four locale dictionaries include content and control labels; unavailable or non-DMG download URLs keep downloading disabled.
- PASS: `bun run check`: 19 files, zero errors, warnings, or hints.
- PASS: `bun run build`: four prerendered routes and 28 optimized image variants. The adapter selected inspector port 9230 because 9229 was occupied; the build completed.
- PASS: `git diff --check`.

## Browser checks

The production build's `dist/client` directory was served temporarily at `127.0.0.1:4322` with Python's static HTTP server and inspected in Brave through browser automation. This did not start the Wonder Preview dev server. The test server was stopped after verification.

| Item | Observed result |
| --- | --- |
| Locale routes | `/`, `/id/`, `/zh-cn/`, `/ja/` render the matching heading, title, and document language. |
| Responsive layout | All four routes at 320, 390, 768, and 1440 pixels have no horizontal document overflow; no broken loaded images were observed. |
| Theme | Light/dark controls update the page; light persists after reload. System follows emulated light/dark changes after the media event fires. Theme persists across language navigation. |
| Mobile menu | Opens, exposes navigation, and closes with Escape or language navigation. |
| Language picker | Native disclosure opens; selecting Japanese from Indonesian navigates to `/ja/` and updates the document language. |
| Gallery tabs | Clicking selects the corresponding screenshot. Arrow keys, Home, and End update selection and keyboard focus. |
| Screenshot dialog | Expands the selected screenshot. Escape and the close button dismiss it; focus returns to the invoking button. The dialog opts out of Lenis scrolling. |
| PiP scroll demonstration | At desktop width, reaching the section floats the screenshot once. Manual controls take priority afterward. |
| PiP controls | Enter/return updates the presentation and pressed state. All four corner buttons work; arrow keys select corners. Dragging at desktop and mobile widths stays inside the stage. |
| Favorite | Add/remove toggles the accessible name, pressed state, heart, and status text. The demo starts empty. |
| Download | Hero CTA reaches the download section. The macOS release control and four Windows/Linux controls remain disabled. `noindex, nofollow` is present while the DMG URL is empty. |
| Anchors | Every internal anchor has a target. Back to top returns to the beginning after moving the target off the sticky header. |
| Reduced motion | No Lenis class or hero transform; scrolling does not automatically float PiP. Controls remain available. |
| JavaScript disabled | Navigation remains visible, all three screenshot panels render, enhancement-only controls disappear, and the 390-pixel layout has no overflow. A CSS layer-priority issue found during this check was fixed and retested. |
| Console | Normal browser navigation and interaction checks reported zero console errors and zero page errors. Storage fault-injection attempts produced blank automation page errors; initial storage denial is not claimed as verified. The actual reads/writes are guarded in source. |
| External links | GitHub, releases, issues, license, and installation URLs are real repository destinations; the installation fragment matches `docs/release/macos.md`. External destinations were inspected rather than all clicked during the local QA session. |

Visual inspection covered the desktop hero in light/dark, Indonesian mobile with navigation open, Chinese mobile in dark, and the desktop PiP section. These checks establish browser behavior; they do not replace the owner's design acceptance or native-speaker copy review.

## Anti-slop delivery evidence

| Gate item | Result and evidence |
| --- | --- |
| R-02, R-15, R-16 | PASS: product copy contains no em dashes or generic marketing superlatives; the primary CTA names macOS downloading. |
| R-03, R-34 | PASS: four locales at four widths have no horizontal overflow; light/dark and CJK mobile views were inspected. |
| R-17, R-18, R-36, R-38 | PASS: no invented metrics, testimonials, identities, or performance claims; features come from repository docs and screenshots. |
| R-23, R-37 | PASS: user-authorized landing sections and references drive the composition; existing mascot/screenshots are reused; the simulation is labelled. |
| R-24, R-26, R-35 | PASS: internal targets exist and the browser control results are recorded above; unavailable platforms and DMG are visibly disabled. |
| R-25 | PASS: computed contrast is 17.72:1 light body, 6.20:1 light secondary on surface, 5.72:1 light CTA, 18.17:1 dark body, 6.83:1 dark secondary on raised surface, 4.63:1 dark CTA, and 7.63:1 dark action text. |
| R-27, R-28 | PASS: download unavailability is explained, favorite starts empty, screenshots are prerendered, and the optional recording includes an error message. No filler FAQ is present. |
| R-32, R-39 | PASS: native buttons/select/details/dialog, labelled controls, visible keyboard rings, tab/dialog/PiP keyboard behavior, document landmarks/language, and reduced-motion behavior are implemented. |
| R-40 | PASS: media has dimensions, hero is eager/high-priority, fonts are local, images have responsive variants, and no performance score is claimed. |
| Purpose and identity | PASS: the typography, sparse red, varying section composition, real app imagery, mascot, and functional motion have the purposes described above. |
| R-33 process note | Mechanical cleanup earlier used a source-edit script. Final feature logic and fixes were edited directly in source and reviewed; strict retrospective compliance with this process rule is not claimed. No rewrite script ships in the website. |

## Release inputs still required

- Public macOS Apple Silicon DMG URL and version in `src/data/site.ts`.
- Approved MP4 recording; its optional playback/error branch is source-checked but has not been exercised with the real recording.
- Production domain in Astro's `site` configuration.
- Owner visual/motion acceptance and native-speaker Chinese/Japanese review.

Cloudflare publication, the real DMG download/install flow, Safari/Firefox, touch hardware, screen-reader output, and Core Web Vitals were not tested. No deployment, commit, or push was performed at this initial verification stage.
