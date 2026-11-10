// Package player defines mumu's playback control and event-driven state
// machine.
//
// It owns no UI and no video surface. An Engine adapter supplies media
// playback; the native surface adapter is separate and reaches frames through
// the narrow Renderer port in render.go. All state changes are published as
// comparable snapshots so the app can dispatch them to MyGo on the main thread.
package player
