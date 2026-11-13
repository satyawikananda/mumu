# Settings, shortcuts and About

Native MyGo implementation notes for #40. The visual contract is
[`DESIGN.md`](../../DESIGN.md); frame IDs and tokens are in the
[M5 handoff](revamp/README.md).

## Layout

Settings is a secondary destination, reached from the top-bar gear or `⌘,`.
The page owns a screen-local rail, not a permanent shell sidebar.

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ shell top bar: brand · SETTINGS · Control Center · Settings              │
├───────────────────────┬──────────────────────────────────────────────────┤
│ SETTINGS              │ panel title + explanation                        │
│ Appearance            │                                                  │
│ Shortcuts             │ scrollable panel content                          │
│ About                 │                                                  │
│                       │                                                  │
│ mumu <version>        │                                                  │
├───────────────────────┴──────────────────────────────────────────────────┤
│ shared shortcut strip                                                    │
└──────────────────────────────────────────────────────────────────────────┘
```

- The shell top bar stays authoritative. Settings does not draw a second back
  button, wordmark or clock.
- The rail keeps its position; only the panel scrolls. Its width comes from
  `layoutFor(width, height).SettingsRail`, capped so the panel stays usable at
  the minimum window size.
- The panel is laid out from the real window size: theme cards stack when the
  panel is narrow, and the shortcut reference uses one or two columns.
- `Settings.Scrolled()` feeds the shell's scrim state. There is no second scroll
  state in `Shell`.
- The rail footer shows the actual build version. It does not show a mock
  milestone such as `0.1.0 · M2`.
- Escape uses the existing Browse back behavior.

## Appearance

The Appearance panel has two groups: Theme and Playback. Default quality,
Sources and their import actions are not part of this build.

### Theme

Dark, Light and System are persisted with `appearance.theme`. The stored values
are `dark`, `light` and `system`; a missing or unknown value falls back to
Dark.

- The preference is read before the main window opens, so the first frame uses
  the stored appearance.
- A click applies immediately through `Shell.SetThemeMode`.
- A failed write restores the previous appearance and shows an inline error.
- System follows the desktop appearance through MyGo's theme context.

### Start muted

`playback.startMuted` defaults to off. It applies after a new channel load
succeeds. It deliberately does not:

- change the current channel when toggled;
- run on a retry of the same channel;
- get written by the runtime `M` command;
- get written by PiP's temporary mute.

A launch still starts audible. The preference is applied by the first explicit
Play, never by autoplay.

## Shortcuts

The reference is rendered from `shortcutRegistry` through `shortcutGroups()`.
There is no second command list and no remapping UI. The designed panel has
three surface cards with an icon, an uppercase title and one divider between
rows:

- Player
- Browse
- Control Center

At wide panel widths Player sits in the left column and Browse stacks above
Control Center on the right. Narrow panels stack all three. Labels and key text
come from the same registry entries that handlers and hint bars use. Home,
retry and the Settings command stay implemented but are not part of this panel.
Commands that are not `Available` are omitted.

## About

About uses real build and local data only.

| Row | Source |
| --- | --- |
| Version and commit | packaged app version, `runtime/debug` VCS settings |
| Channel catalog | enabled cached sources and their real `ItemCount` |
| License | project MIT license |
| Repository | verified `github.com/satyawikananda/mumu` URL |
| Credits | iptv-org, Lucide, Lexend/Inter Tight and the MyGo build dependency |
| Runtime | Go version, OS and architecture |
| Display | current logical window size and text scale |

The public MyGo API does not expose a display backing scale factor.
The Display row reports logical DIPs and text scale; it never invents a `2.0x`
value.

### Copy system info

`Copy system info` writes a redacted report through
`ui.Context.Services().WriteClipboard`. It contains:

- mumu version and commit;
- Go/MyGo runtime and platform;
- display size and text scale;
- real catalog counts.

It does not contain stream URLs, credentials, headers, favorites, watch history,
database paths, log paths or other unrelated local data.

### Manual update check

The GitHub release check runs only after the viewer clicks `Check for updates`.
Construction, launch and page rendering never call the checker.

- Feed: fixed GitHub Releases metadata for `satyawikananda/mumu`.
- Stable builds ignore prereleases.
- Prerelease builds accept stable releases and newer prereleases in the same
  channel (the first prerelease identifier).
- Drafts are ignored.
- States: idle, checking, up to date, available, no release, and explicit
  access/network/rate-limit errors.
- `Up to date` has no invented "checked 2 minutes ago" timestamp.
- An available release shows its real GitHub release page. mumu only opens the
  page; it never downloads, installs or replaces itself.

Repository has no published release at the time of this change, so the live
result is the explicit no-release state; new-release behavior is covered by
injected checker tests.

## Accessibility and state

- Rail entries are native buttons with labels and the shared focus ring.
- Theme cards are native buttons; selection shows a check mark, not only color.
- Switches have field labels and descriptions linked through `LabelledBy` and
  `DescribedBy`; clicking the row label toggles the switch.
- Errors appear inline next to the affected control.
- The panel is keyboard reachable; the shared hint strip still reports Escape.
- The page has loading and unavailable states for real catalog counts.

## Verification

Focused tests:

```sh
go test ./internal/ui ./internal/app ./internal/update ./internal/netguard
```

Repository checks:

```sh
gofmt -l ./cmd ./internal
go vet ./...
go tool mygo vet .
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
golangci-lint run ./cmd/... ./internal/...
```

Native macOS checks still required for real font rendering, Light/System
appearance, keyboard focus, clipboard and release-page opening.
