// Package video owns native video surface presentation.
//
// It stays separate from playback control: it receives a player.Renderer,
// keeps one retained native view, and runs one render thread. The player
// controller in #5 never imports it, and the UI dispatches intents instead of
// touching the surface.
//
// Only darwin/arm64 carries the M0-proven NSOpenGLView bridge. Other targets
// report ErrUnsupported so cross-builds stay honest.
package video
