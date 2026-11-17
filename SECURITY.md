# Security Policy

Mumu TV is a native macOS desktop TV player. It runs entirely on the local
machine, has no account system, no hosted backend, and no telemetry.

## Supported versions

| Version | Supported |
| --- | --- |
| `0.1.x` (pre-release) | Yes |
| `main` branch | Yes; fixes land here first |
| Older pre-releases | No |

v0.1.0 is an early pre-release. Fixes ship on `main` and in the next patch
release.

## Reporting a vulnerability

Do not open a public issue for a security problem.

Use GitHub's private vulnerability reporting:

<https://github.com/satyawikananda/mumu/security/advisories/new>

If private reporting is unavailable, contact the maintainer through the
repository profile and ask for a private channel. Do not put exploit details in
a public issue or discussion.

Please include:

- the affected version or commit, and whether you used a published DMG or a
  source build;
- your macOS version and architecture (Apple Silicon is the supported release
  target);
- the trust boundary you crossed and the concrete impact;
- minimal reproduction steps or a proof of concept;
- relevant logs with stream URLs, credentials, and header values redacted.

Use dummy playlists, dummy accounts, and synthetic streams. Do not send real
credentials, private playlist URLs, or another person's data.

## What to expect

- We aim to acknowledge a report within 5 business days.
- We will triage and reproduce it, and tell you whether we consider it in scope.
- We fix on `main` and in the next release; timing depends on severity.
- We default to coordinated disclosure: we ask for time to ship a fix before
  public details, and we are happy to credit you if you want.
- This project has no bug bounty.

## Scope

In scope:

- the mumu source in this repository and the macOS arm64 build;
- playlist and stream URL parsing, including M3U input;
- the network policy in `internal/netguard` and the playback proxy, including
  redirects, DNS rebinding, and private/link-local destinations;
- the libmpv adapter, the retained native video surface, and their lifecycle;
- local SQLite storage, migrations, and log/error output;
- packaging, the bundled LGPL runtime, and release scripts.

Out of scope:

- third-party streams, providers, and their content or availability;
- upstream projects such as MyGo, libmpv, and FFmpeg: report those upstream.
  Tell us if mumu misuses them in a way that creates a mumu-specific issue;
- DRM, paywall, authentication, or geographic-restriction circumvention. We
  will not help bypass them, and a report that depends on doing so is out of
  scope;
- social engineering, physical access, or a machine already compromised with
  the user's own privileges;
- missing hardening with no demonstrated boundary violation, unless it defeats
  a control this repository explicitly claims to enforce.

## Security model and known limitations

- The app is local-first. It has no accounts, no cloud sync, and no telemetry.
  Playlists, favorites, history, and settings stay in SQLite on the Mac.
- Remote playlists and streams are untrusted. Only `http` and `https` are
  accepted, redirects and response sizes are bounded, and loopback, private,
  link-local, and local destinations are refused unless the user explicitly
  consents for one source. The libmpv playback network path applies the same
  policy through a loopback validating proxy.
- libmpv and the decoder run in the app process. v0.1.0 does not sandbox
  playback and is ad-hoc signed without notarization. It has no automatic
  update mechanism. These are documented release limitations, not by
  themselves vulnerabilities. See `AGENTS.md`, `docs/architecture/`, and
  `docs/release/macos.md`.
- The built-in catalog from iptv-org is a community discovery list. It is not a
  rights review, and a listed stream is not verified, licensed, or online.
  Report content or rights problems through the normal issue tracker, not the
  security channel.

## Safe harbor

We welcome good-faith research that follows this policy. We will not pursue
legal action for research that:

- stays within your own machine and dummy data;
- does not access, modify, or exfiltrate another person's data;
- does not degrade a live service, provider, or network;
- does not test against production infrastructure or third-party providers;
- gives us reasonable time to fix the issue before public disclosure.

If you are unsure whether a test is allowed, ask first through the private
channel.

## Non-security reports

Bugs, playback problems, and feature requests go to the normal issue tracker:

<https://github.com/satyawikananda/mumu/issues>

Please follow [`CONTRIBUTING.MD`](CONTRIBUTING.MD).
