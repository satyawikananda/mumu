# Release

- [macOS packaging](macos.md): requirements, runtime build, bundle verification,
  release assets and unsigned distribution.
- [Testing](testing.md): automated checks, manual QA matrix and the release
  checklist.

Release artifacts are arm64 macOS only. The app is ad-hoc signed and not
notarized. Verify the published SHA-256 and follow the Gatekeeper steps in the
packaging guide.
