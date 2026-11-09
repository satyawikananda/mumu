// Package player defines mumu's playback control and event-driven state
// machine.
//
// It owns no UI and no video surface. An Engine adapter supplies media
// playback; the native surface adapter is separate. All state changes are
// published as comparable snapshots so the app can dispatch them to MyGo on
// the main thread.
package player
