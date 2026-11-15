# Contributing to Mumu TV

Contributions are welcome: code, documentation, reproducible bug reports,
feature proposals, and testing on macOS. You do not need to implement a feature
to help. Please follow the [Code of Conduct](CODE_OF_CONDUCT.MD).

## Find or propose work

Search [existing issues](https://github.com/satyawikananda/mumu/issues?q=is%3Aissue),
including closed issues, before opening a new one. Use the
[issue chooser](https://github.com/satyawikananda/mumu/issues/new/choose):

- **Bug report:** include reproduction steps, expected and actual behavior,
  the app version or commit, macOS version, and Mac chip. For playback or PiP,
  include the stream type, runtime details if known, and the actions involved.
  Screenshots and short recordings can help show the problem.
- **Feature request:** describe the problem, proposed behavior, scope,
  acceptance criteria, and verification plan. Link any specification or related
  work. Keep one complete feature or specification in each issue.

Discuss scope in the issue before starting a substantial change. For an
existing issue, comment with your proposed approach so contributors can avoid
duplicating work. Small documentation corrections can go directly into a PR.

Remove credentials, tokens, private playlist URLs, authentication headers, and
personal data from reports, logs, screenshots, and recordings. Use synthetic
fixtures or streams you are authorized to test. A public playlist listing does
not prove permission, availability, or compatibility.

## Set up development

Native development and playback validation target macOS 13 or later on Apple
Silicon. Use the Go version pinned in [go.mod](go.mod), currently Go 1.27.3,
and the pinned MyGo dependency. Cross-builds for other targets do not establish
runtime support.

1. Fork the repository on GitHub, then clone your fork.
2. Create a focused branch from the current `main` branch.
3. Check the toolchain and start the native development app:

   ```sh
   go tool mygo doctor
   go tool mygo dev
   ```

The development bundle is named **Mumu Dev**, separate from an installed app.
Source builds need a local libmpv runtime for playback; see the
[source-build instructions](README.md#libmpv-for-source-builds-developer-only).
The app can start without libmpv and reports a missing runtime when Play is
requested.

MyGo's SQLite native module may download on first use. Offline development
requires a pre-staged module; see [macOS packaging](docs/release/macos.md).

## Follow the project boundaries

Read [AGENTS.md](AGENTS.md) and the relevant
[architecture documentation](docs/architecture/README.md) before changing code.
For interface changes, also read [DESIGN.md](DESIGN.md) and the
[UX documentation](docs/ux/shell.md).

- Use Go and MyGo native UI. Do not introduce a WebView or browser frontend.
- Keep libmpv embedded. Watch and PiP share one logical playback session and
  one retained video surface; do not start an external player or second decoder.
- Keep playlists, favorites, history, and settings local. Preserve the startup
  flow and never autoplay on launch.
- Let UI dispatch intents, the player controller own playback lifetime, and
  the video adapter own presentation. Keep network and file work bounded and
  cancellable, away from the UI thread.
- Respect source permissions and network policy. Do not bypass access controls
  or treat the community catalog as rights clearance.

If a proposal conflicts with these constraints or a verified native limitation,
describe the conflict and discuss an alternative before implementation.

Keep changes focused. Use readable Go, errors with useful context, and comments
that explain non-obvious behavior. Verify pinned MyGo and libmpv APIs against
their actual source. Add table-driven tests for data logic and fake adapters for
player and network failures. Use a small experiment for uncertain native calls.

Update the relevant architecture, UX, or release documentation when behavior
changes. Update the design contract for UI changes. Keep local reference captures
and machine-specific QA artifacts out of git; attach redacted evidence to the
issue or PR instead. Curated product screenshots in `docs/screenshots/` are
maintained as README assets and are the exception to this rule.

## Verify changes

For code changes, run the project checks from the repository root:

```sh
go test ./...
go test -race ./...
go vet ./...
go tool mygo vet .
gofmt -l ./cmd ./internal
golangci-lint run ./cmd/... ./internal/...
```

Race tests require cgo. Formatting must print nothing. `make ci` runs formatting
checks, both vet commands, and race tests; `make lint` runs golangci-lint.
If golangci-lint is unavailable locally, say so in the PR; CI runs it. CI also
cross-builds other targets and checks that the app remains native-only.

For playback adapter changes on macOS arm64, run the gated integration tests
with a compatible runtime. Replace `/path/to/libmpv` with its directory:

```sh
MUMU_MPV_LIBRARY=/path/to/libmpv MUMU_MPV_TEST=1 MUMU_MPV_HEADER_TEST=1 \
  go test -run TestIntegration -v ./internal/player/libmpv
```

A skipped test is not a passing result. These adapter tests do not prove video
rendering or PiP behavior. For native UI or media changes, run the relevant
[manual macOS checks](docs/release/testing.md) using authorized fixtures and
record OS, architecture, MyGo/libmpv versions, commands, stream type, and observed
behavior. Record new native validation results in
[validation](docs/architecture/validation.md), including what remains unverified.

For packaging, runtime, or dependency changes, run `make package` on Apple
Silicon. It builds the LGPL runtime and app, produces the DMG, and runs the
bundle verification gate. Follow the [packaging guide](docs/release/macos.md)
and release checklist for prerequisites and manual distribution checks.

For documentation-only changes, check links, examples, Markdown formatting,
and any edited configuration syntax. App tests and native playback QA are not
required unless the change also affects code or runtime configuration.

## Submit a pull request

Push your branch to your fork and open a PR against `main`. GitHub prefills the
[PR template](.github/pull_request_template.md) once it reaches the repository's
default branch. Complete the applicable sections and remove optional sections
that do not apply. If a relevant check was not run, keep that limitation explicit.

Include:

- The problem and resulting behavior, with a linked issue when one exists.
- The scope of the change and any relevant design or implementation decisions.
- Commands actually run and their results, including failures and skips.
- Manual macOS observations separately from automated checks.
- Known limitations, unverified behavior, and redacted visual evidence when useful.

Use a clear title and keep unrelated changes in separate PRs. Use `Closes #123`
only when the PR fully addresses that issue. Address review feedback and keep
the PR description aligned with the final change.

## Maintain the changelog

Add a short entry under **Unreleased** in [CHANGELOG.MD](CHANGELOG.MD) for
meaningful user-facing changes, compatibility changes, or contributor workflow
changes. Use `## [Unreleased]` and `### Added`, `### Changed`, or `### Fixed`
as appropriate; omit empty categories.
Minor typo fixes and internal refactors without observable effects do not need
an entry. Describe the result rather than listing commits.

When publishing a release, the maintainer moves its entries into a version
section formatted as `## [X.Y.Z] - YYYY-MM-DD`, using the actual publication
date and Semantic Versioning. Link the version to the published release with a
Markdown reference link at the bottom of the changelog, and retain Unreleased
for subsequent work. Tags and draft
releases alone do not establish publication. Keep changelog entries consistent
with the published release notes and documented validation limits.

## Licensing

Mumu's application code uses the [MIT license](LICENSE). Check
[third-party notices](THIRD_PARTY_NOTICES.md) before adding dependencies or
assets; native runtime and bundled font licenses are separate. Preserve license
notices and credit sources. The Code of Conduct carries its own attribution and
CC BY-SA 4.0 license notice.
