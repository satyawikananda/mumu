# ADR 0001: Native MyGo UI only

- Status: accepted
- Date: 2026-11-09
- Issues: #3, #4

## Context

mumu is a desktop TV player that must open quickly, use little memory and
stay responsive while decoding video. M0 proved a native video surface can
live inside a MyGo window on the tested Mac, with libmpv rendering into a
child `NSOpenGLView` (`docs/architecture/native-playback-feasibility.md`).
MyGo also offers a web frontend mode, but that would add a webview, a
JavaScript build toolchain and a second rendering path.

## Decision

Every window uses MyGo native UI (`WindowOptions.Content` built with
`github.com/egoist/mygo/ui`). The repository has:

- no webview, `Page` or `WindowOptions.URL` usage;
- no `package.json`, `index.html`, TypeScript or JSX;
- no Bun, Node.js or other frontend runtime;
- no hosted backend or account service.

CI guards the file-level part of this rule: a build fails if frontend
project files or script sources appear. Code review checks the API-level
part, because a webview can be created from Go without those files.

## Consequences

- The interface is Go: tests use `ui.NewTester` without a window.
- Video presentation needs a native child surface, not a browser player.
  That path stays conditional on the M0 limits: OpenGL deprecation,
  presentation gaps and unmeasured per-process GPU cost.
- Do not solve a rendering or packaging problem by adding a webview or an
  external mpv process. That requires a new decision record and maintainer
  approval.
