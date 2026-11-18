# Changelog

All notable changes to Mumu TV are recorded here. Versions follow
[Semantic Versioning](https://semver.org/).

## [0.1.0]

Mumu TV's first release targets Apple Silicon Macs running macOS 13 or later.
The DMG bundles the playback runtime, so Homebrew is optional. The app uses
ad-hoc signing and has not been notarized. If macOS blocks the first launch,
follow the [installation guide](README.md#install-a-published-build).

### Added

- Watch live TV in the native player and move the same stream into a floating
  Picture-in-Picture window. Pause live TV, resume at the live edge, or switch
  between the stream qualities a channel offers.
- Browse the iptv-org community catalog by country and category, search for a
  channel, or import your own M3U playlist from a file or URL. You can also
  open a direct stream link.
- Keep playlists, favorites, recent channels, and preferences on your Mac.
  Home lets you resume a recent channel when you choose to play it.
- Open Control Center over live video, use keyboard shortcuts, and choose a
  light, dark, or system appearance. Check for updates from Settings.
- Verify the DMG with the supplied SHA-256 checksum. The release package
  includes third-party notices and source code for the bundled LGPL runtime,
  which uses software decoding. See the [macOS packaging guide](docs/release/macos.md).
- Explore the multilingual Mumu TV website for screenshots and interactive
  demos of Picture-in-Picture and favorites.
- Report bugs with screenshots or video using the issue forms, or follow the
  [contribution guide](CONTRIBUTING.MD) to submit a pull request.
